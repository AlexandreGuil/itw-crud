package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/AlexandreGuil/itw-crud/internal/domain"
	"github.com/AlexandreGuil/itw-crud/internal/infrastructure/storage"
)

// handleWriteTranslationState (S44 Phase 2 new endpoint).
// Body: TranslationResponseInput from translator-agent v3.0 via RabbitmqSource.
// request_id parsed for md5_url, UPDATE by md5_url.
func (s *Server) handleWriteTranslationState(w http.ResponseWriter, r *http.Request) {
	var in domain.TranslationResponseInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if in.RequestID == "" {
		writeError(w, http.StatusBadRequest, "request_id required")
		return
	}

	if err := s.repo.WriteTranslationState(r.Context(), in); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "article not found for request_id")
			return
		}
		s.logger.Error("write translation state", "error", err, "request_id", in.RequestID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePatchTranslationState(w http.ResponseWriter, r *http.Request) {
	ifMatchHeader := r.Header.Get("If-Match")
	if ifMatchHeader == "" {
		writeError(w, http.StatusPreconditionRequired, "If-Match header required")
		return
	}
	versionStr := strings.Trim(ifMatchHeader, `"`)
	ifMatch, err := strconv.Atoi(versionStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "If-Match header must be quoted integer (ETag)")
		return
	}
	url, err := decodeURLParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid url_b64")
		return
	}
	var in domain.PatchTranslationStateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	newVersion, err := s.repo.PatchTranslationState(r.Context(), url, ifMatch, in)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrVersionMismatch), strings.Contains(err.Error(), "version mismatch"):
			writeError(w, http.StatusPreconditionFailed, "ETag mismatch — refetch and retry")
			return
		case errors.Is(err, storage.ErrNotFound), strings.Contains(err.Error(), "not found"):
			writeError(w, http.StatusNotFound, "article not found")
			return
		}
		s.logger.Error("patch translation state", "error", err, "url", url)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("ETag", `"`+strconv.Itoa(newVersion)+`"`)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"url": url, "version": newVersion})
}
