package config

import (
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Port         int
	RegistryPath string
}

func Load() *Config {
	port := 8080
	if p, err := strconv.Atoi(os.Getenv("PORT")); err == nil {
		port = p
	}

	registryPath := os.Getenv("REGISTRY_PATH")
	if registryPath == "" {
		home, _ := os.UserHomeDir()
		registryPath = filepath.Join(home, ".claudette")
	}

	return &Config{
		Port:         port,
		RegistryPath: registryPath,
	}
}
