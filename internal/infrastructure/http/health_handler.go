package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	w.Header().Set("Content-Type", "application/json")
	if s.readyFn == nil {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}
	if err := s.readyFn(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "not ready",
			"error":  err.Error(),
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}
