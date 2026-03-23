package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewGCPDeployer(t *testing.T) {
	originalLookPath := gcloudLookPath
	gcloudLookPath = func(string) (string, error) {
		return "/usr/local/bin/gcloud", nil
	}
	t.Cleanup(func() {
		gcloudLookPath = originalLookPath
		_ = os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
		_ = os.Unsetenv("GCP_PROJECT_ID")
	})

	credentialsFile := filepath.Join(t.TempDir(), "service-account.json")
	if err := os.WriteFile(credentialsFile, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}

	tests := []struct {
		name          string
		credentials   string
		projectID     string
		projectEnv    string
		region        string
		wantRegion    string
		wantService   string
		wantErrSubstr string
	}{
		{
			name:        "valid config",
			credentials: credentialsFile,
			projectID:   "demo-project",
			region:      "us-west1",
			wantRegion:  "us-west1",
			wantService: "my-app",
		},
		{
			name:        "project falls back to env",
			credentials: credentialsFile,
			projectEnv:  "env-project",
			wantRegion:  defaultGCPRegion,
			wantService: "my-app",
		},
		{
			name:          "missing credentials env",
			wantErrSubstr: "GOOGLE_APPLICATION_CREDENTIALS",
		},
		{
			name:          "missing project id",
			credentials:   credentialsFile,
			wantErrSubstr: "GCP project ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.credentials == "" {
				_ = os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
			} else {
				_ = os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", tt.credentials)
			}

			if tt.projectEnv == "" {
				_ = os.Unsetenv("GCP_PROJECT_ID")
			} else {
				_ = os.Setenv("GCP_PROJECT_ID", tt.projectEnv)
			}

			deployer, err := NewGCPDeployer("My App", tt.projectID, tt.region)
			if tt.wantErrSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErrSubstr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewGCPDeployer returned error: %v", err)
			}

			if deployer.Region != tt.wantRegion {
				t.Fatalf("expected region %q, got %q", tt.wantRegion, deployer.Region)
			}
			if deployer.ServiceName != tt.wantService {
				t.Fatalf("expected service name %q, got %q", tt.wantService, deployer.ServiceName)
			}
		})
	}
}

func TestGCPDeployCreateService(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatalf("write Dockerfile: %v", err)
	}

	credentialsFile := filepath.Join(tmpDir, "service-account.json")
	if err := os.WriteFile(credentialsFile, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	var calls [][]string
	deployer := &GCPDeployer{
		CredentialsFile: credentialsFile,
		ProjectID:       "demo-project",
		Region:          "us-central1",
		AppName:         "My App",
		ServiceName:     "my-app",
		runCommand: func(_ context.Context, workdir string, env []string, args ...string) ([]byte, error) {
			if workdir != "." {
				t.Fatalf("expected workdir '.', got %q", workdir)
			}
			if !containsEnvValue(env, "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE="+credentialsFile) {
				t.Fatalf("missing credential override in env: %v", env)
			}

			calls = append(calls, append([]string(nil), args...))
			switch len(calls) {
			case 1:
				return nil, fmt.Errorf("service not found")
			case 2:
				if !containsArg(args, "--source", ".") {
					t.Fatalf("deploy args missing source flag: %v", args)
				}
				if !containsArg(args, "--port", "80") {
					t.Fatalf("deploy args missing static port: %v", args)
				}
				envFile := valueAfterArg(args, "--env-vars-file")
				if envFile == "" {
					t.Fatalf("expected --env-vars-file in deploy args: %v", args)
				}
				data, err := os.ReadFile(envFile)
				if err != nil {
					t.Fatalf("read env file: %v", err)
				}
				content := string(data)
				if !strings.Contains(content, "API_KEY: abc123") || !strings.Contains(content, "MODE: production") {
					t.Fatalf("unexpected env file content: %s", content)
				}
				return []byte(`{}`), nil
			case 3:
				return []byte(`{"metadata":{"name":"my-app"},"status":{"url":"https://my-app-abc.a.run.app"}}`), nil
			default:
				t.Fatalf("unexpected extra gcloud call: %v", args)
				return nil, nil
			}
		},
	}

	result, err := deployer.Deploy(true, map[string]string{
		"API_KEY": "abc123",
		"MODE":    "production",
	})
	if err != nil {
		t.Fatalf("Deploy returned error: %v", err)
	}

	if result.WasUpdate {
		t.Fatalf("expected create path, got update")
	}
	if result.Service.URL != "https://my-app-abc.a.run.app" {
		t.Fatalf("expected service URL to be populated, got %q", result.Service.URL)
	}
	if !strings.Contains(result.ConsoleURL, "project=demo-project") {
		t.Fatalf("expected console URL to contain project, got %q", result.ConsoleURL)
	}
}

func TestGCPDeployUpdateService(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatalf("write Dockerfile: %v", err)
	}

	credentialsFile := filepath.Join(tmpDir, "service-account.json")
	if err := os.WriteFile(credentialsFile, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	describeResponse := []byte(`{"metadata":{"name":"next-app"},"status":{"url":"https://next-app-xyz.a.run.app"}}`)
	callCount := 0
	deployer := &GCPDeployer{
		CredentialsFile: credentialsFile,
		ProjectID:       "demo-project",
		Region:          "europe-west1",
		AppName:         "Next App",
		ServiceName:     "next-app",
		runCommand: func(_ context.Context, _ string, _ []string, args ...string) ([]byte, error) {
			callCount++
			switch callCount {
			case 1:
				return describeResponse, nil
			case 2:
				if !containsArg(args, "--port", "3000") {
					t.Fatalf("deploy args missing SSR port: %v", args)
				}
				return []byte(`{}`), nil
			case 3:
				return describeResponse, nil
			default:
				t.Fatalf("unexpected extra gcloud call: %v", args)
				return nil, nil
			}
		},
	}

	result, err := deployer.Deploy(false, nil)
	if err != nil {
		t.Fatalf("Deploy returned error: %v", err)
	}
	if !result.WasUpdate {
		t.Fatalf("expected update path")
	}
	if result.Service.Name != "next-app" {
		t.Fatalf("expected service name next-app, got %q", result.Service.Name)
	}
}

func containsEnvValue(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func containsArg(args []string, flag, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func valueAfterArg(args []string, flag string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}
