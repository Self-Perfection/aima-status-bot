package bot

import (
	"context"
	"fmt"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"

	"github.com/Self-Perfection/aima-renew-watch-bot/internal/aima"
	"github.com/Self-Perfection/aima-renew-watch-bot/internal/store"
)

const MaxSubscriptionsPerUser = 4

type Bot struct {
	api     *telego.Bot
	store   *store.Store
	fetcher *aima.Fetcher
	encKey  []byte
}

func New(token string, st *store.Store, fetcher *aima.Fetcher, encKey []byte) (*Bot, error) {
	api, err := telego.NewBot(token)
	if err != nil {
		return nil, fmt.Errorf("telego.NewBot: %w", err)
	}
	return &Bot{api: api, store: st, fetcher: fetcher, encKey: encKey}, nil
}

func (b *Bot) Run(ctx context.Context) error {
	updates, err := b.api.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		return fmt.Errorf("UpdatesViaLongPolling: %w", err)
	}

	bh, err := th.NewBotHandler(b.api, updates)
	if err != nil {
		return fmt.Errorf("NewBotHandler: %w", err)
	}

	bh.HandleMessage(b.handleStart, th.CommandEqual("start"))
	bh.HandleMessage(b.handleHelp, th.CommandEqual("help"))
	bh.HandleMessage(b.handleAgree, th.CommandEqual("agree"))
	bh.HandleMessage(b.handleAdd, th.CommandEqual("add"))
	bh.HandleMessage(b.handleList, th.CommandEqual("list"))
	bh.HandleMessage(b.handleRemove, th.CommandEqual("remove"))
	bh.HandleMessage(b.handleForgetMe, th.CommandEqual("forget_me"))

	go func() {
		<-ctx.Done()
		_ = bh.Stop()
	}()
	return bh.Start()
}
