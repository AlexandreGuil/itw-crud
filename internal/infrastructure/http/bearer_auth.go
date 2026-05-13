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
