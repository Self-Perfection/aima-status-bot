package bot

import (
	"context"
	"fmt"
	"strings"

	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/Self-Perfection/aima-renew-watch-bot/internal/aima"
	"github.com/Self-Perfection/aima-renew-watch-bot/internal/store"
)

// NotifyStatusChange реализует monitor.Notifier. Сообщение
// формируется: «Статус подписки <name>: A → B (label)» (или просто
// «B (label)», если предыдущий статус неизвестен).
func (b *Bot) NotifyStatusChange(
	ctx context.Context,
	chatID, subID int64,
	nickname string,
	oldStatus *int,
	newStatus int,
) error {
	name := nickname
	if name == "" {
		name = fmt.Sprintf("#%d", subID)
	}
	var text string
	if oldStatus == nil {
		text = fmt.Sprintf("Статус подписки %s: %d (%s)",
			name, newStatus, aima.Label(newStatus))
	} else {
		text = fmt.Sprintf("Статус подписки %s: %d (%s) → %d (%s)",
			name,
			*oldStatus, aima.Label(*oldStatus),
			newStatus, aima.Label(newStatus))
	}
	_, err := b.api.SendMessage(ctx, tu.Message(tu.ID(chatID), text))
	return err
}

// NotifyApproved отправляет «🎉 одобрено» со сводкой переходов и
// сообщает что подписка снята (это делает monitor после notify).
func (b *Bot) NotifyApproved(
	ctx context.Context,
	chatID, subID int64,
	nickname string,
	history []store.StatusEntry,
) error {
	name := nickname
	if name == "" {
		name = fmt.Sprintf("#%d", subID)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "🎉 Подписка %s: одобрено!\n", name)
	if len(history) > 0 {
		sb.WriteString("\nСводка переходов:\n")
		for _, e := range history {
			fmt.Fprintf(&sb, "%s — %d (%s)\n",
				e.ObservedAt.Format("2006-01-02 15:04"),
				e.Status, aima.Label(e.Status))
		}
	}
	sb.WriteString("\nПодписка снята, данные удалены.")
	_, err := b.api.SendMessage(ctx, tu.Message(tu.ID(chatID), sb.String()))
	return err
}

// NotifyDead — URL перестал отвечать после N неудачных попыток.
func (b *Bot) NotifyDead(
	ctx context.Context,
	chatID, subID int64,
	nickname string,
	failCount int,
) error {
	name := nickname
	if name == "" {
		name = fmt.Sprintf("#%d", subID)
	}
	text := fmt.Sprintf(
		"Подписка %s: ссылка перестала отвечать (%d неудачных попыток подряд). "+
			"Возможно, токен в URL больше не валиден. Подписка снята.",
		name, failCount)
	_, err := b.api.SendMessage(ctx, tu.Message(tu.ID(chatID), text))
	return err
}
