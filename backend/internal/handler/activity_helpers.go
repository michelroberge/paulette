package handler

import (
	"log"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/repository"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

// writeActivity records a running operation for a stage in activity.json.
func writeActivity(repo repository.ActivityRepo, hostDir string, stage model.StageName, op string) {
	if err := repo.SetActivity(hostDir, stage, &model.StageActivity{
		Operation: op,
		Status:    "running",
		StartedAt: time.Now(),
	}); err != nil {
		log.Printf("writeActivity(%s, %s): %v", stage, op, err)
	}
}

// clearActivity removes the activity entry for a stage (run succeeded).
func clearActivity(repo repository.ActivityRepo, hostDir string, stage model.StageName) {
	if err := repo.ClearActivity(hostDir, stage); err != nil {
		log.Printf("clearActivity(%s): %v", stage, err)
	}
}

// failActivity marks a stage operation as failed.
func failActivity(repo repository.ActivityRepo, hostDir string, stage model.StageName, op, errMsg string) {
	if err := repo.SetActivity(hostDir, stage, &model.StageActivity{
		Operation: op,
		Status:    "failed",
		StartedAt: time.Now(),
		Error:     errMsg,
	}); err != nil {
		log.Printf("failActivity(%s, %s): %v", stage, op, err)
	}
}

// mergeActivities merges live in-memory runs with persisted activities.
// In-memory runs take precedence because a running process is authoritative.
func mergeActivities(liveRuns []stream.RunInfo, persisted map[model.StageName]*model.StageActivity) map[model.StageName]*model.StageActivity {
	result := make(map[model.StageName]*model.StageActivity, len(persisted))
	for k, v := range persisted {
		result[k] = v
	}
	for _, r := range liveRuns {
		t, _ := time.Parse(time.RFC3339, r.StartedAt)
		stage := model.StageName(r.Stage)
		result[stage] = &model.StageActivity{
			Operation: r.Operation,
			Status:    "running",
			StartedAt: t,
		}
	}
	return result
}
