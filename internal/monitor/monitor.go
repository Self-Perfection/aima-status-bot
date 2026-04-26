// Package monitor крутит периодический опрос AIMA. Каждый тик берёт
// один самый старый «просроченный» URL, фетчит, при изменении статуса
// дёргает Notifier для каждого подписчика. Сериализованная очередь:
// один URL за тик, AIMA не бомбим.
package monitor

import (
	"context"
	"log/slog"
	"time"

	"github.com/Self-Perfection/aima-renew-watch-bot/internal/aima"
	"github.com/Self-Perfection/aima-renew-watch-bot/internal/crypto"
	"github.com/Self-Perfection/aima-renew-watch-bot/internal/store"
)

const (
	DefaultMinAge = 2 * time.Hour
	DefaultTick   = 1 * time.Minute
)

// Notifier — интерфейс, который реализует bot. Вынесен сюда, чтобы
// monitor не зависел от bot и не было циклического импорта.
type Notifier interface {
	NotifyStatusChange(ctx context.Context, chatID, subID int64, nickname string, oldStatus *int, newStatus int) error
}

type Monitor struct {
	store    *store.Store
	fetcher  *aima.Fetcher
	notifier Notifier
	encKey   []byte
	minAge   time.Duration
	tick     time.Duration
}

func New(st *store.Store, fetcher *aima.Fetcher, n Notifier, encKey []byte, minAge, tick time.Duration) *Monitor {
	if minAge == 0 {
		minAge = DefaultMinAge
	}
	if tick == 0 {
		tick = DefaultTick
	}
	return &Monitor{
		store: st, fetcher: fetcher, notifier: n,
		encKey: encKey, minAge: minAge, tick: tick,
	}
}

// Run блокируется до отмены ctx.
func (m *Monitor) Run(ctx context.Context) {
	t := time.NewTicker(m.tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := m.step(ctx); err != nil {
				slog.Warn("monitor step failed", "err", err)
			}
		}
	}
}

func (m *Monitor) step(ctx context.Context) error {
	due, err := m.store.PickDueURL(ctx, m.minAge)
	if err != nil {
		return err
	}
	if due == nil {
		return nil
	}

	rawURL, err := crypto.Decrypt(due.EncryptedURL, m.encKey)
	if err != nil {
		slog.Error("decrypt failed", "url_id", due.ID, "err", err)
		return err
	}

	status, err := m.fetcher.FetchStatus(ctx, string(rawURL))
	if err != nil {
		n, recErr := m.store.MarkFetchFailed(ctx, due.ID)
		if recErr != nil {
			slog.Error("mark fetch failed", "url_id", due.ID, "err", recErr)
		}
		// Не логируем сам URL — только id и причину.
		slog.Warn("fetch failed", "url_id", due.ID, "fail_count", n, "err", err)
		return nil
	}

	if due.LastStatus != nil && *due.LastStatus == status {
		return m.store.MarkFetchedNoChange(ctx, due.ID)
	}

	if err := m.store.RecordStatus(ctx, due.ID, status); err != nil {
		return err
	}

	subs, err := m.store.Subscribers(ctx, due.ID)
	if err != nil {
		return err
	}
	for _, s := range subs {
		if err := m.notifier.NotifyStatusChange(ctx, s.ChatID, s.SubID, s.Nickname, due.LastStatus, status); err != nil {
			slog.Warn("notify failed", "chat_id", s.ChatID, "err", err)
		}
	}
	return nil
}
