package pipeline

import (
	"fmt"

	"github.com/michelroberge/paulette/backend/internal/model"
)

var StageOrder = []model.StageName{
	model.StageVision,
	model.StageUX,
	model.StageArchitecture,
	model.StageBuild,
	model.StageComplete,
}

var ArtifactPaths = map[model.StageName]string{
	model.StageVision:       "vision/vision.md",
	model.StageUX:           "ux/ux.md",
	model.StageArchitecture: "architecture/architecture.md",
	model.StageBuild:        "build/build.md",
}

func stageIndex(stage model.StageName) int {
	for i, s := range StageOrder {
		if s == stage {
			return i
		}
	}
	return -1
}

// NextStage returns the stage after the given one, or an error if already complete.
func NextStage(current model.StageName) (model.StageName, error) {
	idx := stageIndex(current)
	if idx < 0 {
		return "", fmt.Errorf("unknown stage: %s", current)
	}
	if idx >= len(StageOrder)-1 {
		return "", fmt.Errorf("already at final stage: %s", current)
	}
	return StageOrder[idx+1], nil
}

// BuildPipelineState returns the full pipeline state for a project.
// activities maps stage names to their current or last-known activity (may be nil).
func BuildPipelineState(currentStage model.StageName, activities map[model.StageName]*model.StageActivity) model.PipelineState {
	currentIdx := stageIndex(currentStage)
	stages := make([]model.StageInfo, len(StageOrder))

	for i, name := range StageOrder {
		var status model.StageStatus
		switch {
		case i < currentIdx:
			status = model.StageStatusApproved
		case i == currentIdx:
			status = model.StageStatusActive
		default:
			status = model.StageStatusLocked
		}
		stages[i] = model.StageInfo{
			Name:         name,
			Status:       status,
			ArtifactPath: ArtifactPaths[name],
			Activity:     activities[name],
		}
	}

	return model.PipelineState{
		CurrentStage: currentStage,
		Stages:       stages,
	}
}
