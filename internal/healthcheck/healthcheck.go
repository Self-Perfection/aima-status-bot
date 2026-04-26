// Package healthcheck отправляет периодический GET на healthchecks.io
// (или совместимый сервис), чтобы внешний наблюдатель знал, что бот
// жив. Если URL пустой — Run возвращается сразу, фоновых горутин нет.
package healthcheck

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Run блокируется до отмены ctx и каждые interval секунд дёргает url.
// Любая ошибка только логируется — не валим бот из-за пинга.
func Run(ctx context.Context, url string, interval time.Duration) {
	if url == "" {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	ping := func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			slog.Warn("healthcheck: build request failed", "err", err)
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			slog.Warn("healthcheck: request failed", "err", err)
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			slog.Warn("healthcheck: non-2xx response", "status", resp.StatusCode)
		}
	}

	ping() // сразу дать сигнал «поднялись»
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ping()
		}
	}
}
