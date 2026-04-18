package rag

import (
	"context"
	"fmt"
	"log"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// RecordPositiveOutcome sends positive feedback signals for chunks associated
// with a bead that passed code review on the first attempt.
func RecordPositiveOutcome(ctx context.Context, client *Client, project *model.Project, bead model.Bead) error {
	if client == nil || !client.IsHealthy() {
		return nil
	}

	// Search for recently ingested chunks from this bead's target files.
	for _, targetFile := range bead.TargetFiles {
		results, err := client.SearchCode(ctx, AgentSearchRequest{
			Query:       targetFile,
			Collections: []string{"code"},
			Limit:       3,
		})
		if err != nil || results == nil {
			continue
		}

		for _, r := range results.Results {
			if fp, ok := r.Metadata["file_path"].(string); ok && fp == targetFile {
				// Record positive signal.
				if _, err := client.Feedback(ctx, FeedbackRequest{
					ChunkID:    r.ChunkID,
					Signal:     "high_quality",
					Source:     "paulette_code_review",
					Collection: "code",
				}); err != nil {
					log.Printf("RAG feedback failed for chunk %s: %v", r.ChunkID, err)
				}
			}
		}
	}

	log.Printf("RAG positive feedback recorded for bead %s/%s", project.Name, bead.ID)
	return nil
}

// RecordNegativeOutcome sends negative feedback signals when a bead's code
// is rejected by the Devil's Advocate reviewer.
func RecordNegativeOutcome(ctx context.Context, client *Client, project *model.Project, bead model.Bead, findings string) error {
	if client == nil || !client.IsHealthy() {
		return nil
	}

	for _, targetFile := range bead.TargetFiles {
		results, err := client.SearchCode(ctx, AgentSearchRequest{
			Query:       targetFile,
			Collections: []string{"code"},
			Limit:       3,
		})
		if err != nil || results == nil {
			continue
		}

		for _, r := range results.Results {
			if fp, ok := r.Metadata["file_path"].(string); ok && fp == targetFile {
				if _, err := client.Feedback(ctx, FeedbackRequest{
					ChunkID:    r.ChunkID,
					Signal:     "not_useful",
					Source:     "paulette_code_review",
					Collection: "code",
				}); err != nil {
					log.Printf("RAG negative feedback failed for chunk %s: %v", r.ChunkID, err)
				}
			}
		}
	}

	log.Printf("RAG negative feedback recorded for bead %s/%s", project.Name, bead.ID)
	return nil
}

// PromoteProjectArtifacts promotes all artifact chunks for a completed project
// as golden patterns, boosting their retrieval score for future projects.
func PromoteProjectArtifacts(ctx context.Context, client *Client, project *model.Project) error {
	if client == nil || !client.IsHealthy() {
		return nil
	}

	// Search for all chunks tagged with this project's artifact metadata.
	for _, collection := range []string{"specs", "docs"} {
		results, err := client.Search(ctx, SearchRequest{
			Query:       fmt.Sprintf("%s artifact", project.Name),
			Collections: []string{collection},
			Filters: map[string]any{
				"paulette_project_id": project.ID,
				"type":                "artifact",
			},
			Limit: 50,
		})
		if err != nil || results == nil {
			continue
		}

		for _, r := range results.Results {
			if _, err := client.Promote(ctx, PromoteRequest{
				ChunkID:    r.ChunkID,
				Target:     "golden_patterns",
				Collection: collection,
			}); err != nil {
				log.Printf("RAG promote failed for chunk %s: %v", r.ChunkID, err)
			}
		}
	}

	log.Printf("RAG artifacts promoted for completed project %s", project.Name)
	return nil
}
