package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daryllundy/mvp-bridge/internal/config"
	"github.com/daryllundy/mvp-bridge/internal/deploy"
	"github.com/daryllundy/mvp-bridge/internal/detect"
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

func TestRunDeployUnknownTargetListsAzure(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".mvpbridge")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("version: 1\nframework: vite\ntarget: do\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	err = runDeploy("heroku")
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if !strings.Contains(err.Error(), "supported: do, aws, gcp, azure, local") {
		t.Fatalf("expected error to list azure support, got %q", err.Error())
	}
}

func TestDeployLocalDispatchesToGenerator(t *testing.T) {
	originalPrepare := prepareDeployFunc
	originalGenerator := generateLocalWorkspaceFunc
	t.Cleanup(func() {
		prepareDeployFunc = originalPrepare
		generateLocalWorkspaceFunc = originalGenerator
	})

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".mvpbridge")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("version: 1\nframework: vite\ntarget: local\ndetected:\n  output_type: static\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{"scripts":{"build":"npm run build"},"devDependencies":{"vite":"^5.0.0"}}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "vite.config.js"), []byte("export default {}"), 0o644); err != nil {
		t.Fatalf("write vite config: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	prepareDeployFunc = func(cfg *config.Config) (*deployPreparation, error) {
		return &deployPreparation{
			RepoURL: "https://github.com/acme/repo",
			AppName: deriveAppName(cfg.Deploy.AppName, "https://github.com/acme/repo"),
			EnvVars: map[string]string{"API_URL": "https://example.com"},
		}, nil
	}

	var gotOpts deploy.LocalWorkspaceOptions
	generateLocalWorkspaceFunc = func(opts deploy.LocalWorkspaceOptions) (*deploy.LocalWorkspaceResult, error) {
		gotOpts = opts
		return &deploy.LocalWorkspaceResult{
			OutputDir:   filepath.Join(tmpDir, "local-deploy"),
			Persistence: detect.Persistence{Kind: detect.PersistenceNone},
		}, nil
	}

	if err := runDeploy("local"); err != nil {
		t.Fatalf("runDeploy local returned error: %v", err)
	}

	if gotOpts.Framework != detect.Vite {
		t.Fatalf("expected Vite framework, got %s", gotOpts.Framework)
	}
	if gotOpts.OutputType != detect.Static {
		t.Fatalf("expected static output type, got %s", gotOpts.OutputType)
	}
	if gotOpts.BuildCommand != "npm run build" {
		t.Fatalf("expected build command to be forwarded, got %q", gotOpts.BuildCommand)
	}
}

type fakeGCPDeployer struct {
	deployFn func(isStatic bool, envVars map[string]string) (*deploy.CloudRunServiceResponse, error)
}

func (f fakeGCPDeployer) Deploy(isStatic bool, envVars map[string]string) (*deploy.CloudRunServiceResponse, error) {
	return f.deployFn(isStatic, envVars)
}

type fakeAzureDeployer struct {
	deployFn func(isStatic bool, envVars map[string]string) (*deploy.AzureContainerAppResponse, error)
}

func (f fakeAzureDeployer) Deploy(isStatic bool, envVars map[string]string) (*deploy.AzureContainerAppResponse, error) {
	return f.deployFn(isStatic, envVars)
}

func TestDeployGCPUsesExplicitConfigValues(t *testing.T) {
	originalPrepare := prepareDeployFunc
	originalConstructor := newGCPDeployerFunc
	t.Cleanup(func() {
		prepareDeployFunc = originalPrepare
		newGCPDeployerFunc = originalConstructor
	})

	cfg := &config.Config{
		Version:   1,
		Framework: "vite",
		Target:    "gcp",
	}
	cfg.Detected.OutputType = "static"
	cfg.Deploy.AppName = "configured-name"
	cfg.Deploy.ProjectID = "cfg-project"
	cfg.Deploy.Region = "us-west1"

	prepareDeployFunc = func(cfg *config.Config) (*deployPreparation, error) {
		return &deployPreparation{
			RepoURL: "https://github.com/acme/repo",
			AppName: deriveAppName(cfg.Deploy.AppName, "https://github.com/acme/repo"),
			EnvVars: map[string]string{"TOKEN": "abc"},
		}, nil
	}

	var gotAppName, gotProjectID, gotRegion string
	newGCPDeployerFunc = func(appName, projectID, region string) (gcpDeployer, error) {
		gotAppName = appName
		gotProjectID = projectID
		gotRegion = region
		return fakeGCPDeployer{
			deployFn: func(isStatic bool, envVars map[string]string) (*deploy.CloudRunServiceResponse, error) {
				if !isStatic {
					t.Fatalf("expected static deployment")
				}
				if envVars["TOKEN"] != "abc" {
					t.Fatalf("expected env vars to be forwarded, got %v", envVars)
				}
				var resp deploy.CloudRunServiceResponse
				resp.Service.URL = "https://configured-name.a.run.app"
				resp.ConsoleURL = "https://console.cloud.google.com/run/detail/us-west1/configured-name/metrics?project=cfg-project"
				return &resp, nil
			},
		}, nil
	}

	output := captureStdout(t, func() {
		if err := deployGCP(cfg); err != nil {
			t.Fatalf("deployGCP returned error: %v", err)
		}
	})

	if gotAppName != "configured-name" {
		t.Fatalf("expected configured app name, got %q", gotAppName)
	}
	if gotProjectID != "cfg-project" {
		t.Fatalf("expected config project ID, got %q", gotProjectID)
	}
	if gotRegion != "us-west1" {
		t.Fatalf("expected config region, got %q", gotRegion)
	}
	if !strings.Contains(output, "App URL: https://configured-name.a.run.app") {
		t.Fatalf("expected app URL in output, got %q", output)
	}
	if !strings.Contains(output, "Console: https://console.cloud.google.com/run/detail/us-west1/configured-name/metrics?project=cfg-project") {
		t.Fatalf("expected console URL in output, got %q", output)
	}
}

