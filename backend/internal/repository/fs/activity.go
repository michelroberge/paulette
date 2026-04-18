package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
)

const activityFile = "activity.json"

// activityMu serialises all reads and writes to activity.json on a per-dataDir basis.
var activityMu sync.Map // key: dataDir → *sync.Mutex

func activityLock(dataDir string) *sync.Mutex {
	v, _ := activityMu.LoadOrStore(dataDir, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func activityPath(dataDir string) string {
	return filepath.Join(dataDir, activityFile)
}

// ActivityRepo persists per-stage activity state to {dataDir}/activity.json.
type ActivityRepo struct{}

func NewActivityRepo() *ActivityRepo {
	return &ActivityRepo{}
}

// ReadActivity returns all stage activities for a project. Returns an empty map if none exist.
func (r *ActivityRepo) ReadActivity(dataDir string) (map[model.StageName]*model.StageActivity, error) {
	mu := activityLock(dataDir)
	mu.Lock()
	defer mu.Unlock()
	return readActivityLocked(dataDir)
}

func readActivityLocked(dataDir string) (map[model.StageName]*model.StageActivity, error) {
	data, err := os.ReadFile(activityPath(dataDir))
	if os.IsNotExist(err) {
		return make(map[model.StageName]*model.StageActivity), nil
	}
	if err != nil {
		return nil, err
	}
	var m map[model.StageName]*model.StageActivity
	if err := json.Unmarshal(data, &m); err != nil {
		return make(map[model.StageName]*model.StageActivity), nil
	}
	if m == nil {
		m = make(map[model.StageName]*model.StageActivity)
	}
	return m, nil
}

func writeActivityLocked(dataDir string, m map[model.StageName]*model.StageActivity) error {
	if err := os.MkdirAll(filepath.Dir(activityPath(dataDir)), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(activityPath(dataDir), data, 0644)
}

// SetActivity writes or replaces the activity for a single stage.
func (r *ActivityRepo) SetActivity(dataDir string, stage model.StageName, a *model.StageActivity) error {
	mu := activityLock(dataDir)
	mu.Lock()
	defer mu.Unlock()
	m, _ := readActivityLocked(dataDir)
	m[stage] = a
	return writeActivityLocked(dataDir, m)
}

// ClearActivity removes the activity entry for a stage.
func (r *ActivityRepo) ClearActivity(dataDir string, stage model.StageName) error {
	mu := activityLock(dataDir)
	mu.Lock()
	defer mu.Unlock()
	m, _ := readActivityLocked(dataDir)
	if _, exists := m[stage]; !exists {
		return nil // nothing to do
	}
	delete(m, stage)
	return writeActivityLocked(dataDir, m)
}

// AppendBtw appends a btw message to the pending queue for a stage.
func (r *ActivityRepo) AppendBtw(dataDir string, stage model.StageName, message string, sentAt time.Time) error {
	mu := activityLock(dataDir)
	mu.Lock()
	defer mu.Unlock()
	m, _ := readActivityLocked(dataDir)
	a := m[stage]
	if a == nil {
		// Activity may not exist yet if the run just started; create a placeholder
		a = &model.StageActivity{}
		m[stage] = a
	}
	a.PendingBtw = append(a.PendingBtw, model.BtwMessage{Message: message, SentAt: sentAt})
	return writeActivityLocked(dataDir, m)
}

// ClearBtw drains all pending btw messages for a stage and returns them.
func (r *ActivityRepo) ClearBtw(dataDir string, stage model.StageName) ([]model.BtwMessage, error) {
	mu := activityLock(dataDir)
	mu.Lock()
	defer mu.Unlock()
	m, _ := readActivityLocked(dataDir)
	a := m[stage]
	if a == nil || len(a.PendingBtw) == 0 {
		return nil, nil
	}
	msgs := a.PendingBtw
	a.PendingBtw = nil
	if err := writeActivityLocked(dataDir, m); err != nil {
		return nil, err
	}
	return msgs, nil
}
