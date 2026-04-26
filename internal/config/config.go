package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"time"
)

type Config struct {
	BotToken          string
	DBPath            string
	EncKey            []byte
	HealthcheckURL    string
	HealthcheckEvery  time.Duration
}

// Load reads configuration from environment variables:
//
//	BOT_TOKEN          — Telegram bot token from @BotFather (required)
//	DB_PATH            — path to SQLite file (default: ./bot.db)
//	ENC_KEY            — base64-encoded 32 bytes for AES-256-GCM (required)
//	                     generate: openssl rand -base64 32
//	HEALTHCHECK_URL    — optional URL to GET periodically (e.g., healthchecks.io)
//	HEALTHCHECK_EVERY  — Go duration string, default 5m (only used if URL set)
func Load() (*Config, error) {
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("BOT_TOKEN is required")
	}

	encKeyB64 := os.Getenv("ENC_KEY")
	if encKeyB64 == "" {
		return nil, fmt.Errorf("ENC_KEY is required (base64-encoded 32 bytes)")
	}
	encKey, err := base64.StdEncoding.DecodeString(encKeyB64)
	if err != nil {
		return nil, fmt.Errorf("ENC_KEY: invalid base64: %w", err)
	}
	if len(encKey) != 32 {
		return nil, fmt.Errorf("ENC_KEY: must decode to 32 bytes, got %d", len(encKey))
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./bot.db"
	}

	hcEvery := 5 * time.Minute
	if v := os.Getenv("HEALTHCHECK_EVERY"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("HEALTHCHECK_EVERY: %w", err)
		}
		hcEvery = d
	}

	return &Config{
		BotToken:         token,
		DBPath:           dbPath,
		EncKey:           encKey,
		HealthcheckURL:   os.Getenv("HEALTHCHECK_URL"),
		HealthcheckEvery: hcEvery,
	}, nil
}
