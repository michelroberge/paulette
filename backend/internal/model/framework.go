package model

// UXFramework identifies a supported UI framework for mock generation.
type UXFramework string

const (
	FrameworkTailwind  UXFramework = "tailwind"
	FrameworkBootstrap UXFramework = "bootstrap"
	FrameworkMUI       UXFramework = "mui"
	FrameworkShadcn    UXFramework = "shadcn"
	FrameworkVanilla   UXFramework = "vanilla"
	FrameworkOther     UXFramework = "other"
)

// FrameworkConfig is stored at .ai-factory/ux/framework.json.
type FrameworkConfig struct {
	Framework  UXFramework `json:"framework"`
	CustomName string      `json:"customName,omitempty"`
}
