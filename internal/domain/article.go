// Package domain holds the entities and repository contracts that itw-crud manages.
package domain

import "time"

// Article is the full record stored in PG article_records table, as returned by GET /articles/{url}.
type Article struct {
	URL                    string     `json:"url"`
	MD5URL                 string     `json:"md5_url"`
	TitleVO                string     `json:"title_vo"`
	Content                string     `json:"content"`
	Summary                string     `json:"summary"`
	Axes                   []string   `json:"axes"`
	Tags                   []string   `json:"tags"`
	SourceURL              string     `json:"source_url"`
	FinalScore             *float64   `json:"final_score,omitempty"`
	FinalDecision          string     `json:"final_decision"`
	IngestedAt             *time.Time `json:"ingested_at,omitempty"`
	// S39 P4 translation state
	TitleFR               *string    `json:"title_fr,omitempty"`
	SummaryFR             *string    `json:"summary_fr,omitempty"`
	TranslationModel      *string    `json:"translation_model,omitempty"`
	TranslationTokensIn   *int       `json:"translation_tokens_input,omitempty"`
	TranslationTokensOut  *int       `json:"translation_tokens_output,omitempty"`
	TranslationDurationMs *int       `json:"translation_duration_ms,omitempty"`
	TranslatedAt          *time.Time `json:"translated_at,omitempty"`
	ReadwiseID            *string    `json:"readwise_id,omitempty"`
	ReaderPushedAt        *time.Time `json:"reader_pushed_at,omitempty"`
	// S43 new — itw-crud
	ReaderPayloadPendingAt *time.Time `json:"reader_payload_pending_at,omitempty"`
	ReaderTags             []string   `json:"reader_tags"`
	Version                int        `json:"version"`
}

// CreateArticleInput is the payload received on POST /articles.
type CreateArticleInput struct {
	URL        string   `json:"url"`
	TitleVO    string   `json:"title_vo"`
	Content    string   `json:"content"`
	Summary    string   `json:"summary"`
	Axes       []string `json:"axes"`
	Tags       []string `json:"tags"`
	SourceURL  string   `json:"source_url"`
	ReaderTags []string `json:"reader_tags"`
}

// PatchTranslationStateInput is the payload received on PATCH /translation-state/{url}.
// All fields optional — column-level update of the present ones only.
type PatchTranslationStateInput struct {
	TitleFR               *string `json:"title_fr,omitempty"`
	SummaryFR             *string `json:"summary_fr,omitempty"`
	TranslationModel      *string `json:"translation_model,omitempty"`
	TranslationTokensIn   *int    `json:"translation_tokens_input,omitempty"`
	TranslationTokensOut  *int    `json:"translation_tokens_output,omitempty"`
	TranslationDurationMs *int    `json:"translation_duration_ms,omitempty"`
	MarkTranslated        bool    `json:"mark_translated,omitempty"`
	ReadwiseID            *string `json:"readwise_id,omitempty"`
	MarkPushedToReader    bool    `json:"mark_pushed_to_reader,omitempty"`
}
