package deploy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNewDODeployer(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		appName string
		repoURL string
		branch  string
		wantErr bool
	}{
		{
			name:    "Valid token",
			token:   "test-token-123",
			appName: "test-app",
			repoURL: "https://github.com/user/repo",
			branch:  "main",
			wantErr: false,
		},
		{
			name:    "Missing token",
			token:   "",
			appName: "test-app",
			repoURL: "https://github.com/user/repo",
			branch:  "main",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.token != "" {
				os.Setenv("DIGITALOCEAN_TOKEN", tt.token)
			} else {
				os.Unsetenv("DIGITALOCEAN_TOKEN")
			}

			deployer, err := NewDODeployer(tt.appName, tt.repoURL, tt.branch)

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
			if deployer.Token != tt.token {
				t.Errorf("Expected token %s, got %s", tt.token, deployer.Token)
			}
		})
	}

	os.Unsetenv("DIGITALOCEAN_TOKEN")
}

func TestDOBuildSpec(t *testing.T) {
	tests := []struct {
		name     string
		isStatic bool
		envVars  map[string]string
		wantType string
	}{
		{
			name:     "Static site",
			isStatic: true,
			envVars: map[string]string{
				"API_KEY": "test123",
			},
			wantType: "static_sites",
		},
		{
			name:     "Service (SSR)",
			isStatic: false,
			envVars: map[string]string{
				"DATABASE_URL": "postgres://...",
			},
			wantType: "services",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("DIGITALOCEAN_TOKEN", "test-token")
			defer os.Unsetenv("DIGITALOCEAN_TOKEN")

			deployer, err := NewDODeployer("test-app", "https://github.com/user/repo", "main")
			if err != nil {
				t.Fatalf("Failed to create deployer: %v", err)
			}

			spec := deployer.buildSpec(tt.isStatic, tt.envVars)

			// Verify the spec structure
			if tt.isStatic {
				if len(spec.StaticSites) == 0 {
					t.Error("Expected static_sites to be populated for static app")
				}
				if len(spec.Services) > 0 {
					t.Error("Expected services to be empty for static app")
				}

				site := spec.StaticSites[0]
				if site.Name != deployer.AppName {
					t.Errorf("Expected site name %s, got %s", deployer.AppName, site.Name)
				}
				if site.GitHub.Repo != "user/repo" {
					t.Errorf("Expected repo user/repo, got %s", site.GitHub.Repo)
				}
				if site.GitHub.Branch != deployer.Branch {
					t.Errorf("Expected branch %s, got %s", deployer.Branch, site.GitHub.Branch)
				}
				if !site.GitHub.DeployOnPush {
					t.Error("Expected DeployOnPush to be true")
				}
			} else {
				if len(spec.Services) == 0 {
					t.Error("Expected services to be populated for SSR app")
				}
				if len(spec.StaticSites) > 0 {
					t.Error("Expected static_sites to be empty for SSR app")
				}

				service := spec.Services[0]
				if service.Name != deployer.AppName {
					t.Errorf("Expected service name %s, got %s", deployer.AppName, service.Name)
				}
				if service.HTTPPort != 3000 {
					t.Errorf("Expected HTTP port 3000, got %d", service.HTTPPort)
				}
				if service.InstanceCount != 1 {
					t.Errorf("Expected instance count 1, got %d", service.InstanceCount)
				}
			}

			// Verify environment variables
			var envs []DOEnvVar
			if tt.isStatic {
				envs = spec.StaticSites[0].Envs
			} else {
				envs = spec.Services[0].Envs
			}

			if len(envs) != len(tt.envVars) {
				t.Errorf("Expected %d env vars, got %d", len(tt.envVars), len(envs))
			}

			for k, v := range tt.envVars {
				found := false
				for _, env := range envs {
					if env.Key == k && env.Value == v {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected to find env var %s=%s", k, v)
				}
			}
		})
	}
}