func TestDeployGCPUsesEnvFallbacks(t *testing.T) {
	originalPrepare := prepareDeployFunc
	originalConstructor := newGCPDeployerFunc
	originalProjectEnv := os.Getenv("GCP_PROJECT_ID")
	t.Cleanup(func() {
		prepareDeployFunc = originalPrepare
		newGCPDeployerFunc = originalConstructor
		_ = os.Setenv("GCP_PROJECT_ID", originalProjectEnv)
	})

	cfg := &config.Config{
		Version:   1,
		Framework: "nextjs",
		Target:    "gcp",
	}
	cfg.Detected.OutputType = "ssr"
	_ = os.Setenv("GCP_PROJECT_ID", "env-project")

	prepareDeployFunc = func(cfg *config.Config) (*deployPreparation, error) {
		return &deployPreparation{
			RepoURL: "https://github.com/acme/service",
			AppName: deriveAppName(cfg.Deploy.AppName, "https://github.com/acme/service"),
			EnvVars: map[string]string{},
		}, nil
	}

	var gotAppName, gotProjectID, gotRegion string
	newGCPDeployerFunc = func(appName, projectID, region string) (gcpDeployer, error) {
		gotAppName = appName
		gotProjectID = projectID
		gotRegion = region
		return fakeGCPDeployer{
			deployFn: func(isStatic bool, envVars map[string]string) (*deploy.CloudRunServiceResponse, error) {
				if isStatic {
					t.Fatalf("expected SSR deployment")
				}
				var resp deploy.CloudRunServiceResponse
				resp.Service.URL = "https://service.a.run.app"
				resp.ConsoleURL = "https://console.cloud.google.com/run/detail/us-central1/service/metrics?project=env-project"
				return &resp, nil
			},
		}, nil
	}

	if err := deployGCP(cfg); err != nil {
		t.Fatalf("deployGCP returned error: %v", err)
	}

	if gotAppName != "service" {
		t.Fatalf("expected derived app name from repo, got %q", gotAppName)
	}
	if gotProjectID != "env-project" {
		t.Fatalf("expected env project ID, got %q", gotProjectID)
	}
	if gotRegion != "us-central1" {
		t.Fatalf("expected default region, got %q", gotRegion)
	}
}

func TestDeployGCPSurfacesConstructorFailure(t *testing.T) {
	originalPrepare := prepareDeployFunc
	originalConstructor := newGCPDeployerFunc
	t.Cleanup(func() {
		prepareDeployFunc = originalPrepare
		newGCPDeployerFunc = originalConstructor
	})

	cfg := &config.Config{Version: 1, Framework: "vite", Target: "gcp"}
	cfg.Detected.OutputType = "static"

	prepareDeployFunc = func(_ *config.Config) (*deployPreparation, error) {
		return &deployPreparation{
			RepoURL: "https://github.com/acme/repo",
			AppName: "repo",
			EnvVars: map[string]string{},
		}, nil
	}

	newGCPDeployerFunc = func(appName, projectID, region string) (gcpDeployer, error) {
		return nil, fmt.Errorf("missing project id")
	}

	err := deployGCP(cfg)
	if err == nil {
		t.Fatal("expected constructor error")
	}
	if !strings.Contains(err.Error(), "missing project id") {
		t.Fatalf("expected constructor error to surface, got %q", err.Error())
	}
}

