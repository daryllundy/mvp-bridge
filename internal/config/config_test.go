package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daryllundy/mvp-bridge/internal/detect"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		wantErr    bool
		checkFunc  func(*Config) error
	}{
		{
			name: "Valid config",
			configYAML: `version: 1
framework: vite
target: do
detected:
  package_manager: npm
  build_command: npm run build
  output_dir: dist
  node_version: "20"
  output_type: static
`,
			wantErr: false,
			checkFunc: func(c *Config) error {
				if c.Version != 1 {
					t.Errorf("Expected version 1, got %d", c.Version)
				}
				if c.Framework != "vite" {
					t.Errorf("Expected framework vite, got %s", c.Framework)
				}
				if c.Target != "do" {
					t.Errorf("Expected target do, got %s", c.Target)
				}
				return nil
			},
		},
		{
			name: "Minimal config",
			configYAML: `version: 1
framework: nextjs
target: aws
`,
			wantErr: false,
			checkFunc: func(c *Config) error {
				if c.Framework != "nextjs" {
					t.Errorf("Expected framework nextjs, got %s", c.Framework)
				}
				return nil
			},
		},
		{
			name: "GCP config with project ID",
			configYAML: `version: 1
framework: vite
target: gcp
deploy:
  project_id: demo-project
  region: us-central1
`,
			wantErr: false,
			checkFunc: func(c *Config) error {
				if c.Target != "gcp" {
					t.Errorf("Expected target gcp, got %s", c.Target)
				}
				if c.Deploy.ProjectID != "demo-project" {
					t.Errorf("Expected project ID demo-project, got %s", c.Deploy.ProjectID)
				}
				return nil
			},
		},
		{
			name: "Azure config with deploy settings",
			configYAML: `version: 1
framework: vite
target: azure
deploy:
  subscription_id: sub-123
  resource_group: rg-app
  environment: env-app
  region: eastus
`,
			wantErr: false,
			checkFunc: func(c *Config) error {
				if c.Target != "azure" {
					t.Errorf("Expected target azure, got %s", c.Target)
				}
				if c.Deploy.SubscriptionID != "sub-123" {
					t.Errorf("Expected subscription sub-123, got %s", c.Deploy.SubscriptionID)
				}
				if c.Deploy.ResourceGroup != "rg-app" {
					t.Errorf("Expected resource group rg-app, got %s", c.Deploy.ResourceGroup)
				}
				if c.Deploy.Environment != "env-app" {
					t.Errorf("Expected environment env-app, got %s", c.Deploy.Environment)
				}
				return nil
			},
		},
		{
			name:       "Missing config",
			configYAML: "",
			wantErr:    true,
		},
		{
			name: "Invalid YAML",
			configYAML: `version: 1
framework vite
invalid yaml
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if tt.configYAML != "" {
				configDir := filepath.Join(tmpDir, ConfigDir)
				if err := os.MkdirAll(configDir, 0755); err != nil {
					t.Fatalf("Failed to create config dir: %v", err)
				}

				configPath := filepath.Join(configDir, ConfigFile)
				if err := os.WriteFile(configPath, []byte(tt.configYAML), 0644); err != nil {
					t.Fatalf("Failed to write config file: %v", err)
				}
			}

			cfg, err := Load(tmpDir)

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

			if tt.checkFunc != nil {
				if err := tt.checkFunc(cfg); err != nil {
					t.Errorf("Check function failed: %v", err)
				}
			}
		})
	}
}

func TestSave(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "Save complete config",
			config: &Config{
				Version:   1,
				Framework: "vite",
				Target:    "do",
			},
		},
		{
			name: "Save with detected values",
			config: &Config{
				Version:   1,
				Framework: "nextjs",
				Target:    "aws",
			},
		},
		{
			name: "Save with GCP deploy settings",
			config: &Config{
				Version:   1,
				Framework: "vite",
				Target:    "gcp",
			},
		},
		{
			name: "Save with Azure deploy settings",
			config: &Config{
				Version:   1,
				Framework: "vite",
				Target:    "azure",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Set detected values
			tt.config.Detected.PackageManager = "npm"
			tt.config.Detected.NodeVersion = "20"
			if tt.config.Target == "gcp" {
				tt.config.Deploy.ProjectID = "demo-project"
				tt.config.Deploy.Region = "us-central1"
			}
			if tt.config.Target == "azure" {
				tt.config.Deploy.SubscriptionID = "sub-123"
				tt.config.Deploy.ResourceGroup = "rg-app"
				tt.config.Deploy.Environment = "env-app"
				tt.config.Deploy.Region = "eastus"
			}

			err := tt.config.Save(tmpDir)
			if err != nil {
				t.Fatalf("Failed to save config: %v", err)
			}

			// Verify file was created
			configPath := filepath.Join(tmpDir, ConfigDir, ConfigFile)
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				t.Error("Config file was not created")
			}

			// Load it back and verify
			loaded, err := Load(tmpDir)
			if err != nil {
				t.Fatalf("Failed to load saved config: %v", err)
			}

			if loaded.Version != tt.config.Version {
				t.Errorf("Expected version %d, got %d", tt.config.Version, loaded.Version)
			}
			if loaded.Framework != tt.config.Framework {
				t.Errorf("Expected framework %s, got %s", tt.config.Framework, loaded.Framework)
			}
			if loaded.Target != tt.config.Target {
				t.Errorf("Expected target %s, got %s", tt.config.Target, loaded.Target)
			}
			if loaded.Deploy.ProjectID != tt.config.Deploy.ProjectID {
				t.Errorf("Expected project ID %s, got %s", tt.config.Deploy.ProjectID, loaded.Deploy.ProjectID)
			}
			if loaded.Deploy.SubscriptionID != tt.config.Deploy.SubscriptionID {
				t.Errorf("Expected subscription ID %s, got %s", tt.config.Deploy.SubscriptionID, loaded.Deploy.SubscriptionID)
			}
			if loaded.Deploy.ResourceGroup != tt.config.Deploy.ResourceGroup {
				t.Errorf("Expected resource group %s, got %s", tt.config.Deploy.ResourceGroup, loaded.Deploy.ResourceGroup)
			}
			if loaded.Deploy.Environment != tt.config.Deploy.Environment {
				t.Errorf("Expected environment %s, got %s", tt.config.Deploy.Environment, loaded.Deploy.Environment)
			}
		})
	}
}

func TestNewFromDetection(t *testing.T) {
	tests := []struct {
		name      string
		detection *detect.Detection
		target    string
		expected  *Config
	}{
		{
			name: "Vite project",
			detection: &detect.Detection{
				Framework:      detect.Vite,
				PackageManager: detect.NPM,
				NodeVersion:    "20",
				BuildCommand:   "npm run build",
				OutputDir:      "dist",
				OutputType:     detect.Static,
			},
			target: "do",
			expected: &Config{
				Version:   1,
				Framework: "vite",
				Target:    "do",
			},
		},
		{
			name: "Next.js project",
			detection: &detect.Detection{
				Framework:      detect.NextJS,
				PackageManager: detect.Yarn,
				NodeVersion:    "18",
				BuildCommand:   "yarn build",
				OutputDir:      ".next",
				OutputType:     detect.SSR,
			},
			target: "aws",
			expected: &Config{
				Version:   1,
				Framework: "nextjs",
				Target:    "aws",
			},
		},
		{
			name: "GCP target preserved",
			detection: &detect.Detection{
				Framework:      detect.Vite,
				PackageManager: detect.PNPM,
				NodeVersion:    "20",
				BuildCommand:   "pnpm build",
				OutputDir:      "dist",
				OutputType:     detect.Static,
			},
			target: "gcp",
			expected: &Config{
				Version:   1,
				Framework: "vite",
				Target:    "gcp",
			},
		},
		{
			name: "Azure target preserved",
			detection: &detect.Detection{
				Framework:      detect.Vite,
				PackageManager: detect.PNPM,
				NodeVersion:    "20",
				BuildCommand:   "pnpm build",
				OutputDir:      "dist",
				OutputType:     detect.Static,
			},
			target: "azure",
			expected: &Config{
				Version:   1,
				Framework: "vite",
				Target:    "azure",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewFromDetection(tt.detection, tt.target)

			if cfg.Version != tt.expected.Version {
				t.Errorf("Expected version %d, got %d", tt.expected.Version, cfg.Version)
			}
			if cfg.Framework != tt.expected.Framework {
				t.Errorf("Expected framework %s, got %s", tt.expected.Framework, cfg.Framework)
			}
			if cfg.Target != tt.expected.Target {
				t.Errorf("Expected target %s, got %s", tt.expected.Target, cfg.Target)
			}

			// Check detected values
			if cfg.Detected.PackageManager != string(tt.detection.PackageManager) {
				t.Errorf("Expected package manager %s, got %s",
					tt.detection.PackageManager, cfg.Detected.PackageManager)
			}
			if cfg.Detected.NodeVersion != tt.detection.NodeVersion {
				t.Errorf("Expected node version %s, got %s",
					tt.detection.NodeVersion, cfg.Detected.NodeVersion)
			}
			if cfg.Detected.OutputType != string(tt.detection.OutputType) {
				t.Errorf("Expected output type %s, got %s",
					tt.detection.OutputType, cfg.Detected.OutputType)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid vite config",
			config: &Config{
				Version:   1,
				Framework: "vite",
				Target:    "do",
			},
			wantErr: false,
		},
		{
			name: "Valid nextjs config",
			config: &Config{
				Version:   1,
				Framework: "nextjs",
				Target:    "aws",
			},
			wantErr: false,
		},
		{
			name: "Valid gcp config",
			config: &Config{
				Version:   1,
				Framework: "vite",
				Target:    "gcp",
			},
			wantErr: false,
		},
		{
			name: "Valid azure config",
			config: &Config{
				Version:   1,
				Framework: "vite",
				Target:    "azure",
			},
			wantErr: false,
		},
		{
			name: "Invalid version",
			config: &Config{
				Version:   2,
				Framework: "vite",
				Target:    "do",
			},
			wantErr: true,
			errMsg:  "unsupported config version",
		},
		{
			name: "Missing framework",
			config: &Config{
				Version: 1,
				Target:  "do",
			},
			wantErr: true,
			errMsg:  "framework not set",
		},
		{
			name: "Invalid framework",
			config: &Config{
				Version:   1,
				Framework: "react",
				Target:    "do",
			},
			wantErr: true,
			errMsg:  "unsupported framework",
		},
		{
			name: "Invalid target",
			config: &Config{
				Version:   1,
				Framework: "vite",
				Target:    "heroku",
			},
			wantErr: true,
			errMsg:  "unsupported target",
		},
		{
			name: "Empty target is allowed",
			config: &Config{
				Version:   1,
				Framework: "vite",
				Target:    "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error message containing %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestIsStatic(t *testing.T) {
	tests := []struct {
		name       string
		outputType string
		expected   bool
	}{
		{
			name:       "Static output",
			outputType: "static",
			expected:   true,
		},
		{
			name:       "SSR output",
			outputType: "ssr",
			expected:   false,
		},
		{
			name:       "Empty output type",
			outputType: "",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			cfg.Detected.OutputType = tt.outputType

			result := cfg.IsStatic()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetFramework(t *testing.T) {
	tests := []struct {
		name      string
		framework string
		expected  detect.Framework
	}{
		{
			name:      "Vite framework",
			framework: "vite",
			expected:  detect.Vite,
		},
		{
			name:      "Next.js framework",
			framework: "nextjs",
			expected:  detect.NextJS,
		},
		{
			name:      "Unknown framework",
			framework: "angular",
			expected:  detect.Unknown,
		},
		{
			name:      "Empty framework",
			framework: "",
			expected:  detect.Unknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Framework: tt.framework,
			}

			result := cfg.GetFramework()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConfigRoundTrip(t *testing.T) {
	// Create a config with all fields populated
	original := &Config{
		Version:   1,
		Framework: "vite",
		Target:    "do",
	}
	original.Detected.PackageManager = "npm"
	original.Detected.BuildCommand = "npm run build"
	original.Detected.OutputDir = "dist"
	original.Detected.NodeVersion = "20"
	original.Detected.OutputType = "static"
	original.Deploy.AppName = "my-app"
	original.Deploy.Region = "nyc"

	tmpDir := t.TempDir()

	// Save
	if err := original.Save(tmpDir); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Load
	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify all fields match
	if loaded.Version != original.Version {
		t.Errorf("Version mismatch: expected %d, got %d", original.Version, loaded.Version)
	}
	if loaded.Framework != original.Framework {
		t.Errorf("Framework mismatch: expected %s, got %s", original.Framework, loaded.Framework)
	}
	if loaded.Target != original.Target {
		t.Errorf("Target mismatch: expected %s, got %s", original.Target, loaded.Target)
	}
	if loaded.Detected.PackageManager != original.Detected.PackageManager {
		t.Errorf("PackageManager mismatch: expected %s, got %s",
			original.Detected.PackageManager, loaded.Detected.PackageManager)
	}
	if loaded.Deploy.AppName != original.Deploy.AppName {
		t.Errorf("AppName mismatch: expected %s, got %s",
			original.Deploy.AppName, loaded.Deploy.AppName)
	}
}
