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
