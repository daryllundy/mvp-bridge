package normalize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvpbridge/internal/detect"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name          string
		framework     detect.Framework
		dryRun        bool
		expectedRules int
	}{
		{
			name:          "Vite project",
			framework:     detect.Vite,
			dryRun:        false,
			expectedRules: 6, // 4 universal + 2 vite-specific
		},
		{
			name:          "Next.js project",
			framework:     detect.NextJS,
			dryRun:        false,
			expectedRules: 5, // 4 universal + 1 nextjs-specific
		},
		{
			name:          "Unknown framework",
			framework:     detect.Unknown,
			dryRun:        false,
			expectedRules: 4, // only universal rules
		},
		{
			name:          "Dry run mode",
			framework:     detect.Vite,
			dryRun:        true,
			expectedRules: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			n := New(tmpDir, tt.framework, tt.dryRun)

			if n.Root != tmpDir {
				t.Errorf("Expected root %s, got %s", tmpDir, n.Root)
			}

			if n.Framework != tt.framework {
				t.Errorf("Expected framework %v, got %v", tt.framework, n.Framework)
			}

			if n.DryRun != tt.dryRun {
				t.Errorf("Expected dry run %v, got %v", tt.dryRun, n.DryRun)
			}

			if len(n.Rules) != tt.expectedRules {
				t.Errorf("Expected %d rules, got %d", tt.expectedRules, len(n.Rules))
			}
		})
	}
}

func TestUniversalRules(t *testing.T) {
	rules := universalRules()

	// Check we have all expected universal rules
	expectedRules := []string{
		"Pin Node version",
		"Add .env.example",
		"Update .gitignore",
		"Add GitHub Actions workflow",
	}

	if len(rules) != len(expectedRules) {
		t.Errorf("Expected %d universal rules, got %d", len(expectedRules), len(rules))
	}

	for i, expected := range expectedRules {
		if i >= len(rules) {
			t.Errorf("Missing rule: %s", expected)
			continue
		}
		if rules[i].Name != expected {
			t.Errorf("Expected rule %d to be %q, got %q", i, expected, rules[i].Name)
		}
	}
}

func TestViteRules(t *testing.T) {
	rules := viteRules()

	expectedRules := []string{
		"Add Vite Dockerfile",
		"Add nginx config",
	}

	if len(rules) != len(expectedRules) {
		t.Errorf("Expected %d vite rules, got %d", len(expectedRules), len(rules))
	}

	for i, expected := range expectedRules {
		if i >= len(rules) {
			t.Errorf("Missing rule: %s", expected)
			continue
		}
		if rules[i].Name != expected {
			t.Errorf("Expected rule %d to be %q, got %q", i, expected, rules[i].Name)
		}
	}
}

func TestNextJSRules(t *testing.T) {
	rules := nextjsRules()

	if len(rules) != 1 {
		t.Errorf("Expected 1 nextjs rule, got %d", len(rules))
	}

	if len(rules) > 0 && rules[0].Name != "Add Next.js Dockerfile" {
		t.Errorf("Expected rule to be %q, got %q", "Add Next.js Dockerfile", rules[0].Name)
	}
}

func TestNodeVersionRule(t *testing.T) {
	tmpDir := t.TempDir()

	rules := universalRules()
	nodeRule := rules[0] // Pin Node version is first

	// Check should return false when .nvmrc doesn't exist
	if nodeRule.Check(tmpDir) {
		t.Error("Expected Check to return false when .nvmrc doesn't exist")
	}

	// Apply the rule
	if err := nodeRule.Apply(tmpDir, false); err != nil {
		t.Fatalf("Failed to apply rule: %v", err)
	}

	// Verify .nvmrc was created
	nvmrcPath := filepath.Join(tmpDir, ".nvmrc")
	data, err := os.ReadFile(nvmrcPath)
	if err != nil {
		t.Fatalf("Failed to read .nvmrc: %v", err)
	}

	content := strings.TrimSpace(string(data))
	if content != "20" {
		t.Errorf("Expected .nvmrc content to be '20', got %q", content)
	}

	// Check should now return true
	if !nodeRule.Check(tmpDir) {
		t.Error("Expected Check to return true after applying rule")
	}
}

