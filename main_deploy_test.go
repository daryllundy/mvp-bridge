package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeGitHubRemoteURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "SSH URL converted to HTTPS",
			input:    "git@github.com:owner/repo.git",
			expected: "https://github.com/owner/repo",
		},
		{
			name:     "HTTPS URL trims .git suffix",
			input:    "https://github.com/owner/repo.git",
			expected: "https://github.com/owner/repo",
		},
		{
			name:     "Already normalized remains unchanged",
			input:    "https://github.com/owner/repo",
			expected: "https://github.com/owner/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeGitHubRemoteURL(tt.input)
			if got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestDeriveAppName(t *testing.T) {
	tests := []struct {
		name      string
		configVal string
		repoURL   string
		expected  string
	}{
		{
			name:      "config value takes precedence",
			configVal: "configured-name",
			repoURL:   "https://github.com/org/repo",
			expected:  "configured-name",
		},
		{
			name:      "derived from repo URL",
			configVal: "",
			repoURL:   "https://github.com/org/repo",
			expected:  "repo",
		},
		{
			name:      "handles trailing slash",
			configVal: "",
			repoURL:   "https://github.com/org/repo/",
			expected:  "repo",
		},
		{
			name:      "falls back when repo missing name",
			configVal: "",
			repoURL:   "",
			expected:  "mvpbridge-app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveAppName(tt.configVal, tt.repoURL)
			if got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestParseEnvVars(t *testing.T) {
	content := `
# comment
API_KEY="abc123"
DATABASE_URL='postgres://localhost/db'
EMPTY=
SPACED = value
INVALID_LINE
`
	got := parseEnvVars(content)

	expected := map[string]string{
		"API_KEY":      "abc123",
		"DATABASE_URL": "postgres://localhost/db",
		"EMPTY":        "",
		"SPACED":       "value",
	}

	if len(got) != len(expected) {
		t.Fatalf("expected %d vars, got %d", len(expected), len(got))
	}

	for k, want := range expected {
		if got[k] != want {
			t.Fatalf("expected %s=%q, got %q", k, want, got[k])
		}
	}
}

func TestExtractEnvVarsNoEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	vars, err := extractEnvVars()
	if err != nil {
		t.Fatalf("expected no error when .env missing, got %v", err)
	}
	if len(vars) != 0 {
		t.Fatalf("expected empty vars for missing .env, got %v", vars)
	}
}

func TestExtractEnvVarsReadsCurrentDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("TOKEN='abc'\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	vars, err := extractEnvVars()
	if err != nil {
		t.Fatalf("extract env vars: %v", err)
	}
	if vars["TOKEN"] != "abc" {
		t.Fatalf("expected TOKEN=abc, got %q", vars["TOKEN"])
	}
}
