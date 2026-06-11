package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

type changelogResponse struct {
	Version   string `json:"version"`
	UpdatedAt string `json:"updated_at"`
	Markdown  string `json:"markdown"`
}

func readReleaseVersion() string {
	b, err := os.ReadFile("VERSION")
	if err != nil {
		return "0.0.0"
	}
	v := strings.TrimSpace(string(b))
	if v == "" {
		return "0.0.0"
	}
	return v
}

func readChangelogMarkdown() string {
	b, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		return "# Changelog\n\nNo changelog entries have been published yet.\n"
	}
	return string(b)
}

func handleAPIChangelog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	info := changelogResponse{
		Version:   readReleaseVersion(),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Markdown:  readChangelogMarkdown(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}
