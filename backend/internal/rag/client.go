package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/michelroberge/paulette/backend/internal/config"
)

// Client is an HTTP client for the R³ RAG Pipeline API.
// All methods are nil-safe: if the client is nil or disabled, they return
// (nil, nil) so callers can treat "no RAG" as "no context" without error branching.
type Client struct {
	baseURL    string
	httpClient *http.Client
	enabled    bool
	health     *HealthMonitor
}

// NewClient creates a RAG client from configuration. Returns nil if not enabled.
func NewClient(cfg config.RAGConfig) *Client {
	if !cfg.Enabled {
		log.Println("RAG integration disabled")
		return nil
	}
	c := &Client{
		baseURL: cfg.BaseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		enabled: true,
	}
	c.health = NewHealthMonitor(c, 30*time.Second)
	log.Printf("RAG integration enabled, base URL: %s", cfg.BaseURL)
	return c
}

// StartHealthMonitor starts the background health polling loop.
// Must be called after NewClient and before using IsHealthy.
func (c *Client) StartHealthMonitor(ctx context.Context) {
	if c == nil {
		return
	}
	c.health.Start(ctx)
}

// IsHealthy returns true if the RAG service is reachable.
// Lock-free via atomic.Bool in the health monitor.
func (c *Client) IsHealthy() bool {
	if c == nil {
		return false
	}
	return c.health.IsHealthy()
}

// Enabled returns true if the RAG client is configured and enabled.
func (c *Client) Enabled() bool {
	if c == nil {
		return false
	}
	return c.enabled
}

// BaseURL returns the configured RAG base URL.
func (c *Client) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.baseURL
}

// ── Internal helpers ────────────────────────────────────────────────────────

func (c *Client) doJSON(ctx context.Context, method, path string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("rag: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("rag: create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("rag: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("rag: %s %s returned %d: %s", method, path, resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("rag: decode response from %s: %w", path, err)
		}
	}
	return nil
}

// ── Health ───────────────────────────────────────────────────────────────────

// CheckHealth calls GET /health and returns the response.
func (c *Client) CheckHealth(ctx context.Context) (*HealthResponse, error) {
	if c == nil || !c.enabled {
		return nil, nil
	}
	var resp HealthResponse
	if err := c.doJSON(ctx, http.MethodGet, "/health", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ── Search ──────────────────────────────────────────────────────────────────

// Search performs a semantic search via POST /search.
func (c *Client) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	if c == nil || !c.enabled {
		return nil, nil
	}
	var resp SearchResponse
	if err := c.doJSON(ctx, http.MethodPost, "/search", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SearchCode performs a code-tuned search via POST /search/code.
func (c *Client) SearchCode(ctx context.Context, req AgentSearchRequest) (*SearchResponse, error) {
	if c == nil || !c.enabled {
		return nil, nil
	}
	var resp SearchResponse
	if err := c.doJSON(ctx, http.MethodPost, "/search/code", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SearchDocs performs a docs-tuned search via POST /search/docs.
func (c *Client) SearchDocs(ctx context.Context, req AgentSearchRequest) (*SearchResponse, error) {
	if c == nil || !c.enabled {
		return nil, nil
	}
	var resp SearchResponse
	if err := c.doJSON(ctx, http.MethodPost, "/search/docs", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ── Context ─────────────────────────────────────────────────────────────────

// BuildContext calls POST /context/build.
func (c *Client) BuildContext(ctx context.Context, req ContextBuildRequest) (*ContextBuildResponse, error) {
	if c == nil || !c.enabled {
		return nil, nil
	}
	var resp ContextBuildResponse
	if err := c.doJSON(ctx, http.MethodPost, "/context/build", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CompressContext calls POST /context/compress.
func (c *Client) CompressContext(ctx context.Context, req ContextCompressRequest) (*ContextCompressResponse, error) {
	if c == nil || !c.enabled {
		return nil, nil
	}
	var resp ContextCompressResponse
	if err := c.doJSON(ctx, http.MethodPost, "/context/compress", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ── Intent ──────────────────────────────────────────────────────────────────

// DetectIntent calls POST /intent.
func (c *Client) DetectIntent(ctx context.Context, req IntentRequest) (*IntentResponse, error) {
	if c == nil || !c.enabled {
		return nil, nil
	}
	var resp IntentResponse
	if err := c.doJSON(ctx, http.MethodPost, "/intent", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ── Ingest ──────────────────────────────────────────────────────────────────

// Ingest starts an async ingestion job via PUT /ingest.
func (c *Client) Ingest(ctx context.Context, req IngestRequest) (*IngestResponse, error) {
	if c == nil || !c.enabled {
		return nil, nil
	}
	var resp IngestResponse
	if err := c.doJSON(ctx, http.MethodPut, "/ingest", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// IngestURL starts an async URL ingestion job via PUT /ingest/url.
func (c *Client) IngestURL(ctx context.Context, req URLIngestRequest) (*IngestResponse, error) {
	if c == nil || !c.enabled {
		return nil, nil
	}
	var resp IngestResponse
	if err := c.doJSON(ctx, http.MethodPut, "/ingest/url", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// IngestStatus polls GET /ingest/{job_id}.
func (c *Client) IngestStatus(ctx context.Context, jobID string) (*IngestResponse, error) {
	if c == nil || !c.enabled {
		return nil, nil
	}
	var resp IngestResponse
	if err := c.doJSON(ctx, http.MethodGet, "/ingest/"+jobID, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ── Feedback ────────────────────────────────────────────────────────────────

// Feedback records a quality signal via POST /feedback.
func (c *Client) Feedback(ctx context.Context, req FeedbackRequest) (*FeedbackResponse, error) {
	if c == nil || !c.enabled {
		return nil, nil
	}
	var resp FeedbackResponse
	if err := c.doJSON(ctx, http.MethodPost, "/feedback", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Promote promotes a chunk via POST /promote.
func (c *Client) Promote(ctx context.Context, req PromoteRequest) (*PromoteResponse, error) {
	if c == nil || !c.enabled {
		return nil, nil
	}
	var resp PromoteResponse
	if err := c.doJSON(ctx, http.MethodPost, "/promote", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ── Metadata ────────────────────────────────────────────────────────────────

// Enrich AI-enriches chunk metadata via POST /metadata/enrich.
func (c *Client) Enrich(ctx context.Context, req EnrichRequest) (*EnrichResponse, error) {
	if c == nil || !c.enabled {
		return nil, nil
	}
	var resp EnrichResponse
	if err := c.doJSON(ctx, http.MethodPost, "/metadata/enrich", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
