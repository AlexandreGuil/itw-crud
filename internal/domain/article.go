// Package domain holds the entities and repository contracts that itw-crud manages.
package domain

import "time"

// Article is a row from article_records as returned by GET /articles/{url}.
type Article struct {
	ArticleID     string     `json:"article_id"`
	URL           string     `json:"url"`
	MD5URL        string     `json:"md5_url"`
	Title         string     `json:"title"`
	FinalDecision string     `json:"final_decision"`
	FinalScore    *float64   `json:"final_score,omitempty"`
	IngestedAt    *time.Time `json:"ingested_at,omitempty"`
	Tags          *string    `json:"tags,omitempty"`
	Source        *string    `json:"source,omitempty"`
	Content       *string    `json:"content,omitempty"`
	Summary       *string    `json:"summary,omitempty"`
	TitleFR               *string    `json:"title_fr,omitempty"`
	SummaryFR             *string    `json:"summary_fr,omitempty"`
	TranslationModel      *string    `json:"translation_model,omitempty"`
	TranslationTokensIn   *int       `json:"translation_tokens_input,omitempty"`
	TranslationTokensOut  *int       `json:"translation_tokens_output,omitempty"`
	TranslationDurationMs *int64     `json:"translation_duration_ms,omitempty"`
	TranslatedAt          *time.Time `json:"translated_at,omitempty"`
	ReadwiseID            *string    `json:"readwise_id,omitempty"`
	ReaderPushedAt        *time.Time `json:"reader_pushed_at,omitempty"`
	// S43 new
	ReaderPayloadPendingAt *time.Time `json:"reader_payload_pending_at,omitempty"`
	ReaderTags             []string   `json:"reader_tags"`
	Version                int        `json:"version"`
}

// SetReaderPayloadInput is the payload received on POST /articles.
// Sets reader_payload_pending_at + reader_tags on an EXISTING row identified by URL.
// (Rows are created by the ITW cron via record_run, not by itw-crud.)
type SetReaderPayloadInput struct {
	URL        string   `json:"url"`
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
	TranslationDurationMs *int64  `json:"translation_duration_ms,omitempty"`
	MarkTranslated        bool    `json:"mark_translated,omitempty"`
	ReadwiseID            *string `json:"readwise_id,omitempty"`
	MarkPushedToReader    bool    `json:"mark_pushed_to_reader,omitempty"`
}
