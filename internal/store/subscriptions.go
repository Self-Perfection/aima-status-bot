package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ErrAlreadySubscribed возвращается AddSubscription, если у chat_id
// уже есть подписка на этот URL (UNIQUE-ограничение).
var ErrAlreadySubscribed = errors.New("already subscribed to this URL")

func (s *Store) CountSubscriptions(ctx context.Context, chatID int64) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM subscriptions WHERE chat_id = ?`, chatID).Scan(&n)
	return n, err
}

// AddSubscription создаёт (или находит по url_hash) запись в
// monitored_urls и подписывает chat_id. На первой регистрации URL
// добавляет начальную запись в status_history. Атомарно через tx.
func (s *Store) AddSubscription(
	ctx context.Context,
	chatID int64,
	encryptedURL []byte,
	urlHash, nickname string,
	status int,
) (subID int64, err error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().Unix()

	var urlID int64
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM monitored_urls WHERE url_hash = ?`, urlHash).Scan(&urlID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, ierr := tx.ExecContext(ctx,
			`INSERT INTO monitored_urls
			   (url_encrypted, url_hash, last_status, last_fetched_at, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			encryptedURL, urlHash, status, now, now)
		if ierr != nil {
			err = ierr
			return 0, err
		}
		urlID, err = res.LastInsertId()
		if err != nil {
			return 0, err
		}
		if _, ierr := tx.ExecContext(ctx,
			`INSERT INTO status_history (monitored_url_id, status, observed_at)
			 VALUES (?, ?, ?)`, urlID, status, now); ierr != nil {
			err = ierr
			return 0, err
		}
	case err != nil:
		return 0, err
	}
	// URL уже известен — не трогаем monitored_urls.
	// Следующий monitor-тик подхватит свежий статус, если он изменился.

	var nick sql.NullString
	if nickname != "" {
		nick = sql.NullString{String: nickname, Valid: true}
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO subscriptions (chat_id, monitored_url_id, nickname, created_at)
		 VALUES (?, ?, ?, ?)`, chatID, urlID, nick, now)
	if err != nil {
		if isUniqueViolation(err) {
			err = ErrAlreadySubscribed
		}
		return 0, err
	}
	subID, err = res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return subID, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// SubscriptionInfo — то, что показывается пользователю в /list.
// LastStatus/LastFetched могут быть nil если URL ещё ни разу не фетчился.
type SubscriptionInfo struct {
	ID          int64
	Nickname    string
	LastStatus  *int
	LastFetched *time.Time
	CreatedAt   time.Time
}

func (s *Store) ListSubscriptions(ctx context.Context, chatID int64) ([]SubscriptionInfo, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT s.id, s.nickname, m.last_status, m.last_fetched_at, s.created_at
		FROM subscriptions s
		JOIN monitored_urls m ON m.id = s.monitored_url_id
		WHERE s.chat_id = ?
		ORDER BY s.created_at`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SubscriptionInfo
	for rows.Next() {
		var (
			info        SubscriptionInfo
			nick        sql.NullString
			lastStatus  sql.NullInt64
			lastFetched sql.NullInt64
			created     int64
		)
		if err := rows.Scan(&info.ID, &nick, &lastStatus, &lastFetched, &created); err != nil {
			return nil, err
		}
		if nick.Valid {
			info.Nickname = nick.String
		}
		if lastStatus.Valid {
			n := int(lastStatus.Int64)
			info.LastStatus = &n
		}
		if lastFetched.Valid {
			t := time.Unix(lastFetched.Int64, 0)
			info.LastFetched = &t
		}
		info.CreatedAt = time.Unix(created, 0)
		out = append(out, info)
	}
	return out, rows.Err()
}

// RemoveSubscription удаляет подписку по id, проверяя что она
// принадлежит chatID. После удаления GC-чистит monitored_urls,
// у которых не осталось подписчиков. Возвращает true если что-то
// удалилось.
func (s *Store) RemoveSubscription(ctx context.Context, chatID, subID int64) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`DELETE FROM subscriptions WHERE id = ? AND chat_id = ?`, subID, chatID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, tx.Commit()
	}
	if err := gcOrphanURLs(ctx, tx); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// ForgetUser удаляет все подписки chat_id, осиротевшие monitored_urls
// и саму запись в users.
func (s *Store) ForgetUser(ctx context.Context, chatID int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM subscriptions WHERE chat_id = ?`, chatID); err != nil {
		return err
	}
	if err := gcOrphanURLs(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM users WHERE chat_id = ?`, chatID); err != nil {
		return err
	}
	return tx.Commit()
}

func gcOrphanURLs(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx,
		`DELETE FROM monitored_urls
		 WHERE id NOT IN (SELECT monitored_url_id FROM subscriptions)`)
	return err
}
