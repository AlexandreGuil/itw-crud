package domain

import "time"

// SyncStateInput is the body for PUT /sync-state/:key.
type SyncStateInput struct {
	Value string `json:"value"`
}

// SyncState is the response for GET /sync-state/:key.
type SyncState struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}
