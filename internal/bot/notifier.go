package bot

import (
	"context"
	"fmt"

	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/Self-Perfection/aima-renew-watch-bot/internal/aima"
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
