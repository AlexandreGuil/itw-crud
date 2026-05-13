package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer(readyFn func(context.Context) error) *httptest.Server {
	cfg := ServerConfig{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReadinessProbe: readyFn,
		BearerTokens:   map[string]string{},
	}
	srv := NewServer(cfg)
	return httptest.NewServer(srv.handler)
}

func doGet(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(nil)
	defer srv.Close()
	resp := doGet(t, srv.URL+"/healthz")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status=%d, want 200", resp.StatusCode)
	}
}

func TestReadyz_NoBackend(t *testing.T) {
	srv := newTestServer(nil)
	defer srv.Close()
	resp := doGet(t, srv.URL+"/readyz")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status=%d, want 200", resp.StatusCode)
	}
}

func TestReadyz_BackendDown(t *testing.T) {
	srv := newTestServer(func(_ context.Context) error { return errors.New("pg down") })
	defer srv.Close()
	resp := doGet(t, srv.URL+"/readyz")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status=%d, want 503", resp.StatusCode)
	}
}
