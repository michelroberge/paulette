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

func Load() *Config {
	port := 8080
	if p, err := strconv.Atoi(os.Getenv("PORT")); err == nil {
		port = p
	}

	registryPath := os.Getenv("REGISTRY_PATH")
	if registryPath == "" {
		home, _ := os.UserHomeDir()
		registryPath = filepath.Join(home, ".Claudine")
	}

	reposPath := os.Getenv("REPOS_PATH")
	if reposPath == "" {
		cwd, _ := os.Getwd()
		reposPath = filepath.Join(cwd, "repos")
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
