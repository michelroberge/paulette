package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// SourceRef is a reference to a RAG chunk used as context.
type SourceRef struct {
	ChunkID    string `json:"chunk_id"`
	FilePath   string `json:"file_path,omitempty"`
	Collection string `json:"collection,omitempty"`
	Score      float64 `json:"score"`
	Preview    string `json:"preview,omitempty"`
}

// StageContextConfig holds per-stage defaults for RAG context retrieval.
type StageContextConfig struct {
	Collections []string
	Groups      []string
	MaxChunks   int
	ChunkTypes  []string
	Boost       map[string]any
}

// defaultStageContext maps each pipeline stage to its RAG retrieval defaults.
var defaultStageContext = map[model.StageName]StageContextConfig{
	model.StageVision: {
		Collections: []string{"docs", "specs"},
		Groups:      []string{"patterns", "constraints"},
		MaxChunks:   6,
	},
	model.StageUX: {
		Collections: []string{"docs", "specs"},
		Groups:      []string{"examples", "patterns"},
		MaxChunks:   8,
	},
	model.StageArchitecture: {
		Collections: []string{"code", "docs", "specs"},
		Groups:      []string{"examples", "patterns", "constraints"},
		MaxChunks:   12,
		Boost:       map[string]any{"_promoted": true},
	},
	model.StageBuild: {
		Collections: []string{"code", "docs"},
		Groups:      []string{"examples", "patterns"},
		MaxChunks:   8,
	},
}

// maxContextChars is the rough character budget for RAG context (~2000 tokens × 4 chars).
const maxContextChars = 8000

// BuildStageContext retrieves relevant knowledge from the RAG pipeline for a
// given pipeline stage and user query. Returns a formatted context string ready
// for system prompt injection and a list of source references.
//
// Returns ("", nil, nil) when the client is nil, unhealthy, or no results are found.
func BuildStageContext(ctx context.Context, client *Client, stage model.StageName, query string, projectName string) (string, []SourceRef, error) {
	if client == nil || !client.IsHealthy() {
		return "", nil, nil
	}

	cfg, ok := defaultStageContext[stage]
	if !ok {
		return "", nil, nil
	}

	strategy := &ContextStrategy{
		Groups:    cfg.Groups,
		MaxChunks: cfg.MaxChunks,
		Diversity: true,
		Rerank:    true,
	}
	if len(cfg.ChunkTypes) > 0 {
		strategy.ChunkTypes = cfg.ChunkTypes
	}
	if cfg.Boost != nil {
		strategy.Boost = cfg.Boost
	}

	resp, err := client.BuildContext(ctx, ContextBuildRequest{
		Query:       query,
		Collections: cfg.Collections,
		Strategy:    strategy,
	})
	if err != nil {
		return "", nil, fmt.Errorf("rag context build: %w", err)
	}
	if resp == nil || len(resp.ContextBlocks) == 0 {
		return "", nil, nil
	}

	// If the response is too large, compress it.
	contextText := resp.FinalPromptContext
	if len(contextText) > maxContextChars {
		compressed, compErr := client.CompressContext(ctx, ContextCompressRequest{
			Context:   contextText,
			Strategy:  "summarize",
			MaxTokens: 1500,
		})
		if compErr == nil && compressed != nil && compressed.CompressedContext != "" {
			contextText = compressed.CompressedContext
		}
	}

	// Collect source references for frontend display.
	var sources []SourceRef
	for _, block := range resp.ContextBlocks {
		for _, chunk := range block.Chunks {
			ref := SourceRef{
				ChunkID:    chunk.ChunkID,
				Collection: block.Type,
				Score:      chunk.Score,
			}
			if chunk.Metadata != nil {
				if fp, ok := chunk.Metadata["file_path"].(string); ok {
					ref.FilePath = fp
				}
			}
			// Short preview for frontend display.
			preview := chunk.Content
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			ref.Preview = preview
			sources = append(sources, ref)
		}
	}

	formatted := formatRAGContext(contextText, sources)
	return formatted, sources, nil
}

// BuildBeadContext retrieves relevant code patterns and documentation for
// bead execution. Uses code-tuned and docs-tuned search endpoints.
func BuildBeadContext(ctx context.Context, client *Client, query string, projectName string) (string, []SourceRef, error) {
	if client == nil || !client.IsHealthy() {
		return "", nil, nil
	}

	limit := 6
	codeResp, err := client.SearchCode(ctx, AgentSearchRequest{
		Query:       query,
		Collections: []string{"code"},
		Limit:       limit,
	})
	if err != nil {
		return "", nil, fmt.Errorf("rag bead code search: %w", err)
	}

	docsResp, err := client.SearchDocs(ctx, AgentSearchRequest{
		Query:       query,
		Collections: []string{"docs", "specs"},
		Limit:       4,
	})
	if err != nil {
		return "", nil, fmt.Errorf("rag bead docs search: %w", err)
	}

	var sb strings.Builder
	var sources []SourceRef

	if codeResp != nil && len(codeResp.Results) > 0 {
		sb.WriteString("[Code Patterns]\n")
		for _, r := range codeResp.Results {
			filePath := ""
			if r.Metadata != nil {
				if fp, ok := r.Metadata["file_path"].(string); ok {
					filePath = fp
				}
			}
			if filePath != "" {
				sb.WriteString(fmt.Sprintf("# Source: %s\n", filePath))
			}
			sb.WriteString(r.Content)
			sb.WriteString("\n\n")

			preview := r.Content
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			sources = append(sources, SourceRef{
				ChunkID:    r.ChunkID,
				FilePath:   filePath,
				Collection: "code",
				Score:      r.Score,
				Preview:    preview,
			})
		}
	}

	if docsResp != nil && len(docsResp.Results) > 0 {
		sb.WriteString("[Documentation]\n")
		for _, r := range docsResp.Results {
			filePath := ""
			if r.Metadata != nil {
				if fp, ok := r.Metadata["file_path"].(string); ok {
					filePath = fp
				}
			}
			if filePath != "" {
				sb.WriteString(fmt.Sprintf("# Source: %s\n", filePath))
			}
			sb.WriteString(r.Content)
			sb.WriteString("\n\n")

			preview := r.Content
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			sources = append(sources, SourceRef{
				ChunkID:    r.ChunkID,
				FilePath:   filePath,
				Collection: "docs",
				Score:      r.Score,
				Preview:    preview,
			})
		}
	}

	if sb.Len() == 0 {
		return "", nil, nil
	}

	formatted := formatRAGContext(sb.String(), sources)
	return formatted, sources, nil
}

func formatRAGContext(contextText string, sources []SourceRef) string {
	var sb strings.Builder
	sb.WriteString("--- RAG KNOWLEDGE CONTEXT ---\n")
	sb.WriteString("The following is relevant knowledge retrieved from past projects and approved patterns.\n")
	sb.WriteString("Use this as reference material but always defer to the current project's approved artifacts.\n\n")
	sb.WriteString(contextText)

	if len(sources) > 0 {
		sb.WriteString("\n\nSources: ")
		refs := make([]string, 0, len(sources))
		for _, s := range sources {
			if s.FilePath != "" {
				refs = append(refs, fmt.Sprintf("%s (%s)", s.ChunkID, s.FilePath))
			} else {
				refs = append(refs, s.ChunkID)
			}
		}
		sb.WriteString(strings.Join(refs, ", "))
	}
	sb.WriteString("\n--- END RAG KNOWLEDGE CONTEXT ---")
	return sb.String()
}
