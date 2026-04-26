package bot

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/Self-Perfection/aima-renew-watch-bot/internal/aima"
	"github.com/Self-Perfection/aima-renew-watch-bot/internal/crypto"
	"github.com/Self-Perfection/aima-renew-watch-bot/internal/store"
)

const disclaimerText = `🔔 AIMA Renew Watch Bot

Я мониторю страницы Validação на portal-renovacoes.aima.gov.pt и присылаю сообщение, когда меняется числовой статус заявки на ВНЖ.

⚠️ Важно про приватность.
URL страницы Validação — это credential. По нему открывается ваша персональная информация: имя, NIF, NISS, NIE, адрес, email, тип заявки. Когда вы подписываете URL на мониторинг, оператор бота (Self-Perfection) и сервер, на котором бот работает, технически имеют доступ к этим данным.

Что бот делает:
• хранит URL зашифрованным в БД
• никогда не логирует URL и тело ответов AIMA
• сохраняет только числовой статус (1, 5, 11, 14, 15, 20, 6)
• автоматически снимает с мониторинга при статусе 6 (Aprovado)
• /forget_me удаляет все ваши данные мгновенно

Лимит: 4 подписки на одного пользователя.

Исходный код: https://github.com/Self-Perfection/aima-renew-watch-bot

Если согласны — отправьте /agree`

const helpText = `Команды:
/start — приветствие и дисклеймер
/agree — подтвердить согласие на обработку данных
/add <url> [имя] — подписаться на мониторинг (после /agree)
/list — мои подписки
/remove <id> — отписаться
/forget_me — удалить все мои данные
/help — это сообщение`

func (b *Bot) handleStart(ctx *th.Context, msg telego.Message) error {
	return b.send(ctx, msg.Chat.ID, disclaimerText)
}

func (b *Bot) handleHelp(ctx *th.Context, msg telego.Message) error {
	return b.send(ctx, msg.Chat.ID, helpText)
}

func (b *Bot) handleAgree(ctx *th.Context, msg telego.Message) error {
	if err := b.store.SetAgreed(ctx, msg.Chat.ID); err != nil {
		return b.send(ctx, msg.Chat.ID, "Не удалось сохранить согласие. Попробуйте позже.")
	}
	return b.send(ctx, msg.Chat.ID, "✓ Согласие записано. Используйте /add <url> [имя], чтобы подписаться. /help — список команд.")
}

func (b *Bot) handleAdd(ctx *th.Context, msg telego.Message) error {
	if ok, err := b.requireConsent(ctx, msg.Chat.ID); !ok {
		return err
	}
	parts := strings.Fields(msg.Text)
	if len(parts) < 2 {
		return b.send(ctx, msg.Chat.ID,
			"Использование: /add <url> [имя]\n"+
				"Например: /add https://portal-renovacoes.aima.gov.pt/ords/r/aima/aima-pr/validar?... я")
	}
	rawURL := parts[1]
	var nickname string
	if len(parts) >= 3 {
		nickname = strings.TrimSpace(strings.Join(parts[2:], " "))
	}

	if !aima.IsValidacaoURL(rawURL) {
		return b.send(ctx, msg.Chat.ID,
			"URL не похож на страницу Validação. Ожидается "+
				"https://portal-renovacoes.aima.gov.pt/ords/r/aima/aima-pr/validar?...")
	}
	canonical, hash, err := aima.NormalizeURL(rawURL)
	if err != nil {
		return b.send(ctx, msg.Chat.ID, "URL не парсится: "+err.Error())
	}

	count, err := b.store.CountSubscriptions(ctx, msg.Chat.ID)
	if err != nil {
		slog.Error("count subscriptions", "err", err)
		return b.send(ctx, msg.Chat.ID, "Внутренняя ошибка.")
	}
	if count >= MaxSubscriptionsPerUser {
		return b.send(ctx, msg.Chat.ID,
			fmt.Sprintf("У вас уже %d подписок (лимит). Сначала /remove.", MaxSubscriptionsPerUser))
	}

	status, err := b.fetcher.FetchStatus(ctx, canonical)
	if err != nil {
		if errors.Is(err, aima.ErrStatusNotFound) {
			return b.send(ctx, msg.Chat.ID,
				"На странице не нашёлся индикатор статуса. Возможно, токен в URL уже не работает.")
		}
		return b.send(ctx, msg.Chat.ID, "Не удалось загрузить страницу: "+err.Error())
	}

	encrypted, err := crypto.Encrypt([]byte(canonical), b.encKey)
	if err != nil {
		slog.Error("encrypt url", "err", err)
		return b.send(ctx, msg.Chat.ID, "Внутренняя ошибка.")
	}

	subID, err := b.store.AddSubscription(ctx, msg.Chat.ID, encrypted, hash, nickname, status)
	if err != nil {
		if errors.Is(err, store.ErrAlreadySubscribed) {
			return b.send(ctx, msg.Chat.ID, "Вы уже подписаны на этот URL.")
		}
		slog.Error("add subscription", "err", err)
		return b.send(ctx, msg.Chat.ID, "Не удалось сохранить.")
	}

	name := nickname
	if name == "" {
		name = fmt.Sprintf("#%d", subID)
	}
	return b.send(ctx, msg.Chat.ID,
		fmt.Sprintf("✓ Подписка %s добавлена.\nТекущий статус: %d (%s)",
			name, status, aima.Label(status)))
}

