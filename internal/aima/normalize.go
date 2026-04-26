package aima

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

const (
	ValidacaoHost   = "portal-renovacoes.aima.gov.pt"
	ValidacaoPrefix = "/ords/r/aima/aima-pr/validar"
)

// IsValidacaoURL проверяет, что URL указывает на страницу Validação
// AIMA. Не валидирует токен — это делает сам fetch.
func IsValidacaoURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return false
	}
	if !strings.EqualFold(u.Host, ValidacaoHost) {
		return false
	}
	return strings.HasPrefix(u.Path, ValidacaoPrefix)
}

// NormalizeURL приводит URL к каноническому виду для дедупа: lowercase
// scheme/host, drop fragment. Порядок query-параметров НЕ меняется —
// APEX использует параметр cs (checksum), привязанный к конкретному
// порядку параметров; сортировка сломала бы чексумму и запрос.
// Возвращает каноничный URL и его SHA-256 hex.
func NormalizeURL(raw string) (canonical, hash string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("url missing scheme or host")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	// RawQuery оставляем как есть — порядок параметров важен для cs.
	canonical = u.String()
	sum := sha256.Sum256([]byte(canonical))
	hash = hex.EncodeToString(sum[:])
	return canonical, hash, nil
}
