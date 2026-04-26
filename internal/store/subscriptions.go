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
