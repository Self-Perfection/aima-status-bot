package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// DueURL — единица работы для monitor: id, зашифрованный URL,
// предыдущий известный статус.
type DueURL struct {
	ID           int64
	EncryptedURL []byte
	LastStatus   *int
}

// CountOverdueURLs возвращает число URL, у которых last_fetched_at
// старше now-minAge (или NULL). Используется monitor'ом как метрика
// «глубина очереди» — если растёт, пора ускорять тик.
func (s *Store) CountOverdueURLs(ctx context.Context, minAge time.Duration) (int, error) {
	cutoff := time.Now().Add(-minAge).Unix()
	var n int
	err := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM monitored_urls
		 WHERE last_fetched_at IS NULL OR last_fetched_at < ?`, cutoff).Scan(&n)
	return n, err
}

// PickDueURL возвращает URL с самой давней проверкой, но только если
// его last_fetched_at старше now-minAge (или NULL). Если подходящих
// нет — (nil, nil).
func (s *Store) PickDueURL(ctx context.Context, minAge time.Duration) (*DueURL, error) {
	cutoff := time.Now().Add(-minAge).Unix()
	var (
		id           int64
		encryptedURL []byte
		lastStatus   sql.NullInt64
	)
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, url_encrypted, last_status
		FROM monitored_urls
		WHERE last_fetched_at IS NULL OR last_fetched_at < ?
		ORDER BY COALESCE(last_fetched_at, 0)
		LIMIT 1`, cutoff).Scan(&id, &encryptedURL, &lastStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := &DueURL{ID: id, EncryptedURL: encryptedURL}
	if lastStatus.Valid {
		n := int(lastStatus.Int64)
		out.LastStatus = &n
	}
	return out, nil
}

// RecordStatus обновляет last_status/last_fetched_at и пишет
// строку в status_history. fail_count сбрасывается в 0.
func (s *Store) RecordStatus(ctx context.Context, urlID int64, status int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx,
		`UPDATE monitored_urls
		 SET last_status = ?, last_fetched_at = ?, fail_count = 0
		 WHERE id = ?`, status, now, urlID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO status_history (monitored_url_id, status, observed_at)
		 VALUES (?, ?, ?)`, urlID, status, now); err != nil {
		return err
	}
	return tx.Commit()
}

// MarkFetchedNoChange используется когда статус прежний — обновляем
// только last_fetched_at и сбрасываем fail_count.
func (s *Store) MarkFetchedNoChange(ctx context.Context, urlID int64) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE monitored_urls SET last_fetched_at = ?, fail_count = 0 WHERE id = ?`,
		time.Now().Unix(), urlID)
	return err
}

// MarkFetchFailed увеличивает fail_count и обновляет last_fetched_at,
// чтобы failing URL не блокировал очередь. Возвращает новое значение
// fail_count, чтобы caller мог решить о удалении.
func (s *Store) MarkFetchFailed(ctx context.Context, urlID int64) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`UPDATE monitored_urls
		 SET last_fetched_at = ?, fail_count = fail_count + 1
		 WHERE id = ?`, time.Now().Unix(), urlID); err != nil {
		return 0, err
	}
	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT fail_count FROM monitored_urls WHERE id = ?`, urlID).Scan(&n); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// SubscriberInfo — кому слать уведомления при изменении статуса URL.
type SubscriberInfo struct {
	ChatID   int64
	SubID    int64
	Nickname string
}

// StatusEntry — одна точка журнала статусов URL.
type StatusEntry struct {
	Status     int
	ObservedAt time.Time
}

func (s *Store) StatusHistory(ctx context.Context, urlID int64) ([]StatusEntry, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT status, observed_at FROM status_history
		 WHERE monitored_url_id = ? ORDER BY observed_at`, urlID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StatusEntry
	for rows.Next() {
		var (
			status     int
			observedAt int64
		)
		if err := rows.Scan(&status, &observedAt); err != nil {
			return nil, err
		}
		out = append(out, StatusEntry{Status: status, ObservedAt: time.Unix(observedAt, 0)})
	}
	return out, rows.Err()
}

// RemoveURL удаляет monitored_urls и каскадом — subscriptions и
// status_history. Используется monitor'ом при одобрении и при
// исчерпании лимита неудачных fetch'ей.
func (s *Store) RemoveURL(ctx context.Context, urlID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM monitored_urls WHERE id = ?`, urlID)
	return err
}

func (s *Store) Subscribers(ctx context.Context, urlID int64) ([]SubscriberInfo, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT chat_id, id, nickname FROM subscriptions WHERE monitored_url_id = ?`, urlID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SubscriberInfo
	for rows.Next() {
		var info SubscriberInfo
		var nick sql.NullString
		if err := rows.Scan(&info.ChatID, &info.SubID, &nick); err != nil {
			return nil, err
		}
		if nick.Valid {
			info.Nickname = nick.String
		}
		out = append(out, info)
	}
	return out, rows.Err()
}
