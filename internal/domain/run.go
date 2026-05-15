package domain

import "time"

// CreateRunInput is the body for POST /runs.
type CreateRunInput struct {
	RunID      string     `json:"run_id"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	SourceType string     `json:"source_type"`
	Mode       string     `json:"mode,omitempty"`
}

// PatchRunInput is the body for PATCH /runs/:run_id.
type PatchRunInput struct {
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	ArticlesCount *int       `json:"articles_count,omitempty"`
}

// PipelineRun is the response for POST /runs.
type PipelineRun struct {
	RunID     string    `json:"run_id"`
	StartedAt time.Time `json:"started_at"`
}
