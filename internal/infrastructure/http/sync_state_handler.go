package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreGuil/itw-crud/internal/domain"
	"github.com/AlexandreGuil/itw-crud/internal/infrastructure/storage"
)

// handleGetSyncState handles GET /sync-state/:key.
func (s *Server) handleGetSyncState(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "key required")
		return
	}

	state, err := s.repo.GetSyncState(r.Context(), key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "sync state key not found")
			return
		}
		s.logger.Error("get sync state", "error", err, "key", key)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

// handlePutSyncState handles PUT /sync-state/:key.
func (s *Server) handlePutSyncState(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "key required")
		return
	}

	var in domain.SyncStateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if in.Value == "" {
		writeError(w, http.StatusBadRequest, "value required")
		return
	}

	if err := s.repo.SetSyncState(r.Context(), key, in.Value); err != nil {
		s.logger.Error("set sync state", "error", err, "key", key)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusOK)
}
