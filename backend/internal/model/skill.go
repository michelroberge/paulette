package model

import "time"

// Skill is a reusable, parameterized prompt template stored at the installation level.
type Skill struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Category    string       `json:"category"` // backend, frontend, devops, testing
	Tags        []string     `json:"tags"`
	Version     int          `json:"version"`
	Parameters  []SkillParam `json:"parameters,omitempty"`
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
	CreatedBy   string       `json:"createdBy"` // project name that spawned it
}

// SkillParam describes a named parameter in a skill's prompt template.
type SkillParam struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Default     string `json:"default,omitempty"`
}

// SkillSuggestion is a candidate skill identified by analysis (pre-phase or observer).
type SkillSuggestion struct {
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	Category       string       `json:"category"`
	Tags           []string     `json:"tags"`
	Parameters     []SkillParam `json:"parameters,omitempty"`
	PromptTemplate string       `json:"promptTemplate"`
	Approved       bool         `json:"approved"`
	SourceBeads    []string     `json:"sourceBeads,omitempty"` // bead IDs that inspired the pattern (observer)
}

// SkillSuggestions is the on-disk container for pending suggestions.
type SkillSuggestions struct {
	Suggestions []SkillSuggestion `json:"suggestions"`
}
