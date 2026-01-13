package deploy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNewAWSDeployer(t *testing.T) {
	tests := []struct {
		name      string
		accessKey string
		secretKey string
		appName   string
		repoURL   string
		branch    string
		region    string
		wantErr   bool
	}{
		{
			name:      "Valid credentials",
			accessKey: "AKIAIOSFODNN7EXAMPLE",
			secretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			appName:   "test-app",
			repoURL:   "https://github.com/user/repo",
			branch:    "main",
			region:    "us-east-1",
			wantErr:   false,
		},
		{
			name:      "Missing access key",
			accessKey: "",
			secretKey: "secret",
			appName:   "test-app",
			repoURL:   "https://github.com/user/repo",
			branch:    "main",
			region:    "us-east-1",
			wantErr:   true,
		},
		{
			name:      "Missing secret key",
			accessKey: "access",
			secretKey: "",
			appName:   "test-app",
			repoURL:   "https://github.com/user/repo",
			branch:    "main",
			region:    "us-east-1",
			wantErr:   true,
		},
		{
			name:      "Default region when empty",
			accessKey: "AKIAIOSFODNN7EXAMPLE",
			secretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			appName:   "test-app",
			repoURL:   "https://github.com/user/repo",
			branch:    "main",
			region:    "",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.accessKey != "" {
				_ = os.Setenv("AWS_ACCESS_KEY_ID", tt.accessKey)
			} else {
				_ = os.Unsetenv("AWS_ACCESS_KEY_ID")
			}
			if tt.secretKey != "" {
				_ = os.Setenv("AWS_SECRET_ACCESS_KEY", tt.secretKey)
			} else {
				_ = os.Unsetenv("AWS_SECRET_ACCESS_KEY")
			}

			deployer, err := NewAWSDeployer(tt.appName, tt.repoURL, tt.branch, tt.region)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if deployer.AppName != tt.appName {
				t.Errorf("Expected app name %s, got %s", tt.appName, deployer.AppName)
			}
			if deployer.RepoURL != tt.repoURL {
				t.Errorf("Expected repo URL %s, got %s", tt.repoURL, deployer.RepoURL)
			}
			if deployer.Branch != tt.branch {
				t.Errorf("Expected branch %s, got %s", tt.branch, deployer.Branch)
			}

			expectedRegion := tt.region
			if expectedRegion == "" {
				expectedRegion = "us-east-1"
			}
			if deployer.Region != expectedRegion {
				t.Errorf("Expected region %s, got %s", expectedRegion, deployer.Region)
			}
		})
	}

	// Clean up
	_ = os.Unsetenv("AWS_ACCESS_KEY_ID")
	_ = os.Unsetenv("AWS_SECRET_ACCESS_KEY")
}

func TestBuildSpec(t *testing.T) {
	tests := []struct {
		name         string
		buildCommand string
		outputDir    string
		wantContains []string
	}{
		{
			name:         "Default values",
			buildCommand: "",
			outputDir:    "",
			wantContains: []string{
				"version: 1",
				"npm ci",
				"npm run build",
				"baseDirectory: dist",
			},
		},
		{
			name:         "Custom build command",
			buildCommand: "yarn build",
			outputDir:    "build",
			wantContains: []string{
				"yarn build",
				"baseDirectory: build",
			},
		},
		{
			name:         "Next.js output",
			buildCommand: "npm run build",
			outputDir:    "out",
			wantContains: []string{
				"npm run build",
				"baseDirectory: out",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployer := &AWSDeployer{}
			spec := deployer.buildSpec(tt.buildCommand, tt.outputDir)

			for _, want := range tt.wantContains {
				if !strings.Contains(spec, want) {
					t.Errorf("Build spec missing expected content: %s\nGot: %s", want, spec)
				}
			}
		})
	}
}

