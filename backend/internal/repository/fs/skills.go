package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// SkillRepo manages reusable skills stored at the installation level (~/.paulette/skills/).
type SkillRepo struct {
	mu      sync.RWMutex
	baseDir string // e.g. ~/.paulette/skills
}

func NewSkillRepo(registryPath string) *SkillRepo {
	return &SkillRepo{baseDir: filepath.Join(registryPath, "skills")}
}

// List returns all skills in the library.
func (r *SkillRepo) List() ([]model.Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries, err := os.ReadDir(r.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var skills []model.Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, err := r.readSkill(e.Name())
		if err != nil {
			continue
		}
		skills = append(skills, *s)
	}
	return skills, nil
}

// Get returns a skill and its prompt template.
func (r *SkillRepo) Get(id string) (*model.Skill, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, err := r.readSkill(id)
	if err != nil {
		return nil, "", err
	}
	promptPath := filepath.Join(r.baseDir, id, "prompt.md")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		return s, "", nil // skill exists but no prompt template
	}
	return s, string(data), nil
}

// Create writes a new skill to disk.
func (r *SkillRepo) Create(skill *model.Skill, promptTemplate string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dir := filepath.Join(r.baseDir, skill.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}

	skill.Version = 1
	skill.CreatedAt = time.Now()
	skill.UpdatedAt = skill.CreatedAt

	if err := r.writeSkill(skill); err != nil {
		return err
	}

	promptPath := filepath.Join(dir, "prompt.md")
	return os.WriteFile(promptPath, []byte(promptTemplate), 0644)
}

// Search returns skills matching the given category and/or tags.
func (r *SkillRepo) Search(category string, tags []string) ([]model.Skill, error) {
	all, err := r.List()
	if err != nil {
		return nil, err
	}

	var matched []model.Skill
	for _, s := range all {
		if category != "" && s.Category != category {
			continue
		}
		if len(tags) > 0 && !hasOverlap(s.Tags, tags) {
			continue
		}
		matched = append(matched, s)
	}
	return matched, nil
}

func (r *SkillRepo) readSkill(id string) (*model.Skill, error) {
	p := filepath.Join(r.baseDir, id, "skill.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var s model.Skill
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SkillRepo) writeSkill(s *model.Skill) error {
	dir := filepath.Join(r.baseDir, s.ID)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "skill.json"), data, 0644)
}

// Slugify converts a name to a slug suitable for directory names.
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// CopyToProject copies a skill into the project's .paulette/skills/ directory
// so agents working in that repo have local access without external paths.
func (r *SkillRepo) CopyToProject(hostDir string, skillID string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skill, err := r.readSkill(skillID)
	if err != nil {
		return fmt.Errorf("read skill %s: %w", skillID, err)
	}

	promptPath := filepath.Join(r.baseDir, skillID, "prompt.md")
	promptData, _ := os.ReadFile(promptPath)

	projectSkillDir := filepath.Join(hostDir, ".paulette", "skills", skillID)
	if err := os.MkdirAll(projectSkillDir, 0755); err != nil {
		return fmt.Errorf("create project skill dir: %w", err)
	}

	data, err := json.MarshalIndent(skill, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(projectSkillDir, "skill.json"), data, 0644); err != nil {
		return err
	}
	if len(promptData) > 0 {
		if err := os.WriteFile(filepath.Join(projectSkillDir, "prompt.md"), promptData, 0644); err != nil {
			return err
		}
	}
	return nil
}

// ReadProjectSkills reads skills that have been copied into a project's .paulette/skills/.
func ReadProjectSkills(hostDir string) ([]model.Skill, error) {
	dir := filepath.Join(hostDir, ".paulette", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var skills []model.Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name(), "skill.json")
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var s model.Skill
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		skills = append(skills, s)
	}
	return skills, nil
}

// ReadProjectSkillPrompt reads a skill's prompt template from the project copy.
func ReadProjectSkillPrompt(hostDir, skillID string) (string, error) {
	p := filepath.Join(hostDir, ".paulette", "skills", skillID, "prompt.md")
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func hasOverlap(a, b []string) bool {
	set := make(map[string]bool, len(a))
	for _, v := range a {
		set[v] = true
	}
	for _, v := range b {
		if set[v] {
			return true
		}
	}
	return false
}

// ReadSkillSuggestions loads pending suggestions from a project's build directory.
func ReadSkillSuggestions(hostDir string) (*model.SkillSuggestions, error) {
	p := filepath.Join(hostDir, ".paulette", "build", "skill-suggestions.json")
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &model.SkillSuggestions{}, nil
		}
		return nil, err
	}
	var suggestions model.SkillSuggestions
	if err := json.Unmarshal(data, &suggestions); err != nil {
		return nil, err
	}
	return &suggestions, nil
}

// WriteSkillSuggestions saves pending suggestions to a project's build directory.
func WriteSkillSuggestions(hostDir string, suggestions *model.SkillSuggestions) error {
	dir := filepath.Join(hostDir, ".paulette", "build")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(suggestions, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "skill-suggestions.json"), data, 0644)
}

// ReadObservedSkills loads observer-detected suggestions.
func ReadObservedSkills(hostDir string) (*model.SkillSuggestions, error) {
	p := filepath.Join(hostDir, ".paulette", "build", "observed-skills.json")
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &model.SkillSuggestions{}, nil
		}
		return nil, err
	}
	var suggestions model.SkillSuggestions
	if err := json.Unmarshal(data, &suggestions); err != nil {
		return nil, err
	}
	return &suggestions, nil
}

// WriteObservedSkills saves observer-detected suggestions.
func WriteObservedSkills(hostDir string, suggestions *model.SkillSuggestions) error {
	dir := filepath.Join(hostDir, ".paulette", "build")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(suggestions, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "observed-skills.json"), data, 0644)
}
