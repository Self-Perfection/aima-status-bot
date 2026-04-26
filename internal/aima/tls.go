package aima

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// AIMA отдаёт TLS только с листовым сертификатом, без промежуточного
// "Sectigo Public Server Authentication CA OV R36". Браузеры это
// исправляют через AIA-fetching, Go и curl — нет. Чтобы бот не падал
// с "x509: certificate signed by unknown authority", делаем два слоя:
//
//  1. Embed-fallback: вшиваем известный сейчас промежуточный.
//     Покрывает быстрый путь без сетевых запросов.
//  2. AIA-fetching: если embed не помог (промежуточный сменился),
//     при ошибке верификации читаем CA Issuers URL из листового серта,
//     качаем недостающее звено, кешируем в памяти и повторяем.
//
// Кеш — на процесс. После рестарта первая попытка снова embed; если
// AIMA починят TLS или embed станет неактуален — всё само разрулится.

//go:embed sectigo-intermediate.pem
var sectigoIntermediatePEM []byte

// newTLSTransport собирает http.RoundTripper с двойным механизмом
// верификации цепочки. timeout применяется и к dial, и к AIA-fetch'у.
func newTLSTransport(timeout time.Duration) http.RoundTripper {
	systemRoots, err := x509.SystemCertPool()
	if err != nil || systemRoots == nil {
		systemRoots = x509.NewCertPool()
	}

	v := &chainVerifier{
		roots:      systemRoots,
		aiaTimeout: timeout,
	}
	if certs, err := parseCerts(sectigoIntermediatePEM); err == nil {
		v.intermediates = append(v.intermediates, certs...)
	} else {
		// embed невалиден — это баг сборки, не runtime
		slog.Error("aima: failed to parse embedded intermediate cert", "err", err)
	}

	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: timeout,
		IdleConnTimeout:     90 * time.Second,
		TLSClientConfig: &tls.Config{
			// Стандартную верификацию выключаем — делаем её сами в
			// VerifyConnection с возможностью добить промежуточный из AIA.
			InsecureSkipVerify: true,
			VerifyConnection:   v.verify,
			ClientSessionCache: tls.NewLRUClientSessionCache(0),
		},
	}
}

type chainVerifier struct {
	roots      *x509.CertPool
	aiaTimeout time.Duration

	mu            sync.RWMutex
	intermediates []*x509.Certificate // embed + всё, что докачано через AIA
}

// verify пытается собрать цепочку: сначала против текущего пула
// промежуточных (системные корни + embed + всё что докачано раньше),
// и при провале — один раз докачивает по AIA URL из листового серта.
func (v *chainVerifier) verify(cs tls.ConnectionState) error {
	if len(cs.PeerCertificates) == 0 {
		return errors.New("tls: server presented no certificates")
	}
	leaf := cs.PeerCertificates[0]

	if err := v.tryVerify(leaf, cs.PeerCertificates[1:], cs.ServerName); err == nil {
		return nil
	}

	// Embed/кеш не помогли. Пробуем AIA-fetching.
	added, fetchErr := v.fetchAIAIntermediates(leaf)
	if fetchErr != nil {
		return fmt.Errorf("tls: chain verify failed and AIA fetch failed: %w", fetchErr)
	}
	if added == 0 {
		return errors.New("tls: chain verify failed, no AIA issuer URLs in leaf")
	}

	if err := v.tryVerify(leaf, cs.PeerCertificates[1:], cs.ServerName); err != nil {
		return fmt.Errorf("tls: chain verify failed even after AIA fetch: %w", err)
	}
	slog.Info("aima: AIA-fetched intermediate accepted (server didn't ship it)",
		"host", cs.ServerName, "added", added)
	return nil
}

func (v *chainVerifier) tryVerify(leaf *x509.Certificate, serverIntermediates []*x509.Certificate, host string) error {
	pool := x509.NewCertPool()
	for _, c := range serverIntermediates {
		pool.AddCert(c)
	}
	v.mu.RLock()
	for _, c := range v.intermediates {
		pool.AddCert(c)
	}
	v.mu.RUnlock()

	_, err := leaf.Verify(x509.VerifyOptions{
		DNSName:       host,
		Roots:         v.roots,
		Intermediates: pool,
	})
	return err
}

// fetchAIAIntermediates скачивает все сертификаты из расширения
// "CA Issuers" (AIA) листового серта и добавляет их в пул промежуточных.
// Возвращает сколько добавлено.
func (v *chainVerifier) fetchAIAIntermediates(leaf *x509.Certificate) (int, error) {
	if len(leaf.IssuingCertificateURL) == 0 {
		return 0, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), v.aiaTimeout)
	defer cancel()

	httpClient := &http.Client{Timeout: v.aiaTimeout}

	added := 0
	var lastErr error
	for _, u := range leaf.IssuingCertificateURL {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("AIA %s: http %d", u, resp.StatusCode)
			continue
		}
		certs, err := parseCerts(body)
		if err != nil || len(certs) == 0 {
			lastErr = fmt.Errorf("AIA %s: parse certs: %w", u, err)
			continue
		}
		v.mu.Lock()
		v.intermediates = append(v.intermediates, certs...)
		v.mu.Unlock()
		added += len(certs)
	}

	if added == 0 && lastErr != nil {
		return 0, lastErr
	}
	return added, nil
}

// parseCerts принимает DER (один или несколько сертификатов подряд)
// или PEM-блок(и) с сертификатами.
func parseCerts(data []byte) ([]*x509.Certificate, error) {
	if certs, err := x509.ParseCertificates(data); err == nil && len(certs) > 0 {
		return certs, nil
	}
	var out []*x509.Certificate
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, errors.New("no certificates found")
	}
	return out, nil
}
