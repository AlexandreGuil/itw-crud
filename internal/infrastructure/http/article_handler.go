package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreGuil/itw-crud/internal/domain"
	"github.com/AlexandreGuil/itw-crud/internal/infrastructure/storage"
)

func (s *Server) handleCreateArticle(w http.ResponseWriter, r *http.Request) {
	var in domain.CreateArticleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if in.URL == "" {
		writeError(w, http.StatusBadRequest, "url required")
		return
	}
	version, err := s.repo.CreateArticle(r.Context(), in)
	if err != nil {
		if errors.Is(err, storage.ErrConflict) {
			writeError(w, http.StatusConflict, "article already exists")
			return
		}
		s.logger.Error("create article", "error", err, "url", in.URL)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("ETag", `"`+strconv.Itoa(version)+`"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
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
		if errors.Is(err, storage.ErrNotFound) || strings.Contains(err.Error(), "not found") {
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
