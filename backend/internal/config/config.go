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

type Config struct {
	Port         int
	RegistryPath string
	ReposPath    string
	ClaudePath   string
	Version      string
	Author       string
	OIDC         OIDCConfig
}

// Settings represents user-persisted configuration saved by "paulette init".
type Settings struct {
	ReposPath  string `json:"reposPath,omitempty"`
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

	// Priority: env var > settings.json > default
	reposPath := os.Getenv("REPOS_PATH")
	if reposPath == "" {
		reposPath = settings.ReposPath
	}
	if reposPath == "" {
		reposPath = filepath.Join(home, ".paulette", "repos")
	}

	// Priority: env var > settings.json > default ("claude")
	claudePath := os.Getenv("CLAUDE_PATH")
	if claudePath == "" {
		claudePath = settings.ClaudePath
	}
	if claudePath == "" {
		claudePath = "claude"
	}

	version, author := loadProjectMeta()

	oidcEnabled := os.Getenv("OIDC_ENABLED") == "true"
	oidcScopes := strings.Fields(os.Getenv("OIDC_SCOPES"))
	if len(oidcScopes) == 0 {
		oidcScopes = []string{"openid", "profile", "email"}
	}
	sessionSecret := os.Getenv("SESSION_SECRET")
	if oidcEnabled && len(sessionSecret) < 32 {
		log.Println("WARNING: SESSION_SECRET is not set or is shorter than 32 characters; OIDC sessions will be insecure")
	}

	return &Config{
		Port:         port,
		RegistryPath: registryPath,
		ReposPath:    reposPath,
		ClaudePath:   claudePath,
		Version:      version,
		Author:       author,
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

func loadProjectMeta() (version, author string) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", ""
	}
	data, err := os.ReadFile(filepath.Join(cwd, ".paulette", "project.json"))
	if err != nil {
		return "", ""
	}
	var meta struct {
		Version string `json:"version"`
		Author  string `json:"author"`
	}
	if json.Unmarshal(data, &meta) != nil {
		return "", ""
	}
	return meta.Version, meta.Author
}
