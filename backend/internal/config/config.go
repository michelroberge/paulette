package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Port         int
	RegistryPath string
	ReposPath    string
	Version      string
	Author       string
}

// Settings represents user-persisted configuration saved by "claudine init".
type Settings struct {
	ReposPath string `json:"reposPath,omitempty"`
}

// SettingsPath returns the path to the settings file.
func SettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claudine", "settings.json")
}

// LoadSettings reads ~/.claudine/settings.json (returns zero value if missing).
func LoadSettings() Settings {
	var s Settings
	data, err := os.ReadFile(SettingsPath())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	return s
}

// SaveSettings writes settings to ~/.claudine/settings.json.
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
		registryPath = filepath.Join(home, ".claudine")
	}

	// Priority: env var > settings.json > default
	reposPath := os.Getenv("REPOS_PATH")
	if reposPath == "" {
		reposPath = settings.ReposPath
	}
	if reposPath == "" {
		reposPath = filepath.Join(home, ".claudine", "repos")
	}

	version, author := loadProjectMeta()

	return &Config{
		Port:         port,
		RegistryPath: registryPath,
		ReposPath:    reposPath,
		Version:      version,
		Author:       author,
	}
}

func loadProjectMeta() (version, author string) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", ""
	}
	data, err := os.ReadFile(filepath.Join(cwd, ".claudine", "project.json"))
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
