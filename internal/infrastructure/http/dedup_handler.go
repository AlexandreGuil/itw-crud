package http

import (
	"encoding/json"
	"net/http"

	"github.com/AlexandreGuil/itw-crud/internal/domain"
)

// handleDedupCheck handles POST /dedup/check.
func (s *Server) handleDedupCheck(w http.ResponseWriter, r *http.Request) {
	var in domain.DedupCheckInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if len(in.MD5s) == 0 {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(domain.DedupCheckResult{Seen: []string{}})
		return
	}

	seen, err := s.repo.DedupCheck(r.Context(), in.MD5s)
	if err != nil {
		s.logger.Error("dedup check", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if seen == nil {
		seen = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(domain.DedupCheckResult{Seen: seen})
}

// handleDedupMark handles POST /dedup/mark.
func (s *Server) handleDedupMark(w http.ResponseWriter, r *http.Request) {
	var in domain.DedupMarkInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	count, err := s.repo.DedupMark(r.Context(), in.URLs)
	if err != nil {
		s.logger.Error("dedup mark", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(domain.DedupMarkResult{Count: count})
}
