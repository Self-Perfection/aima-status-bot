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
	FailThreshold = 5 // после стольких подряд неудачных fetch'ей URL снимается
)

// Notifier — интерфейс, который реализует bot. Вынесен сюда, чтобы
// monitor не зависел от bot и не было циклического импорта.
type Notifier interface {
	NotifyStatusChange(ctx context.Context, chatID, subID int64, nickname string, oldStatus *int, newStatus int) error
	NotifyApproved(ctx context.Context, chatID, subID int64, nickname string, history []store.StatusEntry) error
	NotifyDead(ctx context.Context, chatID, subID int64, nickname string, failCount int) error
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
		return nil // idle, не шумим в логах
	}

	overdue, err := m.store.CountOverdueURLs(ctx, m.minAge)
	if err != nil {
		// не критично, не валим step — просто 0 в логе
		overdue = 0
	}

	rawURL, err := crypto.Decrypt(due.EncryptedURL, m.encKey)
	if err != nil {
		slog.Error("monitor: decrypt failed", "url_id", due.ID, "err", err)
		return err
	}

	status, err := m.fetcher.FetchStatus(ctx, string(rawURL))
	if err != nil {
		n, recErr := m.store.MarkFetchFailed(ctx, due.ID)
		if recErr != nil {
			slog.Error("monitor: mark fetch failed", "url_id", due.ID, "err", recErr)
		}
		// Никогда не логируем сам URL — только id и причину.
		slog.Warn("monitor: fetch failed",
			"url_id", due.ID, "fail_count", n, "overdue", overdue-1, "err", err)
		if n >= FailThreshold {
			m.handleDead(ctx, due.ID, n)
		}
		return nil
	}

	if due.LastStatus != nil && *due.LastStatus == status {
		if err := m.store.MarkFetchedNoChange(ctx, due.ID); err != nil {
			return err
		}
		slog.Info("monitor: fetched (no change)",
			"url_id", due.ID, "status", status, "overdue", overdue-1)
		return nil
	}

	if err := m.store.RecordStatus(ctx, due.ID, status); err != nil {
		return err
	}

	subs, err := m.store.Subscribers(ctx, due.ID)
	if err != nil {
		return err
	}

	if status == aima.StatusApproved {
		slog.Info("monitor: approved (auto-removing)",
			"url_id", due.ID, "subscribers", len(subs), "overdue", overdue-1)
		m.handleApproved(ctx, due.ID, subs)
		return nil
	}

	from := 0
	if due.LastStatus != nil {
		from = *due.LastStatus
	}
	slog.Info("monitor: status changed",
		"url_id", due.ID, "from", from, "to", status,
		"subscribers", len(subs), "overdue", overdue-1)

	for _, s := range subs {
		if err := m.notifier.NotifyStatusChange(ctx, s.ChatID, s.SubID, s.Nickname, due.LastStatus, status); err != nil {
			slog.Warn("monitor: notify failed", "chat_id", s.ChatID, "err", err)
		}
	}
	return nil
}

// handleApproved отправляет каждому подписчику сводку переходов и
// удаляет URL целиком (subscriptions + history каскадом).
func (m *Monitor) handleApproved(ctx context.Context, urlID int64, subs []store.SubscriberInfo) {
	history, err := m.store.StatusHistory(ctx, urlID)
	if err != nil {
		slog.Error("status history", "url_id", urlID, "err", err)
		// без истории всё равно уведомим — лучше скудное письмо, чем тишина
	}
	for _, s := range subs {
		if err := m.notifier.NotifyApproved(ctx, s.ChatID, s.SubID, s.Nickname, history); err != nil {
			slog.Warn("notify approved failed", "chat_id", s.ChatID, "err", err)
		}
	}
	if err := m.store.RemoveURL(ctx, urlID); err != nil {
		slog.Error("remove approved url", "url_id", urlID, "err", err)
	}
}

// handleDead снимает URL после исчерпания лимита неудачных fetch'ей
// и оповещает всех подписчиков.
func (m *Monitor) handleDead(ctx context.Context, urlID int64, failCount int) {
	subs, err := m.store.Subscribers(ctx, urlID)
	if err != nil {
		slog.Error("subscribers for dead url", "url_id", urlID, "err", err)
		return
	}
	for _, s := range subs {
		if err := m.notifier.NotifyDead(ctx, s.ChatID, s.SubID, s.Nickname, failCount); err != nil {
			slog.Warn("notify dead failed", "chat_id", s.ChatID, "err", err)
		}
	}
	if err := m.store.RemoveURL(ctx, urlID); err != nil {
		slog.Error("remove dead url", "url_id", urlID, "err", err)
	}
}
