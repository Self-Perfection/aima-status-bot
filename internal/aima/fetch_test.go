package aima

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestFetchStatus_OK(t *testing.T) {
	html := `<html><body><input id="P72_ESTADO_1" data-return-value="14"/></body></html>`
	srv := newServer(t, 200, html)
	defer srv.Close()

	got, err := NewFetcher().FetchStatus(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got != 14 {
		t.Errorf("status = %d, want 14", got)
	}
}

func TestFetchStatus_MissingElement(t *testing.T) {
	srv := newServer(t, 200, `<html><body>nope</body></html>`)
	defer srv.Close()

	_, err := NewFetcher().FetchStatus(context.Background(), srv.URL)
	if !errors.Is(err, ErrStatusNotFound) {
		t.Errorf("err = %v, want ErrStatusNotFound", err)
	}
}

func TestFetchStatus_MissingAttribute(t *testing.T) {
	srv := newServer(t, 200, `<html><body><input id="P72_ESTADO_1"/></body></html>`)
	defer srv.Close()

	_, err := NewFetcher().FetchStatus(context.Background(), srv.URL)
	if !errors.Is(err, ErrStatusNotFound) {
		t.Errorf("err = %v, want ErrStatusNotFound", err)
	}
}

func TestFetchStatus_NonNumeric(t *testing.T) {
	srv := newServer(t, 200, `<html><body><input id="P72_ESTADO_1" data-return-value="abc"/></body></html>`)
	defer srv.Close()

	_, err := NewFetcher().FetchStatus(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for non-numeric status")
	}
	if errors.Is(err, ErrStatusNotFound) {
		t.Errorf("got ErrStatusNotFound, want parse error")
	}
}

func TestFetchStatus_HTTP500(t *testing.T) {
	srv := newServer(t, 500, "")
	defer srv.Close()

	_, err := NewFetcher().FetchStatus(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}
