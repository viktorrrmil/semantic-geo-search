package services

import (
	"strings"

	"examle.com/mod/config"
)

const defaultMainBackendURL = "http://localhost:8080"

func mainBackendURL() string {
	base := strings.TrimSpace(config.Current().MainBackendURL)
	if base == "" {
		base = defaultMainBackendURL
	}
	return strings.TrimRight(base, "/")
}
