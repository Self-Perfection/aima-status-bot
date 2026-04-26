package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Self-Perfection/aima-renew-watch-bot/internal/bot"
	"github.com/Self-Perfection/aima-renew-watch-bot/internal/config"
	"github.com/Self-Perfection/aima-renew-watch-bot/internal/healthcheck"
	"github.com/Self-Perfection/aima-renew-watch-bot/internal/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		logger.Error("open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	b, err := bot.New(cfg.BotToken, st)
	if err != nil {
		logger.Error("init bot", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go healthcheck.Run(ctx, cfg.HealthcheckURL, cfg.HealthcheckEvery)

	if err := b.Run(ctx); err != nil {
		logger.Error("bot exited", "err", err)
		os.Exit(1)
	}
}