func TestDOEnvVarTypeDetection(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		wantType string
	}{
		{
			name:     "Secret key",
			key:      "API_SECRET",
			wantType: "SECRET",
		},
		{
			name:     "Password",
			key:      "DB_PASSWORD",
			wantType: "SECRET",
		},
		{
			name:     "Token",
			key:      "AUTH_TOKEN",
			wantType: "SECRET",
		},
		{
			name:     "Key in name",
			key:      "STRIPE_KEY",
			wantType: "SECRET",
		},
		{
			name:     "Regular variable",
			key:      "NODE_ENV",
			wantType: "GENERAL",
		},
		{
			name:     "API URL",
			key:      "API_URL",
			wantType: "GENERAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("DIGITALOCEAN_TOKEN", "test-token")
			defer os.Unsetenv("DIGITALOCEAN_TOKEN")

			deployer, _ := NewDODeployer("test-app", "https://github.com/user/repo", "main")
			envVars := map[string]string{tt.key: "test-value"}
			spec := deployer.buildSpec(true, envVars)

			env := spec.StaticSites[0].Envs[0]
			if env.Type != tt.wantType {
				t.Errorf("Expected type %s for key %s, got %s", tt.wantType, tt.key, env.Type)
			}
		})
	}
}

func TestDORepoURLParsing(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("DIGITALOCEAN_TOKEN", "test-token")
			defer os.Unsetenv("DIGITALOCEAN_TOKEN")

			deployer, _ := NewDODeployer("test-app", tt.repoURL, "main")
			spec := deployer.buildSpec(true, map[string]string{})

			repoPath := spec.StaticSites[0].GitHub.Repo
			if repoPath != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, repoPath)
			}
		})
	}
}

func TestDODeployWithMockServer(t *testing.T) {
	tests := []struct {
		name        string
		existingApp bool
		isStatic    bool
		wantErr     bool
	}{
		{
			name:        "Create new static app",
			existingApp: false,
			isStatic:    true,
			wantErr:     false,
		},
		{
			name:        "Update existing app",
			existingApp: true,
			isStatic:    false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockResponse := DOAppResponse{
				App: struct {
					ID               string `json:"id"`
					DefaultIngress   string `json:"default_ingress"`
					LiveURL          string `json:"live_url"`
					ActiveDeployment struct {
						ID    string `json:"id"`
						Phase string `json:"phase"`
					} `json:"active_deployment"`
				}{
					ID:             "test-app-id",
					DefaultIngress: "test-app.ondigitalocean.app",
					LiveURL:        "https://test-app.ondigitalocean.app",
				},
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify authorization header
				auth := r.Header.Get("Authorization")
				if !strings.HasPrefix(auth, "Bearer ") {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				switch {
				case r.Method == "GET" && r.URL.Path == "/v2/apps":
					// List apps
					if tt.existingApp {
						response := struct {
							Apps []struct {
								ID   string `json:"id"`
								Spec struct {
									Name string `json:"name"`
								} `json:"spec"`
							} `json:"apps"`
						}{
							Apps: []struct {
								ID   string `json:"id"`
								Spec struct {
									Name string `json:"name"`
								} `json:"spec"`
							}{
								{
									ID: "test-app-id",
									Spec: struct {
										Name string `json:"name"`
									}{Name: "test-app"},
								},
							},
						}
						json.NewEncoder(w).Encode(response)
					} else {
						json.NewEncoder(w).Encode(struct {
							Apps []interface{} `json:"apps"`
						}{Apps: []interface{}{}})
					}

				case r.Method == "POST" && r.URL.Path == "/v2/apps":
					// Create app
					json.NewEncoder(w).Encode(mockResponse)

				case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/v2/apps/"):
					// Update app
					json.NewEncoder(w).Encode(mockResponse)

				case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v2/apps/"):
					// Get app details
					json.NewEncoder(w).Encode(mockResponse)

				default:
					http.Error(w, "Not found", http.StatusNotFound)
				}
			}))
			defer server.Close()

			// Note: We can't easily test the full Deploy function without
			// making the API base URL configurable. This test validates
			// the buildSpec logic instead.
			os.Setenv("DIGITALOCEAN_TOKEN", "test-token")
			defer os.Unsetenv("DIGITALOCEAN_TOKEN")

			deployer, err := NewDODeployer("test-app", "https://github.com/user/repo", "main")
			if err != nil {
				t.Fatalf("Failed to create deployer: %v", err)
			}

			spec := deployer.buildSpec(tt.isStatic, map[string]string{})

			// Verify spec structure
			if spec.Name != "test-app" {
				t.Errorf("Expected app name test-app, got %s", spec.Name)
			}

			if tt.isStatic {
				if len(spec.StaticSites) == 0 {
					t.Error("Expected static sites to be populated")
				}
			} else {
				if len(spec.Services) == 0 {
					t.Error("Expected services to be populated")
				}
			}
		})
	}
}

