package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreGuil/itw-crud/internal/domain"
	"github.com/AlexandreGuil/itw-crud/internal/infrastructure/storage"
)

// handleCreateRun handles POST /runs.
func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var in domain.CreateRunInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if in.RunID == "" {
		writeError(w, http.StatusBadRequest, "run_id required")
		return
	}
	if in.SourceType == "" {
		writeError(w, http.StatusBadRequest, "source_type required")
		return
	}

	run, err := s.repo.CreateRun(r.Context(), in)
	if err != nil {
		s.logger.Error("create run", "error", err, "run_id", in.RunID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(run)
}

// handlePatchRun handles PATCH /runs/:run_id.
func (s *Server) handlePatchRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "run_id")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "run_id required")
		return
	}

	var in domain.PatchRunInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	if err := s.repo.PatchRun(r.Context(), runID, in); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		s.logger.Error("patch run", "error", err, "run_id", runID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusOK)
}
