package http

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
)

type ctxKey string

const ctxKeyClient ctxKey = "client"

func bearerAuth(logger *slog.Logger, tokens map[string]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Knative queue-proxy forwards requests from 127.0.0.1 — bypass Bearer auth.
			// The NetworkPolicy enforces which external namespaces can reach the pod;
			// localhost-only access means the request already passed Knative ingress.
			// This allows RabbitmqSource adapters (which send CloudEvents without auth)
			// to reach POST /articles and POST /translation-state. (S44 G20)
			if strings.HasPrefix(r.RemoteAddr, "127.0.0.1:") {
				ctx := context.WithValue(r.Context(), ctxKeyClient, "knative-internal")
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				logger.Warn("auth: missing or non-Bearer Authorization header", "remote", r.RemoteAddr)
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(auth, "Bearer ")
			var matchedClient string
			for client, known := range tokens {
				if subtle.ConstantTimeCompare([]byte(token), []byte(known)) == 1 {
					matchedClient = client
					break
				}
			}
			if matchedClient == "" {
				logger.Warn("auth: invalid bearer token", "remote", r.RemoteAddr)
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyClient, matchedClient)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
