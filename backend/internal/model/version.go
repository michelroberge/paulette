package model

import "time"

// VersionMeta records the git commit and metadata for a completed, approved iteration.
// Written to docs/{version}/version-meta.json during summary approval.
type VersionMeta struct {
	Version    string    `json:"version"`
	Iteration  int       `json:"iteration"`
	CommitHash string    `json:"commitHash"`
	TagName    string    `json:"tagName"`
	ApprovedAt time.Time `json:"approvedAt"`
}
