package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *Store) SetAgreed(ctx context.Context, chatID int64) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO users (chat_id, agreed_at) VALUES (?, ?)
		 ON CONFLICT(chat_id) DO UPDATE SET agreed_at = excluded.agreed_at`,
		chatID, time.Now().Unix())
	return err
}

func (s *Store) IsAgreed(ctx context.Context, chatID int64) (bool, error) {
	var agreedAt sql.NullInt64
	err := s.DB.QueryRowContext(ctx,
		`SELECT agreed_at FROM users WHERE chat_id = ?`, chatID).Scan(&agreedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return agreedAt.Valid, nil
}