func TestDODeployerFieldValidation(t *testing.T) {
	os.Setenv("DIGITALOCEAN_TOKEN", "test-token-123")
	defer os.Unsetenv("DIGITALOCEAN_TOKEN")

	deployer, err := NewDODeployer("my-app", "https://github.com/user/repo", "develop")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}

	if deployer.AppName != "my-app" {
		t.Errorf("Expected AppName 'my-app', got '%s'", deployer.AppName)
	}

	if deployer.RepoURL != "https://github.com/user/repo" {
		t.Errorf("Expected RepoURL 'https://github.com/user/repo', got '%s'", deployer.RepoURL)
	}

	if deployer.Branch != "develop" {
		t.Errorf("Expected Branch 'develop', got '%s'", deployer.Branch)
	}

	if deployer.Token != "test-token-123" {
		t.Errorf("Expected Token 'test-token-123', got '%s'", deployer.Token)
	}

	if deployer.client == nil {
		t.Error("Expected HTTP client to be initialized")
	}
}

func TestDORegionDefault(t *testing.T) {
	os.Setenv("DIGITALOCEAN_TOKEN", "test-token")
	defer os.Unsetenv("DIGITALOCEAN_TOKEN")

	deployer, _ := NewDODeployer("test-app", "https://github.com/user/repo", "main")
	spec := deployer.buildSpec(true, map[string]string{})

	if spec.Region != "nyc" {
		t.Errorf("Expected default region 'nyc', got '%s'", spec.Region)
	}
}

func TestDOInstanceDefaults(t *testing.T) {
	os.Setenv("DIGITALOCEAN_TOKEN", "test-token")
	defer os.Unsetenv("DIGITALOCEAN_TOKEN")

	deployer, _ := NewDODeployer("test-app", "https://github.com/user/repo", "main")
	spec := deployer.buildSpec(false, map[string]string{}) // SSR app

	service := spec.Services[0]
	if service.InstanceCount != 1 {
		t.Errorf("Expected instance count 1, got %d", service.InstanceCount)
	}
	if service.InstanceSizeSlug != "basic-xxs" {
		t.Errorf("Expected instance size 'basic-xxs', got '%s'", service.InstanceSizeSlug)
	}
	if service.HTTPPort != 3000 {
		t.Errorf("Expected HTTP port 3000, got %d", service.HTTPPort)
	}
}

func TestDOStaticSiteDefaults(t *testing.T) {
	os.Setenv("DIGITALOCEAN_TOKEN", "test-token")
	defer os.Unsetenv("DIGITALOCEAN_TOKEN")

	deployer, _ := NewDODeployer("test-app", "https://github.com/user/repo", "main")
	spec := deployer.buildSpec(true, map[string]string{})

	site := spec.StaticSites[0]
	if site.BuildCommand != "npm run build" {
		t.Errorf("Expected build command 'npm run build', got '%s'", site.BuildCommand)
	}
	if site.OutputDir != "dist" {
		t.Errorf("Expected output dir 'dist', got '%s'", site.OutputDir)
	}
}
