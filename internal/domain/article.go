// Package domain holds the entities and repository contracts that itw-crud manages.
package domain

import "time"

// UpsertArticleInput is the payload received on POST /articles (S44 Phase 2).
// Full article payload from AMQP RabbitmqSource (cron Python publish to itw.articles exchange).
// Idempotent UPSERT keyed by md5_url.
type UpsertArticleInput struct {
	URL           string     `json:"url"`
	MD5URL        string     `json:"md5_url"`
	ArticleID     string     `json:"article_id"`
	RunID         string     `json:"run_id"`
	Title         string     `json:"title"`
	Content       string     `json:"content"`
	Summary       string     `json:"summary"`
	Tags          string     `json:"tags"`
	Source        string     `json:"source"`
	SourceURL     string     `json:"source_url"`
	Author        string     `json:"author,omitempty"`
	PublishedDate *time.Time `json:"published_date,omitempty"`
	WordCount     int        `json:"word_count,omitempty"`
	Axes          []string   `json:"axes"`
	ReaderTags    []string   `json:"reader_tags"`
	FinalDecision string     `json:"final_decision"`
	FinalScore    *float64   `json:"final_score,omitempty"`
	Decisions     string     `json:"decisions,omitempty"` // JSONB serialized
	IngestedAt    *time.Time `json:"ingested_at,omitempty"`
}

// Article is a row from article_records as returned by GET /articles/{url}.
type Article struct {
	ArticleID             string     `json:"article_id"`
	URL                   string     `json:"url"`
	MD5URL                string     `json:"md5_url"`
	Title                 string     `json:"title"`
	FinalDecision         string     `json:"final_decision"`
	FinalScore            *float64   `json:"final_score,omitempty"`
	IngestedAt            *time.Time `json:"ingested_at,omitempty"`
	Tags                  *string    `json:"tags,omitempty"`
	Source                *string    `json:"source,omitempty"`
	Content               *string    `json:"content,omitempty"`
	Summary               *string    `json:"summary,omitempty"`
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

// TranslationResponseInput is the body received on POST /translation-state (S44).
// Published by translator-agent v3.0 to translation.responses exchange.
// request_id format: "<source>:<url_md5>" — parse <url_md5> for UPDATE WHERE md5_url=...
type TranslationResponseInput struct {
	RequestID             string  `json:"request_id"`
	Status                string  `json:"status"`
	SourceLanguage        string  `json:"source_language"`
	TargetLanguage        string  `json:"target_language"`
	TitleFR               *string `json:"title_fr,omitempty"`
	SummaryFR             *string `json:"summary_fr,omitempty"`
	TranslationModel      *string `json:"model,omitempty"`
	TranslationTokensIn   *int    `json:"tokens_input,omitempty"`
	TranslationTokensOut  *int    `json:"tokens_output,omitempty"`
	TranslationDurationMs *int64  `json:"duration_ms,omitempty"`
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
