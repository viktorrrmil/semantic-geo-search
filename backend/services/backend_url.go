package services

import (
	"os"
	"strings"
)

const defaultMainBackendURL = "http://localhost:8080"

func mainBackendURL() string {
	base := strings.TrimSpace(os.Getenv("MAIN_BACKEND_URL"))
	if base == "" {
		base = defaultMainBackendURL
	}
	return strings.TrimRight(base, "/")
}