func TestDeployGCPSurfacesDeployFailure(t *testing.T) {
	originalPrepare := prepareDeployFunc
	originalConstructor := newGCPDeployerFunc
	t.Cleanup(func() {
		prepareDeployFunc = originalPrepare
		newGCPDeployerFunc = originalConstructor
	})

	cfg := &config.Config{Version: 1, Framework: "vite", Target: "gcp"}
	cfg.Detected.OutputType = "static"
	cfg.Deploy.ProjectID = "cfg-project"

	prepareDeployFunc = func(_ *config.Config) (*deployPreparation, error) {
		return &deployPreparation{
			RepoURL: "https://github.com/acme/repo",
			AppName: "repo",
			EnvVars: map[string]string{"TOKEN": "abc"},
		}, nil
	}

	newGCPDeployerFunc = func(appName, projectID, region string) (gcpDeployer, error) {
		return fakeGCPDeployer{
			deployFn: func(isStatic bool, envVars map[string]string) (*deploy.CloudRunServiceResponse, error) {
				return nil, fmt.Errorf("gcloud failed")
			},
		}, nil
	}

	err := deployGCP(cfg)
	if err == nil {
		t.Fatal("expected deploy error")
	}
	if !strings.Contains(err.Error(), "deployment failed: gcloud failed") {
		t.Fatalf("expected wrapped deploy error, got %q", err.Error())
	}
}

func TestDeployAzureUsesExplicitConfigValues(t *testing.T) {
	originalPrepare := prepareDeployFunc
	originalConstructor := newAzureDeployerFunc
	t.Cleanup(func() {
		prepareDeployFunc = originalPrepare
		newAzureDeployerFunc = originalConstructor
	})

	cfg := &config.Config{
		Version:   1,
		Framework: "vite",
		Target:    "azure",
	}
	cfg.Detected.OutputType = "static"
	cfg.Deploy.AppName = "configured-name"
	cfg.Deploy.SubscriptionID = "sub-001"
	cfg.Deploy.ResourceGroup = "rg-configured"
	cfg.Deploy.Environment = "env-configured"
	cfg.Deploy.Region = "westus3"

	prepareDeployFunc = func(cfg *config.Config) (*deployPreparation, error) {
		return &deployPreparation{
			RepoURL: "https://github.com/acme/repo",
			AppName: deriveAppName(cfg.Deploy.AppName, "https://github.com/acme/repo"),
			EnvVars: map[string]string{"TOKEN": "abc"},
		}, nil
	}

	var gotAppName, gotRG, gotEnv, gotRegion, gotSub string
	newAzureDeployerFunc = func(appName, resourceGroup, environment, region, subscriptionID string) (azureDeployer, error) {
		gotAppName = appName
		gotRG = resourceGroup
		gotEnv = environment
		gotRegion = region
		gotSub = subscriptionID
		return fakeAzureDeployer{
			deployFn: func(isStatic bool, envVars map[string]string) (*deploy.AzureContainerAppResponse, error) {
				if !isStatic {
					t.Fatalf("expected static deployment")
				}
				if envVars["TOKEN"] != "abc" {
					t.Fatalf("expected env vars to be forwarded, got %v", envVars)
				}
				var resp deploy.AzureContainerAppResponse
				resp.App.URL = "https://configured-name.azurecontainerapps.io"
				resp.ConsoleURL = "https://portal.azure.com/#@/resource/subscriptions/sub-001/resourceGroups/rg-configured/providers/Microsoft.App/containerApps/configured-name"
				return &resp, nil
			},
		}, nil
	}

	output := captureStdout(t, func() {
		if err := deployAzure(cfg); err != nil {
			t.Fatalf("deployAzure returned error: %v", err)
		}
	})

	if gotAppName != "configured-name" {
		t.Fatalf("expected configured app name, got %q", gotAppName)
	}
	if gotRG != "rg-configured" {
		t.Fatalf("expected configured resource group, got %q", gotRG)
	}
	if gotEnv != "env-configured" {
		t.Fatalf("expected configured environment, got %q", gotEnv)
	}
	if gotRegion != "westus3" {
		t.Fatalf("expected configured region, got %q", gotRegion)
	}
	if gotSub != "sub-001" {
		t.Fatalf("expected configured subscription, got %q", gotSub)
	}
	if !strings.Contains(output, "App URL: https://configured-name.azurecontainerapps.io") {
		t.Fatalf("expected app URL in output, got %q", output)
	}
	if !strings.Contains(output, "Console: https://portal.azure.com/#@/resource/subscriptions/sub-001/resourceGroups/rg-configured/providers/Microsoft.App/containerApps/configured-name") {
		t.Fatalf("expected console URL in output, got %q", output)
	}
}

