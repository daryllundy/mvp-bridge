package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewAzureDeployer(t *testing.T) {
	originalLookPath := azLookPath
	originalRunner := azDefaultRunner
	azLookPath = func(string) (string, error) { return "/usr/local/bin/az", nil }
	t.Cleanup(func() {
		azLookPath = originalLookPath
		azDefaultRunner = originalRunner
	})

	tests := []struct {
		name             string
		appName          string
		resourceGroup    string
		environment      string
		region           string
		subscriptionID   string
		runner           azureRunner
		wantErrSubstr    string
		wantAppName      string
		wantRegion       string
		wantRG           string
		wantEnvironment  string
		wantSubscription string
	}{
		{
			name:           "explicit config values",
			appName:        "My App",
			resourceGroup:  "rg-prod",
			environment:    "env-prod",
			region:         "westus3",
			subscriptionID: "sub-123",
			runner: func(_ context.Context, _ string, _ []string, args ...string) ([]byte, error) {
				if containsArg(args, "account", "show") || (len(args) >= 2 && args[0] == "account" && args[1] == "show") {
					return []byte(`{}`), nil
				}
				if containsArg(args, "extension", "show") || (len(args) >= 3 && args[0] == "extension" && args[1] == "show") {
					return []byte(`{}`), nil
				}
				return []byte(`{}`), nil
			},
			wantAppName:      "my-app",
			wantRegion:       "westus3",
			wantRG:           "rg-prod",
			wantEnvironment:  "env-prod",
			wantSubscription: "sub-123",
		},
		{
			name:    "defaults applied",
			appName: "My Sample App",
			runner: func(_ context.Context, _ string, _ []string, _ ...string) ([]byte, error) {
				return []byte(`{}`), nil
			},
			wantAppName:      "my-sample-app",
			wantRegion:       defaultAzureRegion,
			wantRG:           "mvpbridge-my-sample-app-rg",
			wantEnvironment:  "mvpbridge-my-sample-app-env",
			wantSubscription: "",
		},
		{
			name:          "account show failure surfaces login error",
			appName:       "app",
			wantErrSubstr: "az login",
			runner: func(_ context.Context, _ string, _ []string, args ...string) ([]byte, error) {
				if len(args) >= 2 && args[0] == "account" && args[1] == "show" {
					return nil, fmt.Errorf("please run az login")
				}
				return []byte(`{}`), nil
			},
		},
		{
			name:          "extension install failure surfaces",
			appName:       "app",
			wantErrSubstr: "containerapp CLI extension",
			runner: func(_ context.Context, _ string, _ []string, args ...string) ([]byte, error) {
				if len(args) >= 3 && args[0] == "extension" && args[1] == "show" {
					return nil, fmt.Errorf("not found")
				}
				if len(args) >= 3 && args[0] == "extension" && args[1] == "add" {
					return nil, fmt.Errorf("install failed")
				}
				return []byte(`{}`), nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			azDefaultRunner = tt.runner

			deployer, err := NewAzureDeployer(tt.appName, tt.resourceGroup, tt.environment, tt.region, tt.subscriptionID)
			if tt.wantErrSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErrSubstr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewAzureDeployer returned error: %v", err)
			}

			if deployer.AppName != tt.wantAppName {
				t.Fatalf("expected app name %q, got %q", tt.wantAppName, deployer.AppName)
			}
			if deployer.Region != tt.wantRegion {
				t.Fatalf("expected region %q, got %q", tt.wantRegion, deployer.Region)
			}
			if deployer.ResourceGroup != tt.wantRG {
				t.Fatalf("expected resource group %q, got %q", tt.wantRG, deployer.ResourceGroup)
			}
			if deployer.Environment != tt.wantEnvironment {
				t.Fatalf("expected environment %q, got %q", tt.wantEnvironment, deployer.Environment)
			}
			if deployer.SubscriptionID != tt.wantSubscription {
				t.Fatalf("expected subscription %q, got %q", tt.wantSubscription, deployer.SubscriptionID)
			}
		})
	}
}

