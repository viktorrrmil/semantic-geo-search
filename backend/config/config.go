package config

import (
	"os"
	"strings"
)

const (
	defaultPort           = "3001"
	defaultMainBackendURL = "http://localhost:8080"
)

type Config struct {
	Port           string
	MainBackendURL string
	GeminiAPIKey   string
}

var current Config

func Load() (Config, error) {
	current = Config{
		Port:           envOrDefault("PORT", defaultPort),
		MainBackendURL: strings.TrimRight(envOrDefault("MAIN_BACKEND_URL", defaultMainBackendURL), "/"),
		GeminiAPIKey:   strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
	}

	return current, nil
}

func Current() Config {
	return current
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
