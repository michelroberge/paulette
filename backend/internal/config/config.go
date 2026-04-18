package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// OIDCConfig holds optional OIDC authentication settings loaded from env vars.
// When Enabled is false, all fields are ignored and no auth is enforced.
type OIDCConfig struct {
	Enabled       bool
	IssuerURL     string
	ClientID      string
	ClientSecret  string
	RedirectURI   string
	Scopes        []string // defaults to ["openid", "profile", "email"]
	SessionSecret string
}

// RAGConfig holds optional R³ RAG Pipeline integration settings.
// When Enabled is false the RAG client is not created and all RAG
// features degrade gracefully to no-ops.
type RAGConfig struct {
	Enabled bool
	BaseURL string
}

type Config struct {
	Port         int
	RegistryPath string
	GitPath      string // where git repos live (~/.paulette/git by default)
	DataPath     string // where paulette working state lives (~/.paulette/repos); derived from RegistryPath
	ClaudePath   string
	Version      string
	Author       string
	OIDC         OIDCConfig
	RAG          RAGConfig
}

// Settings represents user-persisted configuration saved by "paulette init".
type Settings struct {
	GitPath    string `json:"gitPath,omitempty"`
	ReposPath  string `json:"reposPath,omitempty"` // deprecated: read as fallback for GitPath
	ClaudePath string `json:"claudePath,omitempty"`
}

// SettingsPath returns the path to the settings file.
func SettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".paulette", "settings.json")
}

// LoadSettings reads ~/.paulette/settings.json (returns zero value if missing).
func LoadSettings() Settings {
	var s Settings
	data, err := os.ReadFile(SettingsPath())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	return s
}

// SaveSettings writes settings to ~/.paulette/settings.json.
func SaveSettings(s Settings) error {
	path := SettingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func Load() *Config {
	port := 8080
	if p, err := strconv.Atoi(os.Getenv("PORT")); err == nil {
		port = p
	}

	home, _ := os.UserHomeDir()
	settings := LoadSettings()

	registryPath := os.Getenv("REGISTRY_PATH")
	if registryPath == "" {
		registryPath = filepath.Join(home, ".paulette")
	}

	// Priority: GIT_PATH env var > settings.json gitPath > settings.json reposPath (deprecated) > default
	gitPath := os.Getenv("GIT_PATH")
	if gitPath == "" {
		gitPath = settings.GitPath
	}
	if gitPath == "" {
		gitPath = settings.ReposPath // deprecated fallback
	}
	if gitPath == "" {
		gitPath = filepath.Join(home, ".paulette", "git")
	}

	// DataPath is always derived from registryPath — not user-configurable.
	dataPath := filepath.Join(registryPath, "repos")

	// Priority: env var > settings.json > default ("claude")
	claudePath := os.Getenv("CLAUDE_PATH")
	if claudePath == "" {
		claudePath = settings.ClaudePath
	}
	if claudePath == "" {
		claudePath = "claude"
	}

	oidcEnabled := os.Getenv("OIDC_ENABLED") == "true"
	oidcScopes := strings.Fields(os.Getenv("OIDC_SCOPES"))
	if len(oidcScopes) == 0 {
		oidcScopes = []string{"openid", "profile", "email"}
	}
	sessionSecret := os.Getenv("SESSION_SECRET")
	if oidcEnabled && len(sessionSecret) < 32 {
		log.Println("WARNING: SESSION_SECRET is not set or is shorter than 32 characters; OIDC sessions will be insecure")
	}

	ragEnabled := os.Getenv("RAG_ENABLED") == "true"
	ragBaseURL := os.Getenv("RAG_BASE_URL")
	if ragBaseURL == "" {
		ragBaseURL = "http://localhost:8000/api/v1"
	}

	return &Config{
		Port:         port,
		RegistryPath: registryPath,
		GitPath:      gitPath,
		DataPath:     dataPath,
		ClaudePath:   claudePath,
		RAG: RAGConfig{
			Enabled: ragEnabled,
			BaseURL: ragBaseURL,
		},
		OIDC: OIDCConfig{
			Enabled:       oidcEnabled,
			IssuerURL:     os.Getenv("OIDC_ISSUER_URL"),
			ClientID:      os.Getenv("OIDC_CLIENT_ID"),
			ClientSecret:  os.Getenv("OIDC_CLIENT_SECRET"),
			RedirectURI:   os.Getenv("OIDC_REDIRECT_URI"),
			Scopes:        oidcScopes,
			SessionSecret: sessionSecret,
		},
	}
}