func TestDeployAzureUsesDefaultRegionAndDerivedAppName(t *testing.T) {
	originalPrepare := prepareDeployFunc
	originalConstructor := newAzureDeployerFunc
	t.Cleanup(func() {
		prepareDeployFunc = originalPrepare
		newAzureDeployerFunc = originalConstructor
	})

	cfg := &config.Config{
		Version:   1,
		Framework: "nextjs",
		Target:    "azure",
	}
	cfg.Detected.OutputType = "ssr"

	prepareDeployFunc = func(cfg *config.Config) (*deployPreparation, error) {
		return &deployPreparation{
			RepoURL: "https://github.com/acme/service",
			AppName: deriveAppName(cfg.Deploy.AppName, "https://github.com/acme/service"),
			EnvVars: map[string]string{},
		}, nil
	}

	var gotAppName, gotRG, gotEnv, gotRegion, gotSub string
	newAzureDeployerFunc = func(appName, resourceGroup, environment, region, subscriptionID string) (azureDeployer, error) {
		gotAppName = appName
		gotRG = resourceGroup
		gotEnv = environment
		gotRegion = region
		gotSub = subscriptionID
		return fakeAzureDeployer{
			deployFn: func(isStatic bool, envVars map[string]string) (*deploy.AzureContainerAppResponse, error) {
				if isStatic {
					t.Fatalf("expected SSR deployment")
				}
				var resp deploy.AzureContainerAppResponse
				resp.App.URL = "https://service.azurecontainerapps.io"
				resp.ConsoleURL = "https://portal.azure.com"
				return &resp, nil
			},
		}, nil
	}

	if err := deployAzure(cfg); err != nil {
		t.Fatalf("deployAzure returned error: %v", err)
	}

	if gotAppName != "service" {
		t.Fatalf("expected derived app name from repo, got %q", gotAppName)
	}
	if gotRG != "" {
		t.Fatalf("expected empty RG to allow deployer defaults, got %q", gotRG)
	}
	if gotEnv != "" {
		t.Fatalf("expected empty environment to allow deployer defaults, got %q", gotEnv)
	}
	if gotRegion != "eastus" {
		t.Fatalf("expected default region eastus, got %q", gotRegion)
	}
	if gotSub != "" {
		t.Fatalf("expected empty subscription by default, got %q", gotSub)
	}
}

func TestDeployAzureSurfacesConstructorFailure(t *testing.T) {
	originalPrepare := prepareDeployFunc
	originalConstructor := newAzureDeployerFunc
	t.Cleanup(func() {
		prepareDeployFunc = originalPrepare
		newAzureDeployerFunc = originalConstructor
	})

	cfg := &config.Config{Version: 1, Framework: "vite", Target: "azure"}
	cfg.Detected.OutputType = "static"

	prepareDeployFunc = func(_ *config.Config) (*deployPreparation, error) {
		return &deployPreparation{
			RepoURL: "https://github.com/acme/repo",
			AppName: "repo",
			EnvVars: map[string]string{},
		}, nil
	}

	newAzureDeployerFunc = func(appName, resourceGroup, environment, region, subscriptionID string) (azureDeployer, error) {
		return nil, fmt.Errorf("az login required")
	}

	err := deployAzure(cfg)
	if err == nil {
		t.Fatal("expected constructor error")
	}
	if !strings.Contains(err.Error(), "az login required") {
		t.Fatalf("expected constructor error to surface, got %q", err.Error())
	}
}

func TestDeployAzureSurfacesDeployFailure(t *testing.T) {
	originalPrepare := prepareDeployFunc
	originalConstructor := newAzureDeployerFunc
	t.Cleanup(func() {
		prepareDeployFunc = originalPrepare
		newAzureDeployerFunc = originalConstructor
	})

	cfg := &config.Config{Version: 1, Framework: "vite", Target: "azure"}
	cfg.Detected.OutputType = "static"

	prepareDeployFunc = func(_ *config.Config) (*deployPreparation, error) {
		return &deployPreparation{
			RepoURL: "https://github.com/acme/repo",
			AppName: "repo",
			EnvVars: map[string]string{"TOKEN": "abc"},
		}, nil
	}

	newAzureDeployerFunc = func(appName, resourceGroup, environment, region, subscriptionID string) (azureDeployer, error) {
		return fakeAzureDeployer{
			deployFn: func(isStatic bool, envVars map[string]string) (*deploy.AzureContainerAppResponse, error) {
				return nil, fmt.Errorf("azure deploy failed")
			},
		}, nil
	}

	err := deployAzure(cfg)
	if err == nil {
		t.Fatal("expected deploy error")
	}
	if !strings.Contains(err.Error(), "deployment failed: azure deploy failed") {
		t.Fatalf("expected wrapped deploy error, got %q", err.Error())
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}

	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
	}()

	fn()

	_ = writer.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		t.Fatalf("reading stdout: %v", err)
	}
	_ = reader.Close()

	return buf.String()
}
