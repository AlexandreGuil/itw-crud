package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreGuil/itw-crud/internal/domain"
)

type fakeRepo struct {
	mu       sync.Mutex
	articles map[string]*domain.Article
	orphans  []string
	pingErr  error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{articles: map[string]*domain.Article{}}
}

func (f *fakeRepo) CreateArticle(_ context.Context, in domain.CreateArticleInput) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.articles[in.URL]; ok {
		return 0, errors.New("conflict")
	}
	now := time.Now()
	f.articles[in.URL] = &domain.Article{
		URL: in.URL, TitleVO: in.TitleVO, Content: in.Content, Summary: in.Summary,
		Axes: in.Axes, Tags: in.Tags, SourceURL: in.SourceURL,
		ReaderTags: in.ReaderTags, ReaderPayloadPendingAt: &now, Version: 1,
	}
	return 1, nil
}

func (f *fakeRepo) GetArticleByURL(_ context.Context, url string) (*domain.Article, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.articles[url]
	if !ok {
		return nil, errors.New("not found")
	}
	return a, nil
}

func (f *fakeRepo) PatchTranslationState(_ context.Context, url string, ifMatch int, in domain.PatchTranslationStateInput) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.articles[url]
	if !ok {
		return 0, errors.New("not found")
	}
	if a.Version != ifMatch {
		return 0, errors.New("version mismatch")
	}
	if in.TitleFR != nil {
		a.TitleFR = in.TitleFR
	}
	if in.SummaryFR != nil {
		a.SummaryFR = in.SummaryFR
	}
	if in.MarkTranslated {
		now := time.Now()
		a.TranslatedAt = &now
	}
	if in.ReadwiseID != nil {
		a.ReadwiseID = in.ReadwiseID
	}
	if in.MarkPushedToReader {
		now := time.Now()
		a.ReaderPushedAt = &now
	}
	a.Version++
	return a.Version, nil
}

func (f *fakeRepo) ListOrphans(_ context.Context, _ time.Duration) ([]string, error) {
	return f.orphans, nil
}

func (f *fakeRepo) Ping(_ context.Context) error { return f.pingErr }

func newTestServerWithRepo(repo Repository) *httptest.Server {
	cfg := ServerConfig{
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		BearerTokens: map[string]string{"test": "token-1"},
		Repo:         repo,
	}
	srv := NewServer(cfg)
	return httptest.NewServer(srv.handler)
}

func b64(url string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(url))
}

func TestPostArticle_Success(t *testing.T) {
	repo := newFakeRepo()
	srv := newTestServerWithRepo(repo)
	defer srv.Close()
	body, _ := json.Marshal(domain.CreateArticleInput{
		URL: "https://example.com/foo", TitleVO: "Hello",
		ReaderTags: []string{"axis:ai", "source:rss", "veille-validee"},
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/articles", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-1")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if etag := resp.Header.Get("ETag"); etag != `"1"` {
		t.Errorf("etag=%q", etag)
	}
}

func TestPostArticle_NoAuth_Returns401(t *testing.T) {
	repo := newFakeRepo()
	srv := newTestServerWithRepo(repo)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/articles", bytes.NewReader([]byte(`{}`)))
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status=%d", resp.StatusCode)
	}
}

func TestGetArticle_Success(t *testing.T) {
	repo := newFakeRepo()
	_, _ = repo.CreateArticle(context.Background(), domain.CreateArticleInput{URL: "https://e/x", TitleVO: "T"})
	srv := newTestServerWithRepo(repo)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/articles/"+b64("https://e/x"), nil)
	req.Header.Set("Authorization", "Bearer token-1")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if etag := resp.Header.Get("ETag"); etag != `"1"` {
		t.Errorf("etag=%q", etag)
	}
	var got domain.Article
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got.TitleVO != "T" {
		t.Errorf("title_vo=%q", got.TitleVO)
	}
}

func TestGetArticle_NotFound_Returns404(t *testing.T) {
	srv := newTestServerWithRepo(newFakeRepo())
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/articles/"+b64("https://nope"), nil)
	req.Header.Set("Authorization", "Bearer token-1")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status=%d", resp.StatusCode)
	}
}

func TestPatchTranslationState_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	_, _ = repo.CreateArticle(context.Background(), domain.CreateArticleInput{URL: "https://e/x", TitleVO: "T"})
	srv := newTestServerWithRepo(repo)
	defer srv.Close()
	titleFR := "Titre FR"
	body, _ := json.Marshal(domain.PatchTranslationStateInput{TitleFR: &titleFR, MarkTranslated: true})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/translation-state/"+b64("https://e/x"), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", `"1"`)
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if etag := resp.Header.Get("ETag"); etag != `"2"` {
		t.Errorf("etag=%q, want \"2\"", etag)
	}
}

func TestPatchTranslationState_MissingIfMatch_Returns428(t *testing.T) {
	repo := newFakeRepo()
	_, _ = repo.CreateArticle(context.Background(), domain.CreateArticleInput{URL: "https://e/x", TitleVO: "T"})
	srv := newTestServerWithRepo(repo)
	defer srv.Close()
	body, _ := json.Marshal(domain.PatchTranslationStateInput{})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/translation-state/"+b64("https://e/x"), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-1")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionRequired {
		t.Errorf("status=%d, want 428", resp.StatusCode)
	}
}

func TestPatchTranslationState_StaleIfMatch_Returns412(t *testing.T) {
	repo := newFakeRepo()
	_, _ = repo.CreateArticle(context.Background(), domain.CreateArticleInput{URL: "https://e/x", TitleVO: "T"})
	titleFR := "first"
	_, _ = repo.PatchTranslationState(context.Background(), "https://e/x", 1, domain.PatchTranslationStateInput{TitleFR: &titleFR})
	srv := newTestServerWithRepo(repo)
	defer srv.Close()
	body, _ := json.Marshal(domain.PatchTranslationStateInput{TitleFR: &titleFR})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/translation-state/"+b64("https://e/x"), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-1")
	req.Header.Set("If-Match", `"1"`)
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Errorf("status=%d, want 412", resp.StatusCode)
	}
}

func TestListOrphans_Success(t *testing.T) {
	repo := newFakeRepo()
	repo.orphans = []string{"https://e/orphan1", "https://e/orphan2"}
	srv := newTestServerWithRepo(repo)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/articles/orphans?older_than=1h", nil)
	req.Header.Set("Authorization", "Bearer token-1")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var got struct {
		Urls []string `json:"urls"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if len(got.Urls) != 2 {
		t.Errorf("urls=%v", got.Urls)
	}
}
