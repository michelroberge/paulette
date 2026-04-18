package rag

// Go structs mirroring the R³ RAG Pipeline OpenAPI schema.
// Only types that Paulette actually uses are defined here.

// ── Health ──────────────────────────────────────────────────────────────────

// HealthResponse is the response from GET /health.
type HealthResponse struct {
	Status string `json:"status"` // "healthy", "degraded", "demo"
	Qdrant string `json:"qdrant,omitempty"`
	Ollama string `json:"ollama,omitempty"`
}

// ── Search ──────────────────────────────────────────────────────────────────

// SearchRequest is the body for POST /search.
type SearchRequest struct {
	Query          string         `json:"query"`
	Filters        map[string]any `json:"filters,omitempty"`
	Limit          int            `json:"limit,omitempty"`
	ScoreThreshold *float64       `json:"score_threshold,omitempty"`
	Collection     string         `json:"collection,omitempty"`
	Collections    []string       `json:"collections,omitempty"`
	Rewrite        bool           `json:"rewrite,omitempty"`
	Hybrid         bool           `json:"hybrid,omitempty"`
	HybridWeight   float64        `json:"hybrid_weight,omitempty"`
	Offset         int            `json:"offset,omitempty"`
	FilterMode     string         `json:"filter_mode,omitempty"`
	Debug          bool           `json:"debug,omitempty"`
}

// SearchResult represents a single chunk result.
type SearchResult struct {
	ChunkID        string         `json:"chunk_id"`
	Score          float64        `json:"score"`
	Content        string         `json:"content"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	ScoreBreakdown map[string]any `json:"score_breakdown,omitempty"`
}

// SearchResponse is the response from POST /search.
type SearchResponse struct {
	Results    []SearchResult `json:"results"`
	Query      string         `json:"query"`
	Total      int            `json:"total"`
	NextCursor *string        `json:"next_cursor,omitempty"`
	DebugInfo  map[string]any `json:"debug_info,omitempty"`
}

// AgentSearchRequest is the body for POST /search/code and /search/docs.
type AgentSearchRequest struct {
	Query          string         `json:"query"`
	Filters        map[string]any `json:"filters,omitempty"`
	Limit          int            `json:"limit,omitempty"`
	Collections    []string       `json:"collections,omitempty"`
	ScoreThreshold *float64       `json:"score_threshold,omitempty"`
}

// ── Ask (RAG Q&A) ──────────────────────────────────────────────────────────

// AskRequest is the body for POST /ask.
type AskRequest struct {
	Question       string         `json:"question"`
	Filters        map[string]any `json:"filters,omitempty"`
	ContextLimit   int            `json:"context_limit,omitempty"`
	Model          string         `json:"model,omitempty"`
	Collection     string         `json:"collection,omitempty"`
	Collections    []string       `json:"collections,omitempty"`
	Rewrite        bool           `json:"rewrite,omitempty"`
	Hybrid         bool           `json:"hybrid,omitempty"`
	HybridWeight   float64        `json:"hybrid_weight,omitempty"`
	ScoreThreshold *float64       `json:"score_threshold,omitempty"`
	FilterMode     string         `json:"filter_mode,omitempty"`
}

// SourceReference is a source chunk reference in an AskResponse.
type SourceReference struct {
	ChunkID  string  `json:"chunk_id"`
	FilePath string  `json:"file_path,omitempty"`
	Score    float64 `json:"score"`
}

// AskResponse is the response from POST /ask.
type AskResponse struct {
	Answer  string            `json:"answer"`
	Sources []SourceReference `json:"sources"`
	Model   string            `json:"model"`
}

// ── Context ─────────────────────────────────────────────────────────────────

// ContextStrategy controls retrieval knobs for /context/build.
type ContextStrategy struct {
	Groups         []string       `json:"groups,omitempty"`
	MaxChunks      int            `json:"max_chunks,omitempty"`
	Diversity      bool           `json:"diversity,omitempty"`
	Rerank         bool           `json:"rerank,omitempty"`
	ChunkTypes     []string       `json:"chunk_types,omitempty"`
	Boost          map[string]any `json:"boost,omitempty"`
	FreshnessBoost bool           `json:"freshness_boost,omitempty"`
}

// ContextBuildRequest is the body for POST /context/build.
type ContextBuildRequest struct {
	Query       string          `json:"query"`
	Collections []string        `json:"collections,omitempty"`
	Filters     map[string]any  `json:"filters,omitempty"`
	Strategy    *ContextStrategy `json:"strategy,omitempty"`
	Debug       bool            `json:"debug,omitempty"`
}

// ContextBlock represents one group's retrieval results.
type ContextBlock struct {
	Type   string         `json:"type"`
	Chunks []SearchResult `json:"chunks"`
}

// ContextBuildResponse is the response from POST /context/build.
type ContextBuildResponse struct {
	ContextBlocks      []ContextBlock `json:"context_blocks"`
	FinalPromptContext string         `json:"final_prompt_context"`
}

// ContextCompressRequest is the body for POST /context/compress.
type ContextCompressRequest struct {
	Chunks    []SearchResult `json:"chunks,omitempty"`
	Context   string         `json:"context,omitempty"`
	Strategy  string         `json:"strategy,omitempty"` // summarize, deduplicate, group_merge, merge
	MaxTokens int            `json:"max_tokens,omitempty"`
	Model     string         `json:"model,omitempty"`
}

// ContextCompressResponse is the response from POST /context/compress.
type ContextCompressResponse struct {
	CompressedContext  string `json:"compressed_context"`
	OriginalChunkCount int    `json:"original_chunk_count"`
	StrategyApplied    string `json:"strategy_applied"`
}

// ── Intent ──────────────────────────────────────────────────────────────────

// IntentRequest is the body for POST /intent.
type IntentRequest struct {
	Content string `json:"content"`
	Context string `json:"context,omitempty"`
}

// IntentResponse is the response from POST /intent.
type IntentResponse struct {
	IntentCategory   string         `json:"intent_category"`
	Confidence       string         `json:"confidence"`
	SemanticTopics   []string       `json:"semantic_topics"`
	SuggestedFilters map[string]any `json:"suggested_filters,omitempty"`
	SuggestedQuery   *string        `json:"suggested_query,omitempty"`
	Reasoning        *string        `json:"reasoning,omitempty"`
}

// ── Ingest ──────────────────────────────────────────────────────────────────

// IngestOptions controls ingestion behaviour.
type IngestOptions struct {
	Recursive           *bool    `json:"recursive,omitempty"`
	MaxDepth            int      `json:"max_depth,omitempty"`
	ExcludePatterns     []string `json:"exclude_patterns,omitempty"`
	ForceReindex        bool     `json:"force_reindex,omitempty"`
	Collection          string   `json:"collection,omitempty"`
	EnableRouting       *bool    `json:"enable_routing,omitempty"`
	EnableAIRouting     *bool    `json:"enable_ai_routing,omitempty"`
	AIRoutingThreshold  *float64 `json:"ai_routing_threshold,omitempty"`
}

// IngestRequest is the body for PUT /ingest.
type IngestRequest struct {
	Path     string         `json:"path"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Options  *IngestOptions `json:"options,omitempty"`
}

