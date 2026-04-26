package aima

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	DefaultTimeout = 30 * time.Second
	EstadoElement  = "P72_ESTADO_1"
)

// ErrStatusNotFound — страница загрузилась, но #P72_ESTADO_1 не нашли.
// Признак протухшего/невалидного URL (или редиректа на login).
var ErrStatusNotFound = errors.New("estado element not found")

type Fetcher struct {
	client *http.Client
	ua     string
}

func NewFetcher() *Fetcher {
	return &Fetcher{
		client: &http.Client{Timeout: DefaultTimeout},
		ua:     "aima-renew-watch-bot (+https://github.com/Self-Perfection/aima-renew-watch-bot)",
	}
}

// FetchStatus делает HTTP GET и извлекает числовой estado из
// data-return-value у #P72_ESTADO_1. Тело ответа отбрасывается —
// в памяти остаётся только число.
func (f *Fetcher) FetchStatus(ctx context.Context, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", f.ua)
	resp, err := f.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("http %d", resp.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("parse html: %w", err)
	}
	sel := doc.Find("#" + EstadoElement)
	if sel.Length() == 0 {
		return 0, ErrStatusNotFound
	}
	val, ok := sel.Attr("data-return-value")
	if !ok || val == "" {
		return 0, ErrStatusNotFound
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("invalid status %q: %w", val, err)
	}
	return n, nil
}
