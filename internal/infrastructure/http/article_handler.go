package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreGuil/itw-crud/internal/domain"
	"github.com/AlexandreGuil/itw-crud/internal/infrastructure/storage"
)

// handleUpsertArticle reads UpsertArticleInput JSON body, calls repo.UpsertArticle,
// returns 200 + ETag on success.
// Supports both plain JSON (Content-Type: application/json) and CloudEvent structured mode
// (Content-Type: application/cloudevents+json) sent by Knative RabbitmqSource.
func (s *Server) handleUpsertArticle(w http.ResponseWriter, r *http.Request) {
	var in domain.UpsertArticleInput

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
	if in.URL == "" {
		writeError(w, http.StatusBadRequest, "url required")
		return
	}
	if in.MD5URL == "" {
		writeError(w, http.StatusBadRequest, "md5_url required")
		return
	}
	if in.ArticleID == "" {
		writeError(w, http.StatusBadRequest, "article_id required")
		return
	}
	if in.RunID == "" {
		writeError(w, http.StatusBadRequest, "run_id required")
		return
	}

	version, err := s.repo.UpsertArticle(r.Context(), in)
	if err != nil {
		s.logger.Error("upsert article", "error", err, "url", in.URL)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("ETag", `"`+strconv.Itoa(version)+`"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"url": in.URL, "version": version})
}

func (s *Server) handleGetArticle(w http.ResponseWriter, r *http.Request) {
	url, err := decodeURLParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid url_b64")
		return
	}
	article, err := s.repo.GetArticleByURL(r.Context(), url)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "article not found")
			return
		}
		s.logger.Error("get article", "error", err, "url", url)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("ETag", `"`+strconv.Itoa(article.Version)+`"`)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(article)
}

func decodeURLParam(r *http.Request) (string, error) {
	b64param := chi.URLParam(r, "url_b64")
	raw, err := base64.RawURLEncoding.DecodeString(b64param)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	return string(raw), nil
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
