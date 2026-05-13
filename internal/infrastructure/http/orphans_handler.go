package http

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) handleListOrphans(w http.ResponseWriter, r *http.Request) {
	olderThanStr := r.URL.Query().Get("older_than")
	olderThan := time.Hour
	if olderThanStr != "" {
		parsed, err := time.ParseDuration(olderThanStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid older_than duration")
			return
		}
		olderThan = parsed
	}
	urls, err := s.repo.ListOrphans(r.Context(), olderThan)
	if err != nil {
		s.logger.Error("list orphans", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"urls": urls})
}
