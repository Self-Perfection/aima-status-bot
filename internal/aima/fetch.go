package aima

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	DefaultTimeout = 30 * time.Second
	EstadoElement  = "P72_ESTADO_1"
)

// estadoIDRe совпадает с любым APEX-полем ESTADO, например P72_ESTADO_1
// или P5_ESTADO_2. Используется как fallback, если основной ID не нашёлся.
var estadoIDRe = regexp.MustCompile(`^P\d+_ESTADO_\d+$`)

// ErrStatusNotFound — страница загрузилась, но элемент ESTADO не нашли.
// Признак протухшего/невалидного URL (или редиректа на login).
var ErrStatusNotFound = errors.New("estado element not found")

type Fetcher struct {
	client *http.Client
	ua     string
}

func NewFetcher() *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout:   DefaultTimeout,
			Transport: newTLSTransport(DefaultTimeout),
		},
		ua: "aima-renew-watch-bot (+https://github.com/Self-Perfection/aima-renew-watch-bot)",
	}
}

// FetchStatus делает HTTP GET и извлекает числовой estado из
// data-return-value у APEX-поля ESTADO. Сначала ищет #P72_ESTADO_1,
// при неудаче — любой элемент с id вида P\d+_ESTADO_\d+. Тело ответа
// отбрасывается — в памяти остаётся только число.
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

	val, ok := extractEstado(doc)
	if !ok {
		// Элемент не нашёлся — логируем title для диагностики (не-PII).
		title := strings.TrimSpace(doc.Find("title").First().Text())
		slog.Warn("aima: estado element not found",
			"page_title", title,
			"final_url", resp.Request.URL.Host+resp.Request.URL.Path)
		return 0, ErrStatusNotFound
	}

	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("invalid status %q: %w", val, err)
	}
	return n, nil
}

// extractEstado ищет raw data-return-value сначала у #P72_ESTADO_1, потом
// у любого элемента с id вида P\d+_ESTADO_\d+. Возвращает (val, true) если
// нашёл атрибут, (_, false) если элемент или атрибут отсутствует.
func extractEstado(doc *goquery.Document) (string, bool) {
	if val, ok := readEstadoAttr(doc.Find("#" + EstadoElement)); ok {
		return val, true
	}
	var result string
	var found bool
	doc.Find("[data-return-value]").Each(func(_ int, s *goquery.Selection) {
		if found {
			return
		}
		id, _ := s.Attr("id")
		if estadoIDRe.MatchString(id) {
			if val, ok := readEstadoAttr(s); ok {
				result = val
				found = true
			}
		}
	})
	return result, found
}

func readEstadoAttr(s *goquery.Selection) (string, bool) {
	if s.Length() == 0 {
		return "", false
	}
	val, ok := s.Attr("data-return-value")
	if !ok || val == "" {
		return "", false
	}
	return val, true
}
