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
// Supports both plain JSON (Content-Type: application/json) and CloudEvent structured mode
// (Content-Type: application/cloudevents+json) sent by Knative RabbitmqSource.
func (s *Server) handleWriteTranslationState(w http.ResponseWriter, r *http.Request) {
	var in domain.TranslationResponseInput

	if data, err := cloudEventData(r); err != nil {
		writeError(w, http.StatusBadRequest, "invalid cloudevent body")
		return
	} else if data != nil {
		if err := json.Unmarshal(data, &in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid cloudevent data field")
			return
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
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

	// S45 — publish push-ready trigger so reader-pusher is triggered AFTER
	// translation state is persisted in PG (sequential guarantee, no race condition).
	// Only on status="ok"; skipped/failed translations don't need a Reader push.
	if s.publisher != nil && in.Status == "ok" {
		source := parseSource(in.RequestID)
		routingKey := source + ".article.push-ready"
		triggerBody := []byte(`{"request_id":"` + in.RequestID + `"}`)
		if pubErr := s.publisher.Publish(
			r.Context(),
			"itw.articles",
			routingKey,
			triggerBody,
			map[string]any{"content-type": "application/json"},
		); pubErr != nil {
			// Non-fatal: log warning, don't fail the HTTP response.
			// reader-pusher can recover via orphan sweeper if needed.
			s.logger.Warn("amqp push trigger failed", "error", pubErr, "request_id", in.RequestID)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// parseSource extracts the source prefix from a request_id "<source>:<md5>".
func parseSource(requestID string) string {
	idx := strings.IndexByte(requestID, ':')
	if idx < 0 {
		return "unknown"
	}
	return requestID[:idx]
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
