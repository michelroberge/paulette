package handler

import (
	"time"

	"github.com/google/uuid"

	"github.com/michelroberge/paulette/backend/internal/model"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
)

// recordSession writes a session record to the project's session log.
// Call this from deferred token-flush functions in handlers.
func recordSession(dataDir string, stage model.StageName, kind model.SessionKind, iteration int, startedAt time.Time, totalTokens int) {
	if totalTokens <= 0 {
		return
	}
	sw := fsrepo.GetSessionWriter(dataDir)
	sw.Append(model.Session{
		ID:          uuid.NewString(),
		Stage:       stage,
		Kind:        kind,
		Iteration:   iteration,
		StartedAt:   startedAt,
		EndedAt:     time.Now(),
		TotalTokens: totalTokens,
	})
}