func TestNewAzureDeployerInstallsContainerAppExtensionWhenMissing(t *testing.T) {
	originalLookPath := azLookPath
	originalRunner := azDefaultRunner
	azLookPath = func(string) (string, error) { return "/usr/local/bin/az", nil }
	t.Cleanup(func() {
		azLookPath = originalLookPath
		azDefaultRunner = originalRunner
	})

	calls := 0
	azDefaultRunner = func(_ context.Context, _ string, _ []string, args ...string) ([]byte, error) {
		calls++
		switch calls {
		case 1:
			if args[0] != "account" || args[1] != "show" {
				t.Fatalf("expected account show, got %v", args)
			}
			return []byte(`{}`), nil
		case 2:
			if args[0] != "extension" || args[1] != "show" {
				t.Fatalf("expected extension show, got %v", args)
			}
			return nil, fmt.Errorf("not found")
		case 3:
			if args[0] != "extension" || args[1] != "add" {
				t.Fatalf("expected extension add, got %v", args)
			}
			if !containsArg(args, "--name", "containerapp") || !containsArg(args, "--upgrade", "--output") {
				t.Fatalf("expected extension install flags, got %v", args)
			}
			return []byte(`{}`), nil
		default:
			t.Fatalf("unexpected call %d: %v", calls, args)
			return nil, nil
		}
	}

	if _, err := NewAzureDeployer("my app", "", "", "", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAzureDeployStaticWithEnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatalf("write Dockerfile: %v", err)
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
	deployer := &AzureDeployer{
		AppName:        "my-app",
		ResourceGroup:  "rg-01",
		Environment:    "env-01",
		Region:         "eastus",
		SubscriptionID: "sub-01",
		runCommand: func(_ context.Context, workdir string, _ []string, args ...string) ([]byte, error) {
			if workdir != "." {
				t.Fatalf("expected workdir '.', got %q", workdir)
			}
			calls = append(calls, append([]string(nil), args...))
			switch len(calls) {
			case 1:
				if args[0] != "containerapp" || args[1] != "up" {
					t.Fatalf("expected containerapp up, got %v", args)
				}
				if !containsArg(args, "--target-port", "80") {
					t.Fatalf("expected static port 80, got %v", args)
				}
				if !containsArg(args, "--source", ".") {
					t.Fatalf("expected source '.', got %v", args)
				}
				if !containsArg(args, "--subscription", "sub-01") {
					t.Fatalf("expected subscription flag, got %v", args)
				}
				return []byte(`{}`), nil
			case 2:
				if args[0] != "containerapp" || args[1] != "update" {
					t.Fatalf("expected containerapp update, got %v", args)
				}
				if !containsArg(args, "--set-env-vars", "API_KEY=abc123") {
					t.Fatalf("expected API_KEY env var, got %v", args)
				}
				joined := strings.Join(args, " ")
				if !strings.Contains(joined, "MODE=prod") {
					t.Fatalf("expected MODE env var, got %v", args)
				}
				return []byte(`{}`), nil
			case 3:
				return []byte(`{"name":"my-app","location":"eastus","properties":{"configuration":{"ingress":{"fqdn":"my-app.green-field.azurecontainerapps.io"}},"managedEnvironmentId":"/subscriptions/sub-01/resourceGroups/rg-01/providers/Microsoft.App/managedEnvironments/env-01"}}`), nil
			default:
				t.Fatalf("unexpected extra call: %v", args)
				return nil, nil
			}
		},
	}

	result, err := deployer.Deploy(true, map[string]string{
		"MODE":    "prod",
		"API_KEY": "abc123",
	})
	if err != nil {
		t.Fatalf("Deploy returned error: %v", err)
	}

	if result.App.Name != "my-app" {
		t.Fatalf("expected app name my-app, got %q", result.App.Name)
	}
	if result.App.URL != "https://my-app.green-field.azurecontainerapps.io" {
		t.Fatalf("expected URL from fqdn, got %q", result.App.URL)
	}
	if result.App.Environment != "env-01" {
		t.Fatalf("expected env-01 environment, got %q", result.App.Environment)
	}
	if result.ConsoleURL == "" || !strings.Contains(result.ConsoleURL, "subscriptions/sub-01") {
		t.Fatalf("expected subscription-scoped console URL, got %q", result.ConsoleURL)
	}
}

func TestAzureDeploySSRWithoutEnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatalf("write Dockerfile: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	callCount := 0
	deployer := &AzureDeployer{
		AppName:       "next-app",
		ResourceGroup: "rg-ssr",
		Environment:   "env-ssr",
		Region:        "westus2",
		runCommand: func(_ context.Context, _ string, _ []string, args ...string) ([]byte, error) {
			callCount++
			switch callCount {
			case 1:
				if !containsArg(args, "--target-port", "3000") {
					t.Fatalf("expected SSR port 3000, got %v", args)
				}
				return []byte(`{}`), nil
			case 2:
				return []byte(`{"name":"next-app","location":"westus2","properties":{"configuration":{"ingress":{"fqdn":"next-app.azurecontainerapps.io"}}}}`), nil
			default:
				t.Fatalf("unexpected call count %d", callCount)
				return nil, nil
			}
		},
	}

	result, err := deployer.Deploy(false, nil)
	if err != nil {
		t.Fatalf("Deploy returned error: %v", err)
	}
	if result.App.URL != "https://next-app.azurecontainerapps.io" {
		t.Fatalf("expected URL from fqdn, got %q", result.App.URL)
	}
}

func TestAzureDeployRequiresDockerfile(t *testing.T) {
	tmpDir := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	deployer := &AzureDeployer{
		AppName:       "my-app",
		ResourceGroup: "rg",
		Environment:   "env",
		Region:        "eastus",
		runCommand: func(_ context.Context, _ string, _ []string, _ ...string) ([]byte, error) {
			return nil, nil
		},
	}

	_, err = deployer.Deploy(true, nil)
	if err == nil || !strings.Contains(err.Error(), "run 'mvpbridge normalize' first") {
		t.Fatalf("expected normalize-first error, got %v", err)
	}
}

func TestNewAzureDeployerMissingAz(t *testing.T) {
	originalLookPath := azLookPath
	azLookPath = func(string) (string, error) { return "", fmt.Errorf("not found") }
	t.Cleanup(func() { azLookPath = originalLookPath })

	_, err := NewAzureDeployer("app", "", "", "", "")
	if err == nil || !strings.Contains(err.Error(), "az CLI not found in PATH") {
		t.Fatalf("expected missing az error, got %v", err)
	}
}
