package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFramework(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected Framework
		wantErr  bool
	}{
		{
			name:     "Detects Vite from vite.config.js",
			files:    []string{"vite.config.js"},
			expected: Vite,
			wantErr:  false,
		},
		{
			name:     "Detects Next.js from next.config.js",
			files:    []string{"next.config.js"},
			expected: NextJS,
			wantErr:  false,
		},
		{
			name:     "Next.js takes precedence over Vite",
			files:    []string{"vite.config.js", "next.config.js"},
			expected: NextJS,
			wantErr:  false,
		},
		{
			name:     "Returns error when no framework found",
			files:    []string{},
			expected: Unknown,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()

			// Create test files
			for _, file := range tt.files {
				path := filepath.Join(tmpDir, file)
				if err := os.WriteFile(path, []byte(""), 0644); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			}

			// Test detection
			result, err := DetectFramework(tmpDir)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestDetectPackageManager(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected PackageManager
	}{
		{
			name:     "Detects pnpm",
			files:    []string{"pnpm-lock.yaml"},
			expected: PNPM,
		},
		{
			name:     "Detects yarn",
			files:    []string{"yarn.lock"},
			expected: Yarn,
		},
		{
			name:     "Detects npm",
			files:    []string{"package-lock.json"},
			expected: NPM,
		},
		{
			name:     "Defaults to npm when no lock file",
			files:    []string{},
			expected: NPM,
		},
		{
			name:     "pnpm takes precedence",
			files:    []string{"pnpm-lock.yaml", "yarn.lock", "package-lock.json"},
			expected: PNPM,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			for _, file := range tt.files {
				path := filepath.Join(tmpDir, file)
				if err := os.WriteFile(path, []byte(""), 0644); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			}

			result := DetectPackageManager(tmpDir)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestDetectNodeVersion(t *testing.T) {
	tests := []struct {
		name     string
		nvmrc    string
		expected string
	}{
		{
			name:     "Detects from .nvmrc",
			nvmrc:    "20",
			expected: "20",
		},
		{
			name:     "Returns empty when no .nvmrc",
			nvmrc:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if tt.nvmrc != "" {
				path := filepath.Join(tmpDir, ".nvmrc")
				if err := os.WriteFile(path, []byte(tt.nvmrc), 0644); err != nil {
					t.Fatalf("Failed to create .nvmrc: %v", err)
				}
			}

			result := DetectNodeVersion(tmpDir)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectOutputType(t *testing.T) {
	tests := []struct {
		name       string
		framework  Framework
		configFile string
		content    string
		expected   OutputType
	}{
		{
			name:      "Vite is always static",
			framework: Vite,
			expected:  Static,
		},
		{
			name:       "Next.js with export is static",
			framework:  NextJS,
			configFile: "next.config.js",
			content:    `module.exports = { output: "export" }`,
			expected:   Static,
		},
		{
			name:       "Next.js without export is SSR",
			framework:  NextJS,
			configFile: "next.config.js",
			content:    `module.exports = {}`,
			expected:   SSR,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if tt.configFile != "" {
				path := filepath.Join(tmpDir, tt.configFile)
				if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
					t.Fatalf("Failed to create config file: %v", err)
				}
			}

			result := DetectOutputType(tmpDir, tt.framework)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestDetectPersistence(t *testing.T) {
	tests := []struct {
		name          string
		files         map[string]string
		expectedKind  PersistenceKind
		expectedReason string
	}{
		{
			name: "Postgres from dependencies and env",
			files: map[string]string{
				"package.json": `{"dependencies":{"pg":"^8.0.0"}}`,
				".env":         "DATABASE_URL=postgresql://user:pass@db:5432/app\n",
			},
			expectedKind:  PersistencePostgres,
			expectedReason: "dependency:pg",
		},
		{
			name: "SQLite from prisma schema",
			files: map[string]string{
				"package.json":         `{"dependencies":{"better-sqlite3":"^9.0.0"}}`,
				"prisma/schema.prisma": `datasource db { provider = "sqlite" url = "file:./dev.db" }`,
			},
			expectedKind:  PersistenceSQLite,
			expectedReason: "schema.prisma",
		},
		{
			name: "No persistence inferred",
			files: map[string]string{
				"package.json": `{"dependencies":{"react":"^18.0.0"}}`,
			},
			expectedKind: PersistenceNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			for rel, content := range tt.files {
				path := filepath.Join(tmpDir, rel)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", rel, err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", rel, err)
				}
			}

			got := DetectPersistence(tmpDir)
			if got.Kind != tt.expectedKind {
				t.Fatalf("expected %s, got %s", tt.expectedKind, got.Kind)
			}
			if tt.expectedReason != "" && got.Kind != PersistenceNone && got.Reason == "" {
				t.Fatalf("expected non-empty reason, got empty")
			}
		})
	}
}