// URLIngestRequest is the body for PUT /ingest/url.
type URLIngestRequest struct {
	URLs     []string       `json:"urls"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Options  *IngestOptions `json:"options,omitempty"`
}

// FileResult describes per-file ingestion results.
type FileResult struct {
	Path               string   `json:"path"`
	Chunks             int      `json:"chunks"`
	Status             string   `json:"status"` // ok, skipped, error
	Error              string   `json:"error,omitempty"`
	Collection         string   `json:"collection,omitempty"`
	RoutingConfidence  *float64 `json:"routing_confidence,omitempty"`
	AIValidated        bool     `json:"ai_validated"`
	SemanticTags       []string `json:"semantic_tags,omitempty"`
}

// IngestResponse is the response from PUT /ingest and GET /ingest/{job_id}.
type IngestResponse struct {
	JobID            string       `json:"job_id"`
	Status           string       `json:"status"` // pending, processing, completed, failed
	IsDryRun         bool         `json:"is_dry_run"`
	FilesDiscovered  int          `json:"files_discovered"`
	FilesProcessed   int          `json:"files_processed"`
	FilesSkipped     int          `json:"files_skipped"`
	TotalChunks      int          `json:"total_chunks"`
	CollectionsUsed  []string     `json:"collections_used"`
	AIValidatedCount int          `json:"ai_validated_count"`
	Errors           []string     `json:"errors,omitempty"`
	Details          []FileResult `json:"details,omitempty"`
	CurrentStep      *string      `json:"current_step,omitempty"`
}

// ── Feedback ────────────────────────────────────────────────────────────────

// FeedbackRequest is the body for POST /feedback.
type FeedbackRequest struct {
	ChunkID    string `json:"chunk_id"`
	Signal     string `json:"signal"` // useful, not_useful, high_quality
	Source     string `json:"source,omitempty"`
	Collection string `json:"collection,omitempty"`
}

// FeedbackResponse is the response from POST /feedback.
type FeedbackResponse struct {
	ChunkID        string `json:"chunk_id"`
	Signal         string `json:"signal"`
	TotalUseful    int    `json:"total_useful"`
	TotalNotUseful int    `json:"total_not_useful"`
}

// PromoteRequest is the body for POST /promote.
type PromoteRequest struct {
	ChunkID    string `json:"chunk_id"`
	Target     string `json:"target,omitempty"` // default: golden_patterns
	Collection string `json:"collection,omitempty"`
}

// PromoteResponse is the response from POST /promote.
type PromoteResponse struct {
	ChunkID    string `json:"chunk_id"`
	PromotedTo string `json:"promoted_to"`
	Status     string `json:"status"`
}

// ── Metadata ────────────────────────────────────────────────────────────────

// EnrichRequest is the body for POST /metadata/enrich.
type EnrichRequest struct {
	ChunkID    string   `json:"chunk_id"`
	Collection string   `json:"collection,omitempty"`
	Infer      []string `json:"infer,omitempty"` // topic, quality, pattern
	AddTags    bool     `json:"add_tags,omitempty"`
}

// EnrichResponse is the response from POST /metadata/enrich.
type EnrichResponse struct {
	ChunkID   string         `json:"chunk_id"`
	Inferred  map[string]any `json:"inferred"`
	TagsAdded []string       `json:"tags_added,omitempty"`
}

// ── Collections ─────────────────────────────────────────────────────────────

// CollectionStats is the response from GET /collections/{name}/stats.
type CollectionStats struct {
	Name         string `json:"name"`
	PointsCount  int    `json:"points_count"`
	VectorsCount int    `json:"vectors_count"`
	Status       string `json:"status"`
}