func (b *Bot) handleList(ctx *th.Context, msg telego.Message) error {
	if ok, err := b.requireConsent(ctx, msg.Chat.ID); !ok {
		return err
	}
	subs, err := b.store.ListSubscriptions(ctx, msg.Chat.ID)
	if err != nil {
		slog.Error("list subscriptions", "err", err)
		return b.send(ctx, msg.Chat.ID, "Внутренняя ошибка.")
	}
	if len(subs) == 0 {
		return b.send(ctx, msg.Chat.ID, "У вас нет подписок. Используйте /add <url> [имя].")
	}

	var sb strings.Builder
	sb.WriteString("Ваши подписки:\n")
	for _, s := range subs {
		name := s.Nickname
		if name == "" {
			name = fmt.Sprintf("#%d", s.ID)
		}
		fmt.Fprintf(&sb, "• %d — %s — ", s.ID, name)
		if s.LastStatus != nil {
			fmt.Fprintf(&sb, "статус %d (%s)", *s.LastStatus, aima.Label(*s.LastStatus))
			if s.LastFetched != nil {
				fmt.Fprintf(&sb, ", обновлено %s назад",
					time.Since(*s.LastFetched).Round(time.Minute))
			}
		} else {
			sb.WriteString("ещё не проверен")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n/remove <id> — отписаться")
	return b.send(ctx, msg.Chat.ID, sb.String())
}

func (b *Bot) handleRemove(ctx *th.Context, msg telego.Message) error {
	if ok, err := b.requireConsent(ctx, msg.Chat.ID); !ok {
		return err
	}
	parts := strings.Fields(msg.Text)
	if len(parts) < 2 {
		return b.send(ctx, msg.Chat.ID, "Использование: /remove <id>\n/list — посмотреть id своих подписок")
	}
	subID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return b.send(ctx, msg.Chat.ID, "id должен быть числом. /list — посмотреть id своих подписок")
	}

	removed, err := b.store.RemoveSubscription(ctx, msg.Chat.ID, subID)
	if err != nil {
		slog.Error("remove subscription", "err", err, "sub_id", subID)
		return b.send(ctx, msg.Chat.ID, "Внутренняя ошибка.")
	}
	if !removed {
		return b.send(ctx, msg.Chat.ID, "Подписка с таким id не найдена.")
	}
	return b.send(ctx, msg.Chat.ID, "✓ Подписка снята.")
}

func (b *Bot) handleForgetMe(ctx *th.Context, msg telego.Message) error {
	if err := b.store.ForgetUser(ctx, msg.Chat.ID); err != nil {
		slog.Error("forget user", "err", err)
		return b.send(ctx, msg.Chat.ID, "Внутренняя ошибка.")
	}
	return b.send(ctx, msg.Chat.ID, "✓ Все ваши данные удалены. Если захотите вернуться — /start.")
}

func (b *Bot) requireConsent(ctx *th.Context, chatID int64) (bool, error) {
	agreed, err := b.store.IsAgreed(ctx, chatID)
	if err != nil {
		return false, b.send(ctx, chatID, "Внутренняя ошибка. Попробуйте позже.")
	}
	if !agreed {
		return false, b.send(ctx, chatID, "Сначала прочтите /start и подтвердите /agree.")
	}
	return true, nil
}

func (b *Bot) send(ctx *th.Context, chatID int64, text string) error {
	_, err := ctx.Bot().SendMessage(ctx, tu.Message(tu.ID(chatID), text))
	return err
}