func TestEnvExampleRule(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .env file
	envPath := filepath.Join(tmpDir, ".env")
	envContent := `API_KEY=secret123
DATABASE_URL=postgres://localhost/db
# Comment line
NODE_ENV=production
`
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("Failed to create .env: %v", err)
	}

	rules := universalRules()
	envRule := rules[1] // Add .env.example is second

	// Apply the rule
	if err := envRule.Apply(tmpDir, false); err != nil {
		t.Fatalf("Failed to apply rule: %v", err)
	}

	// Read .env.example
	examplePath := filepath.Join(tmpDir, ".env.example")
	data, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("Failed to read .env.example: %v", err)
	}

	content := string(data)

	// Verify keys are present but values are removed
	if !strings.Contains(content, "API_KEY=") {
		t.Error("Expected API_KEY in .env.example")
	}
	if !strings.Contains(content, "DATABASE_URL=") {
		t.Error("Expected DATABASE_URL in .env.example")
	}
	if !strings.Contains(content, "NODE_ENV=") {
		t.Error("Expected NODE_ENV in .env.example")
	}

	// Verify secrets are not present
	if strings.Contains(content, "secret123") {
		t.Error("Secrets should not be in .env.example")
	}
	if strings.Contains(content, "postgres://localhost/db") {
		t.Error("Secret values should not be in .env.example")
	}
}

func TestGitignoreRule(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a basic .gitignore
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	existing := `# Existing entries
*.log
`
	if err := os.WriteFile(gitignorePath, []byte(existing), 0644); err != nil {
		t.Fatalf("Failed to create .gitignore: %v", err)
	}

	rules := universalRules()
	gitignoreRule := rules[2] // Update .gitignore is third

	// Apply the rule
	if err := gitignoreRule.Apply(tmpDir, false); err != nil {
		t.Fatalf("Failed to apply rule: %v", err)
	}

	// Read .gitignore
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("Failed to read .gitignore: %v", err)
	}

	content := string(data)

	// Verify standard entries were added
	requiredEntries := []string{
		"node_modules",
		".env",
		"dist",
		".next",
	}

	for _, entry := range requiredEntries {
		if !strings.Contains(content, entry) {
			t.Errorf("Expected .gitignore to contain %q", entry)
		}
	}

	// Verify existing content is preserved
	if !strings.Contains(content, "*.log") {
		t.Error("Existing .gitignore content should be preserved")
	}
}

func TestDryRunMode(t *testing.T) {
	tmpDir := t.TempDir()

	n := New(tmpDir, detect.Vite, true)

	// Find the node version rule
	var nodeRule *Rule
	for i := range n.Rules {
		if n.Rules[i].Name == "Pin Node version" {
			nodeRule = &n.Rules[i]
			break
		}
	}

	if nodeRule == nil {
		t.Fatal("Node version rule not found")
	}

	// Apply in dry run mode
	if err := nodeRule.Apply(tmpDir, true); err != nil {
		t.Fatalf("Dry run should not error: %v", err)
	}

	// Verify file was NOT created
	nvmrcPath := filepath.Join(tmpDir, ".nvmrc")
	if _, err := os.Stat(nvmrcPath); err == nil {
		t.Error("File should not be created in dry run mode")
	}
}

func TestViteDockerfileTemplate(t *testing.T) {
	if !strings.Contains(viteDockerfile, "FROM node:20-alpine") {
		t.Error("Vite Dockerfile should use Node 20 Alpine")
	}
	if !strings.Contains(viteDockerfile, "FROM nginx:alpine") {
		t.Error("Vite Dockerfile should use nginx for serving")
	}
	if !strings.Contains(viteDockerfile, "/app/dist") {
		t.Error("Vite Dockerfile should copy from /app/dist")
	}
}

func TestNextJSDockerfileTemplates(t *testing.T) {
	t.Run("Static Dockerfile", func(t *testing.T) {
		if !strings.Contains(nextStaticDockerfile, "FROM node:20-alpine") {
			t.Error("Next.js static Dockerfile should use Node 20 Alpine")
		}
		if !strings.Contains(nextStaticDockerfile, "/app/out") {
			t.Error("Next.js static Dockerfile should copy from /app/out")
		}
	})

	t.Run("SSR Dockerfile", func(t *testing.T) {
		if !strings.Contains(nextSSRDockerfile, "FROM node:20-alpine") {
			t.Error("Next.js SSR Dockerfile should use Node 20 Alpine")
		}
		if !strings.Contains(nextSSRDockerfile, "standalone") {
			t.Error("Next.js SSR Dockerfile should use standalone output")
		}
		if !strings.Contains(nextSSRDockerfile, "PORT=3000") {
			t.Error("Next.js SSR Dockerfile should expose port 3000")
		}
	})
}

