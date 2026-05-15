package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/AlexandreGuil/itw-crud/internal/domain"
)

func TestPostRuns_Success(t *testing.T) {
	repo := newFakeRepo()
	srv := newTestServerWithRepo(repo)
	defer srv.Close()

	now := time.Now().UTC()
	body, _ := json.Marshal(domain.CreateRunInput{
		RunID:      "run-handler-001",
		StartedAt:  &now,
		SourceType: "rss",
		Mode:       "normal",
	})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-1")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status=%d, want 201", resp.StatusCode)
	}
	var got domain.PipelineRun
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got.RunID != "run-handler-001" {
		t.Errorf("run_id=%q, want run-handler-001", got.RunID)
	}
}

func TestPostRuns_MissingRunID_Returns400(t *testing.T) {
	repo := newFakeRepo()
	srv := newTestServerWithRepo(repo)
	defer srv.Close()

	body, _ := json.Marshal(domain.CreateRunInput{SourceType: "rss"})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-1")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", resp.StatusCode)
	}
}

func TestPostRuns_MissingSourceType_Returns400(t *testing.T) {
	repo := newFakeRepo()
	srv := newTestServerWithRepo(repo)
	defer srv.Close()

	body, _ := json.Marshal(domain.CreateRunInput{RunID: "run-handler-002"})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-1")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", resp.StatusCode)
	}
}

func TestPatchRuns_Success(t *testing.T) {
	repo := newFakeRepo()
	srv := newTestServerWithRepo(repo)
	defer srv.Close()

	count := 7
	body, _ := json.Marshal(domain.PatchRunInput{ArticlesCount: &count})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPatch, srv.URL+"/runs/run-exists", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-1")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
}

func TestPatchRuns_NotFound_Returns404(t *testing.T) {
	repo := newFakeRepo()
	srv := newTestServerWithRepo(repo)
	defer srv.Close()

	count := 3
	body, _ := json.Marshal(domain.PatchRunInput{ArticlesCount: &count})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPatch, srv.URL+"/runs/nonexistent", bytes.NewReader(body))
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