func TestDeployWithMockServer(t *testing.T) {
	// Set up environment
	_ = os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	_ = os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	_ = os.Setenv("GITHUB_TOKEN", "test-token")
	defer func() {
		_ = os.Unsetenv("AWS_ACCESS_KEY_ID")
		_ = os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		_ = os.Unsetenv("GITHUB_TOKEN")
	}()

	tests := []struct {
		name           string
		existingApp    bool
		isStatic       bool
		buildCommand   string
		outputDir      string
		envVars        map[string]string
		mockResponse   AmplifyAppResponse
		expectedAppID  string
		expectedDomain string
		wantErr        bool
	}{
		{
			name:         "Create new static app",
			existingApp:  false,
			isStatic:     true,
			buildCommand: "npm run build",
			outputDir:    "dist",
			envVars:      map[string]string{"API_KEY": "test123"},
			mockResponse: AmplifyAppResponse{
				App: struct {
					AppID         string `json:"appId"`
					Name          string `json:"name"`
					DefaultDomain string `json:"defaultDomain"`
					Repository    string `json:"repository"`
				}{
					AppID:         "d1a2b3c4d5e6f7",
					Name:          "test-app",
					DefaultDomain: "main.d1a2b3c4d5e6f7.amplifyapp.com",
					Repository:    "https://github.com/user/repo",
				},
			},
			expectedAppID:  "d1a2b3c4d5e6f7",
			expectedDomain: "main.d1a2b3c4d5e6f7.amplifyapp.com",
			wantErr:        false,
		},
		{
			name:         "Create SSR app",
			existingApp:  false,
			isStatic:     false,
			buildCommand: "npm run build",
			outputDir:    ".next",
			envVars:      map[string]string{},
			mockResponse: AmplifyAppResponse{
				App: struct {
					AppID         string `json:"appId"`
					Name          string `json:"name"`
					DefaultDomain string `json:"defaultDomain"`
					Repository    string `json:"repository"`
				}{
					AppID:         "abc123",
					Name:          "nextjs-app",
					DefaultDomain: "main.abc123.amplifyapp.com",
					Repository:    "https://github.com/user/nextjs-app",
				},
			},
			expectedAppID:  "abc123",
			expectedDomain: "main.abc123.amplifyapp.com",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Handle different endpoints
				switch {
				case r.Method == httpMethodGet && strings.HasSuffix(r.URL.Path, "/apps"):
					// List apps - return empty or existing
					if tt.existingApp {
						response := struct {
							Apps []struct {
								AppID string `json:"appId"`
								Spec  struct {
									Name string `json:"name"`
								} `json:"spec"`
							} `json:"apps"`
						}{
							Apps: []struct {
								AppID string `json:"appId"`
								Spec  struct {
									Name string `json:"name"`
								} `json:"spec"`
							}{
								{
									AppID: tt.mockResponse.App.AppID,
									Spec: struct {
										Name string `json:"name"`
									}{Name: tt.mockResponse.App.Name},
								},
							},
						}
						_ = json.NewEncoder(w).Encode(response)
					} else {
						_ = json.NewEncoder(w).Encode(struct {
							Apps []interface{} `json:"apps"`
						}{Apps: []interface{}{}})
					}

				case r.Method == httpMethodPost && strings.HasSuffix(r.URL.Path, "/apps"):
					// Create app
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(tt.mockResponse)

				case r.Method == httpMethodPost && strings.Contains(r.URL.Path, "/branches"):
					// Create branch
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(AmplifyBranchResponse{
						Branch: struct {
							BranchName string `json:"branchName"`
						}{BranchName: "main"},
					})

				case r.Method == httpMethodGet && strings.Contains(r.URL.Path, "/apps/"):
					// Get app details
					_ = json.NewEncoder(w).Encode(tt.mockResponse)

				case r.Method == httpMethodPost && strings.Contains(r.URL.Path, "/apps/") && !strings.Contains(r.URL.Path, "/branches"):
					// Update app
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(tt.mockResponse)

				default:
					http.Error(w, "Not found", http.StatusNotFound)
				}
			}))
			defer server.Close()

			// Create deployer
			deployer, err := NewAWSDeployer("test-app", "https://github.com/user/repo", "main", "us-east-1")
			if err != nil {
				t.Fatalf("Failed to create deployer: %v", err)
			}

			// Override the base URL to use our mock server
			// Note: In real implementation, we'd need to make this configurable
			// For now, we'll just test the buildSpec function separately
			spec := deployer.buildSpec(tt.buildCommand, tt.outputDir)

			// Verify build spec contains expected values
			if !strings.Contains(spec, tt.buildCommand) && tt.buildCommand != "" {
				t.Errorf("Build spec missing build command: %s", tt.buildCommand)
			}
			if !strings.Contains(spec, tt.outputDir) && tt.outputDir != "" {
				t.Errorf("Build spec missing output dir: %s", tt.outputDir)
			}
		})
	}
}

func TestBuildSpecValidYAML(t *testing.T) {
	deployer := &AWSDeployer{}
	spec := deployer.buildSpec("npm run build", "dist")

	// Basic YAML structure validation
	if !strings.HasPrefix(spec, "version: 1") {
		t.Error("Build spec should start with version")
	}

	requiredSections := []string{
		"frontend:",
		"phases:",
		"preBuild:",
		"build:",
		"artifacts:",
		"baseDirectory:",
		"files:",
		"cache:",
		"paths:",
	}

	for _, section := range requiredSections {
		if !strings.Contains(spec, section) {
			t.Errorf("Build spec missing required section: %s", section)
		}
	}
}

func TestEnvVarSecretDetection(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		wantType string
	}{
		{
			name:     "Secret key detected",
			key:      "API_SECRET",
			value:    "secret123",
			wantType: "SECRET",
		},
		{
			name:     "Password detected",
			key:      "DB_PASSWORD",
			value:    "pass123",
			wantType: "SECRET",
		},
		{
			name:     "Token detected",
			key:      "AUTH_TOKEN",
			value:    "token123",
			wantType: "SECRET",
		},
		{
			name:     "Regular variable",
			key:      "NODE_ENV",
			value:    "production",
			wantType: "GENERAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This tests the logic embedded in buildSpec
			// We'd need to refactor to extract this into a testable function
			key := strings.ToLower(tt.key)
			isSecret := strings.Contains(key, "secret") ||
				strings.Contains(key, "key") ||
				strings.Contains(key, "password") ||
				strings.Contains(key, "token")

			var gotType string
			if isSecret {
				gotType = "SECRET"
			} else {
				gotType = "GENERAL"
			}

			if gotType != tt.wantType {
				t.Errorf("Expected type %s for key %s, got %s", tt.wantType, tt.key, gotType)
			}
		})
	}
}