func TestNginxConfig(t *testing.T) {
	// Verify nginx config has SPA routing
	if !strings.Contains(nginxConfig, "try_files $uri $uri/ /index.html") {
		t.Error("nginx config should support SPA routing")
	}

	// Verify caching is configured
	if !strings.Contains(nginxConfig, "Cache-Control") {
		t.Error("nginx config should have cache control headers")
	}

	// Verify gzip is enabled
	if !strings.Contains(nginxConfig, "gzip on") {
		t.Error("nginx config should enable gzip")
	}
}

func TestGitHubWorkflow(t *testing.T) {
	// Verify workflow has required steps
	if !strings.Contains(githubWorkflow, "actions/checkout@v4") {
		t.Error("GitHub workflow should use checkout action")
	}
	if !strings.Contains(githubWorkflow, "actions/setup-node@v4") {
		t.Error("GitHub workflow should use setup-node action")
	}
	if !strings.Contains(githubWorkflow, "npm ci") {
		t.Error("GitHub workflow should install dependencies")
	}
	if !strings.Contains(githubWorkflow, "npm run build") {
		t.Error("GitHub workflow should run build")
	}
}

func TestGitHubWorkflowAWS(t *testing.T) {
	// Verify AWS workflow has required steps
	if !strings.Contains(githubWorkflowAWS, "aws-actions/configure-aws-credentials") {
		t.Error("AWS workflow should configure credentials")
	}
	if !strings.Contains(githubWorkflowAWS, "AWS_ACCESS_KEY_ID") {
		t.Error("AWS workflow should reference access key")
	}
	if !strings.Contains(githubWorkflowAWS, "AWS_SECRET_ACCESS_KEY") {
		t.Error("AWS workflow should reference secret key")
	}
}

func TestCreateEnvExample(t *testing.T) {
	tests := []struct {
		name        string
		envContent  string
		wantKeys    []string
		wantSecrets []string // Values that should NOT appear
	}{
		{
			name: "Basic env file",
			envContent: `API_KEY=secret123
DATABASE_URL=postgres://localhost/db
NODE_ENV=production
`,
			wantKeys:    []string{"API_KEY=", "DATABASE_URL=", "NODE_ENV="},
			wantSecrets: []string{"secret123", "postgres://localhost/db", "production"},
		},
		{
			name: "With comments",
			envContent: `# API Configuration
API_KEY=abc123
# Database
DATABASE_URL=mysql://localhost
`,
			wantKeys:    []string{"API_KEY=", "DATABASE_URL="},
			wantSecrets: []string{"abc123", "mysql://localhost"},
		},
		{
			name:       "No .env file",
			envContent: "",
			wantKeys:   []string{"# Environment variables"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if tt.envContent != "" {
				envPath := filepath.Join(tmpDir, ".env")
				if err := os.WriteFile(envPath, []byte(tt.envContent), 0644); err != nil {
					t.Fatalf("Failed to create .env: %v", err)
				}
			}

			if err := createEnvExample(tmpDir); err != nil {
				t.Fatalf("createEnvExample failed: %v", err)
			}

			examplePath := filepath.Join(tmpDir, ".env.example")
			data, err := os.ReadFile(examplePath)
			if err != nil {
				t.Fatalf("Failed to read .env.example: %v", err)
			}

			content := string(data)

			// Check for expected keys
			for _, key := range tt.wantKeys {
				if !strings.Contains(content, key) {
					t.Errorf("Expected .env.example to contain %q", key)
				}
			}

			// Check that secrets are NOT present
			for _, secret := range tt.wantSecrets {
				if strings.Contains(content, secret) {
					t.Errorf(".env.example should NOT contain secret value %q", secret)
				}
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if !fileExists(testFile) {
		t.Error("fileExists should return true for existing file")
	}

	nonExistent := filepath.Join(tmpDir, "missing.txt")
	if fileExists(nonExistent) {
		t.Error("fileExists should return false for non-existent file")
	}
}
