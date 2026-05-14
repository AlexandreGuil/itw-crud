package http

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerAuth_MissingHeader_Returns401(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mw := bearerAuth(logger, map[string]string{"cron": "secret-1"})
	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })
	handler := mw(next)
	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401", rec.Code)
	}
	if called {
		t.Error("next handler should not be called when auth fails")
	}
}

func TestBearerAuth_BadToken_Returns401(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mw := bearerAuth(logger, map[string]string{"cron": "secret-1"})
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
	handler := mw(next)
	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d", rec.Code)
	}
}

func TestBearerAuth_LocalhostBypass_PassesThrough(t *testing.T) {
	// Knative queue-proxy forwards from 127.0.0.1 — must bypass Bearer auth (S44 G20).
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mw := bearerAuth(logger, map[string]string{"cron": "secret-1"})
	var clientID string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		clientID, _ = r.Context().Value(ctxKeyClient).(string)
	})
	handler := mw(next)
	req := httptest.NewRequest(http.MethodPost, "/articles", nil)
	req.RemoteAddr = "127.0.0.1:54321" // simulates Knative queue-proxy forwarding
	// No Authorization header — should still pass through
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d, want 200 for localhost bypass", rec.Code)
	}
	if clientID != "knative-internal" {
		t.Errorf("clientID=%q, want knative-internal", clientID)
	}
}

func TestBearerAuth_ValidToken_PassesThrough(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mw := bearerAuth(logger, map[string]string{"cron": "secret-1"})
	called := false
	var clientID string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		clientID, _ = r.Context().Value(ctxKeyClient).(string)
	})
	handler := mw(next)
	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	req.Header.Set("Authorization", "Bearer secret-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !called {
		t.Error("next handler should be called when auth succeeds")
	}
	if clientID != "cron" {
		t.Errorf("clientID=%q, want cron", clientID)
	}
}
