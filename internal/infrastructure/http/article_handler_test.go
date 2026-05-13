package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreGuil/itw-crud/internal/domain"
	"github.com/AlexandreGuil/itw-crud/internal/infrastructure/storage"
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

// seedArticle inserts a minimal article (simulating ITW cron record_run).
func (f *fakeRepo) seedArticle(url string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.articles[url] = &domain.Article{
		URL: url, Title: "", FinalDecision: "", Version: 1,
	}
}

func (f *fakeRepo) SetReaderPayload(_ context.Context, in domain.SetReaderPayloadInput) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.articles[in.URL]
	if !ok {
		return 0, storage.ErrNotFound
	}
	now := time.Now()
	a.ReaderTags = in.ReaderTags
	a.ReaderPayloadPendingAt = &now
	a.Version++
	return a.Version, nil
}

func (f *fakeRepo) GetArticleByURL(_ context.Context, url string) (*domain.Article, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.articles[url]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return a, nil
}

func (f *fakeRepo) PatchTranslationState(_ context.Context, url string, ifMatch int, in domain.PatchTranslationStateInput) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.articles[url]
	if !ok {
		return 0, storage.ErrNotFound
	}
	if a.Version != ifMatch {
		return 0, storage.ErrVersionMismatch
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
	// Seed the article (row created by ITW cron before itw-crud is called).
	repo.seedArticle("https://example.com/foo")
	srv := newTestServerWithRepo(repo)
	defer srv.Close()
	body, _ := json.Marshal(domain.SetReaderPayloadInput{
		URL:        "https://example.com/foo",
		ReaderTags: []string{"axis:ai", "source:rss", "veille-validee"},
	})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/articles", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-1")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if etag := resp.Header.Get("ETag"); etag != `"2"` {
		t.Errorf("etag=%q, want \"2\"", etag)
	}
}

func TestPostArticle_NotFound_Returns404(t *testing.T) {
	repo := newFakeRepo()
	srv := newTestServerWithRepo(repo)
	defer srv.Close()
	body, _ := json.Marshal(domain.SetReaderPayloadInput{
		URL:        "https://example.com/unknown",
		ReaderTags: []string{"veille-validee"},
	})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/articles", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-1")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status=%d, want 404", resp.StatusCode)
	}
}

func TestPostArticle_NoAuth_Returns401(t *testing.T) {
	repo := newFakeRepo()
	srv := newTestServerWithRepo(repo)
	defer srv.Close()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/articles", bytes.NewReader([]byte(`{}`)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status=%d", resp.StatusCode)
	}
}

func TestGetArticle_Success(t *testing.T) {
	repo := newFakeRepo()
	repo.seedArticle("https://e/x")
	repo.articles["https://e/x"].Title = "T"
	srv := newTestServerWithRepo(repo)
	defer srv.Close()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/articles/"+b64("https://e/x"), nil)
	req.Header.Set("Authorization", "Bearer token-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if etag := resp.Header.Get("ETag"); etag != `"1"` {
		t.Errorf("etag=%q", etag)
	}
	var got domain.Article
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got.Title != "T" {
		t.Errorf("title=%q", got.Title)
	}
}

func TestGetArticle_NotFound_Returns404(t *testing.T) {
	srv := newTestServerWithRepo(newFakeRepo())
	defer srv.Close()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/articles/"+b64("https://nope"), nil)
	req.Header.Set("Authorization", "Bearer token-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status=%d", resp.StatusCode)
	}
}

func TestPatchTranslationState_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	repo.seedArticle("https://e/x")
	srv := newTestServerWithRepo(repo)
	defer srv.Close()
	titleFR := "Titre FR"
	body, _ := json.Marshal(domain.PatchTranslationStateInput{TitleFR: &titleFR, MarkTranslated: true})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPatch, srv.URL+"/translation-state/"+b64("https://e/x"), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", `"1"`)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
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
	repo.seedArticle("https://e/x")
	srv := newTestServerWithRepo(repo)
	defer srv.Close()
	body, _ := json.Marshal(domain.PatchTranslationStateInput{})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPatch, srv.URL+"/translation-state/"+b64("https://e/x"), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionRequired {
		t.Errorf("status=%d, want 428", resp.StatusCode)
	}
}

func TestPatchTranslationState_StaleIfMatch_Returns412(t *testing.T) {
	repo := newFakeRepo()
	repo.seedArticle("https://e/x")
	titleFR := "first"
	_, _ = repo.PatchTranslationState(context.Background(), "https://e/x", 1, domain.PatchTranslationStateInput{TitleFR: &titleFR})
	srv := newTestServerWithRepo(repo)
	defer srv.Close()
	body, _ := json.Marshal(domain.PatchTranslationStateInput{TitleFR: &titleFR})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPatch, srv.URL+"/translation-state/"+b64("https://e/x"), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-1")
	req.Header.Set("If-Match", `"1"`)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
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
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/articles/orphans?older_than=1h", nil)
	req.Header.Set("Authorization", "Bearer token-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
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
