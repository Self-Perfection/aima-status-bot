package aima

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
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
	// CookieJar нужен: APEX при первом запросе ставит сессионную куку и
	// редиректит. Без jar Go следует редиректу без куки и получает пустую
	// форму вместо страницы со статусом.
	jar, _ := cookiejar.New(nil)
	return &Fetcher{
		client: &http.Client{
			Timeout:   DefaultTimeout,
			Transport: newTLSTransport(DefaultTimeout),
			Jar:       jar,
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
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return 0, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("http %d (body %d bytes)", resp.StatusCode, len(body))
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("parse html: %w", err)
	}

	val, ok := extractEstado(doc)
	if !ok {
		// Элемент не нашёлся — логируем title и размер для диагностики (не-PII).
		title := strings.TrimSpace(doc.Find("title").First().Text())
		if isGeoblock(body) {
			slog.Warn("aima: geo-blocked — server is not in Portugal",
				"http_status", resp.StatusCode,
				"body_bytes", len(body),
				"final_url", resp.Request.URL.Host+resp.Request.URL.Path)
		} else {
			slog.Warn("aima: estado element not found",
				"page_title", title,
				"http_status", resp.StatusCode,
				"body_bytes", len(body),
				"final_url", resp.Request.URL.Host+resp.Request.URL.Path)
		}
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

// isGeoblock возвращает true, если тело ответа содержит признак
// геоблокировки AIMA («доступ только из Португалии»).
func isGeoblock(body []byte) bool {
	return bytes.Contains(body, []byte("Acesso permitido apenas em Portugal"))
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