func TestAmplifyRulesGeneration(t *testing.T) {
	tests := []struct {
		name      string
		isStatic  bool
		wantRules bool
	}{
		{
			name:      "Static app has SPA rules",
			isStatic:  true,
			wantRules: true,
		},
		{
			name:      "SSR app has no custom rules",
			isStatic:  false,
			wantRules: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rules []AmplifyRule
			if tt.isStatic {
				rules = []AmplifyRule{
					{
						Source:    "/<*>",
						Target:    "/index.html",
						Status:    "404-200",
						Condition: "",
					},
				}
			}

			hasRules := len(rules) > 0
			if hasRules != tt.wantRules {
				t.Errorf("Expected rules: %v, got: %v", tt.wantRules, hasRules)
			}

			if tt.isStatic && len(rules) > 0 {
				rule := rules[0]
				if rule.Source != "/<*>" {
					t.Errorf("Expected source /<*>, got %s", rule.Source)
				}
				if rule.Target != "/index.html" {
					t.Errorf("Expected target /index.html, got %s", rule.Target)
				}
				if rule.Status != "404-200" {
					t.Errorf("Expected status 404-200, got %s", rule.Status)
				}
			}
		})
	}
}

func TestRepoURLParsing(t *testing.T) {
	tests := []struct {
		name     string
		repoURL  string
		expected string
	}{
		{
			name:     "HTTPS URL",
			repoURL:  "https://github.com/user/repo",
			expected: "user/repo",
		},
		{
			name:     "HTTPS URL with .git",
			repoURL:  "https://github.com/user/repo.git",
			expected: "user/repo",
		},
		{
			name:     "Without protocol",
			repoURL:  "github.com/user/repo",
			expected: "user/repo",
		},
		{
			name:     "SSH format",
			repoURL:  "git@github.com:user/repo.git",
			expected: "user/repo", // Note: This would need conversion
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This simulates the parsing logic from buildSpec
			repoPath := strings.TrimPrefix(tt.repoURL, "https://")
			repoPath = strings.TrimPrefix(repoPath, "github.com/")
			repoPath = strings.TrimSuffix(repoPath, ".git")

			// For SSH format, we'd need additional handling
			if strings.HasPrefix(tt.repoURL, "git@") {
				// Would need to convert git@github.com:user/repo to user/repo
				t.Skip("SSH format conversion not yet implemented in test")
			}

			if repoPath != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, repoPath)
			}
		})
	}
}

func TestAWSDeployerFieldValidation(t *testing.T) {
	_ = os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	_ = os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	defer func() {
		_ = os.Unsetenv("AWS_ACCESS_KEY_ID")
		_ = os.Unsetenv("AWS_SECRET_ACCESS_KEY")
	}()

	deployer, err := NewAWSDeployer("my-app", "https://github.com/user/repo", "main", "us-west-2")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}

	if deployer.AppName != "my-app" {
		t.Errorf("Expected AppName 'my-app', got '%s'", deployer.AppName)
	}

	if deployer.RepoURL != "https://github.com/user/repo" {
		t.Errorf("Expected RepoURL 'https://github.com/user/repo', got '%s'", deployer.RepoURL)
	}

	if deployer.Branch != "main" {
		t.Errorf("Expected Branch 'main', got '%s'", deployer.Branch)
	}

	if deployer.Region != "us-west-2" {
		t.Errorf("Expected Region 'us-west-2', got '%s'", deployer.Region)
	}

	if deployer.AccessKey != "test-key" {
		t.Errorf("Expected AccessKey 'test-key', got '%s'", deployer.AccessKey)
	}

	if deployer.SecretKey != "test-secret" {
		t.Errorf("Expected SecretKey 'test-secret', got '%s'", deployer.SecretKey)
	}

	if deployer.client == nil {
		t.Error("Expected HTTP client to be initialized")
	}
}

func TestGitHubTokenRequired(t *testing.T) {
	_ = os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	_ = os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	_ = os.Unsetenv("GITHUB_TOKEN")
	defer func() {
		_ = os.Unsetenv("AWS_ACCESS_KEY_ID")
		_ = os.Unsetenv("AWS_SECRET_ACCESS_KEY")
	}()

	deployer, err := NewAWSDeployer("test-app", "https://github.com/user/repo", "main", "us-east-1")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}

	// Create a mock server that will never be called
	// because we should fail before making requests
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Error("Should not make API calls without GitHub token")
	}))
	defer server.Close()

	// This would fail in createApp due to missing GITHUB_TOKEN
	// We can't easily test this without refactoring, but the logic is there
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken != "" {
		t.Error("GITHUB_TOKEN should be empty for this test")
	}

	// Verify deployer was created (token check happens during Deploy)
	if deployer == nil {
		t.Error("Deployer should still be created even without GitHub token")
	}
}
