package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreGuil/itw-crud/internal/infrastructure/storage"
)

func (s *Server) handleGetArticleByMD5(w http.ResponseWriter, r *http.Request) {
	md5Hex := chi.URLParam(r, "md5_url_hex")
	if md5Hex == "" {
		writeError(w, http.StatusBadRequest, "md5_url_hex required")
		return
	}

	article, err := s.repo.GetArticleByMD5(r.Context(), md5Hex)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "article not found")
			return
		}
		s.logger.Error("get article by md5", "error", err, "md5", md5Hex)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("ETag", `"`+strconv.Itoa(article.Version)+`"`)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(article)
}
