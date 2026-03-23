package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultGCPRegion = "us-central1"

var gcloudLookPath = exec.LookPath

type gcloudRunner func(ctx context.Context, workdir string, env []string, args ...string) ([]byte, error)

// GCPDeployer handles deployments to Google Cloud Run through the gcloud CLI.
type GCPDeployer struct {
	CredentialsFile string
	ProjectID       string
	Region          string
	AppName         string
	ServiceName     string
	runCommand      gcloudRunner
}

// CloudRunServiceResponse represents the service metadata returned after deployment.
type CloudRunServiceResponse struct {
	Service struct {
		Name      string `json:"name"`
		URL       string `json:"url"`
		Region    string `json:"region"`
		ProjectID string `json:"projectId"`
	} `json:"service"`
	ConsoleURL string `json:"consoleUrl"`
	WasUpdate  bool   `json:"wasUpdate"`
}

// NewGCPDeployer creates a new Cloud Run deployer instance.
func NewGCPDeployer(appName, projectID, region string) (*GCPDeployer, error) {
	credentialsFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if credentialsFile == "" {
		return nil, fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS environment variable must be set")
	}
	// #nosec G304,G703 -- credentials path is intentionally operator-supplied via environment variable.
	if _, err := os.Stat(credentialsFile); err != nil {
		return nil, fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS file not accessible: %w", err)
	}

	if projectID == "" {
		projectID = os.Getenv("GCP_PROJECT_ID")
	}
	if projectID == "" {
		return nil, fmt.Errorf("GCP project ID must be set in config deploy.project_id or GCP_PROJECT_ID")
	}

	if region == "" {
		region = defaultGCPRegion
	}

	if _, err := gcloudLookPath("gcloud"); err != nil {
		return nil, fmt.Errorf("gcloud CLI not found in PATH")
	}

	serviceName := sanitizeCloudRunServiceName(appName)
	if serviceName == "" {
		return nil, fmt.Errorf("unable to derive Cloud Run service name from app name %q", appName)
	}

	return &GCPDeployer{
		CredentialsFile: credentialsFile,
		ProjectID:       projectID,
		Region:          region,
		AppName:         appName,
		ServiceName:     serviceName,
		runCommand:      defaultGcloudRunner,
	}, nil
}

// Deploy creates or updates a Cloud Run service from the current directory.
func (d *GCPDeployer) Deploy(isStatic bool, envVars map[string]string) (*CloudRunServiceResponse, error) {
	if d.runCommand == nil {
		d.runCommand = defaultGcloudRunner
	}

	if _, err := os.Stat("Dockerfile"); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("Dockerfile not found - run 'mvpbridge normalize' first")
		}
		return nil, fmt.Errorf("checking Dockerfile: %w", err)
	}

	exists, err := d.serviceExists(context.Background())
	if err != nil {
		return nil, err
	}

	args := []string{
		"run", "deploy", d.ServiceName,
		"--source", ".",
		"--project", d.ProjectID,
		"--region", d.Region,
		"--platform", "managed",
		"--allow-unauthenticated",
		"--quiet",
		"--format=json",
		"--port", d.cloudRunPort(isStatic),
	}

	cleanup := func() error { return nil }
	if len(envVars) > 0 {
		envFile, err := d.writeEnvVarsFile(envVars)
		if err != nil {
			return nil, err
		}
		cleanup = func() error {
			if err := os.Remove(envFile); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		defer func() {
			// Best-effort cleanup; deployment result should not be masked by temp file removal.
			if err := cleanup(); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "warning: failed to remove temp env file: %v\n", err)
			}
		}()
		args = append(args, "--env-vars-file", envFile)
	}

	if _, err := d.run(context.Background(), ".", args...); err != nil {
		return nil, err
	}

	response, err := d.describeService(context.Background())
	if err != nil {
		return nil, err
	}
	response.WasUpdate = exists

	return response, nil
}

func (d *GCPDeployer) serviceExists(ctx context.Context) (bool, error) {
	_, err := d.run(ctx, ".", "run", "services", "describe", d.ServiceName,
		"--project", d.ProjectID,
		"--region", d.Region,
		"--platform", "managed",
		"--format=json",
	)
	if err == nil {
		return true, nil
	}
	if isCloudRunNotFoundError(err) {
		return false, nil
	}
	return false, fmt.Errorf("checking existing Cloud Run service: %w", err)
}

func (d *GCPDeployer) describeService(ctx context.Context) (*CloudRunServiceResponse, error) {
	output, err := d.run(ctx, ".", "run", "services", "describe", d.ServiceName,
		"--project", d.ProjectID,
		"--region", d.Region,
		"--platform", "managed",
		"--format=json",
	)
	if err != nil {
		return nil, fmt.Errorf("describing Cloud Run service: %w", err)
	}

	var result struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Status struct {
			URL string `json:"url"`
		} `json:"status"`
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parsing Cloud Run service response: %w", err)
	}

	serviceURL := result.Status.URL
	if serviceURL == "" {
		serviceURL = result.URI
	}

	response := &CloudRunServiceResponse{
		ConsoleURL: d.consoleURL(),
	}
	response.Service.Name = result.Metadata.Name
	if response.Service.Name == "" {
		response.Service.Name = d.ServiceName
	}
	response.Service.URL = serviceURL
	response.Service.Region = d.Region
	response.Service.ProjectID = d.ProjectID

	return response, nil
}

func (d *GCPDeployer) consoleURL() string {
	return fmt.Sprintf(
		"https://console.cloud.google.com/run/detail/%s/%s/metrics?project=%s",
		d.Region,
		d.ServiceName,
		d.ProjectID,
	)
}

func (d *GCPDeployer) cloudRunPort(isStatic bool) string {
	if isStatic {
		return "80"
	}
	return "3000"
}

func (d *GCPDeployer) writeEnvVarsFile(envVars map[string]string) (string, error) {
	data, err := yaml.Marshal(envVars)
	if err != nil {
		return "", fmt.Errorf("marshaling Cloud Run env vars: %w", err)
	}

	file, err := os.CreateTemp("", "mvpbridge-cloudrun-env-*.yaml")
	if err != nil {
		return "", fmt.Errorf("creating Cloud Run env var file: %w", err)
	}
	defer func() { _ = file.Close() }()

	if _, err := file.Write(data); err != nil {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("writing Cloud Run env var file: %w", err)
	}

	return file.Name(), nil
}

func (d *GCPDeployer) run(ctx context.Context, workdir string, args ...string) ([]byte, error) {
	env := []string{
		"CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE=" + d.CredentialsFile,
		"CLOUDSDK_CORE_PROJECT=" + d.ProjectID,
		"CLOUDSDK_RUN_REGION=" + d.Region,
	}
	return d.runCommand(ctx, workdir, env, args...)
}

func defaultGcloudRunner(ctx context.Context, workdir string, env []string, args ...string) ([]byte, error) {
	// #nosec G204 -- command is fixed ("gcloud"), args are structured flags, and no shell is used.
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), env...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		details := strings.TrimSpace(string(output))
		if details == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%s", details)
	}

	return output, nil
}

func sanitizeCloudRunServiceName(appName string) string {
	return sanitizeDashedName(appName, 63)
}

func isCloudRunNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "cannot find") || strings.Contains(msg, "404")
}
