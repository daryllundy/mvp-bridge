// Package deploy provides cloud platform deployment implementations for
// MVPBridge. It supports DigitalOcean App Platform and AWS Amplify.
package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const doAPIBase = "https://api.digitalocean.com/v2"

var doAPIBaseURL = mustParseURL(doAPIBase)

// DODeployer handles deployments to DigitalOcean App Platform
type DODeployer struct {
	Token   string
	AppName string
	RepoURL string
	Branch  string
	client  *http.Client
}

// DOAppSpec represents the DigitalOcean App Platform app specification
type DOAppSpec struct {
	Name        string         `json:"name"`
	Region      string         `json:"region,omitempty"`
	Services    []DOService    `json:"services,omitempty"`
	StaticSites []DOStaticSite `json:"static_sites,omitempty"`
}

// DOService represents a DigitalOcean service component (for SSR apps)
type DOService struct {
	Name             string     `json:"name"`
	GitHub           *DOGitHub  `json:"github,omitempty"`
	Dockerfile       string     `json:"dockerfile_path,omitempty"`
	SourceDir        string     `json:"source_dir,omitempty"`
	HTTPPort         int        `json:"http_port,omitempty"`
	InstanceCount    int        `json:"instance_count,omitempty"`
	InstanceSizeSlug string     `json:"instance_size_slug,omitempty"`
	Envs             []DOEnvVar `json:"envs,omitempty"`
}

// DOStaticSite represents a DigitalOcean static site component
type DOStaticSite struct {
	Name         string     `json:"name"`
	GitHub       *DOGitHub  `json:"github,omitempty"`
	BuildCommand string     `json:"build_command,omitempty"`
	OutputDir    string     `json:"output_dir,omitempty"`
	Envs         []DOEnvVar `json:"envs,omitempty"`
}

// DOGitHub represents GitHub repository configuration for DigitalOcean
type DOGitHub struct {
	Repo         string `json:"repo"`
	Branch       string `json:"branch"`
	DeployOnPush bool   `json:"deploy_on_push"`
}

// DOEnvVar represents an environment variable in DigitalOcean App Platform
type DOEnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
	Type  string `json:"type,omitempty"` // GENERAL or SECRET
}

// DOAppResponse represents the API response when creating or updating an app
type DOAppResponse struct {
	App struct {
		ID               string `json:"id"`
		DefaultIngress   string `json:"default_ingress"`
		LiveURL          string `json:"live_url"`
		ActiveDeployment struct {
			ID    string `json:"id"`
			Phase string `json:"phase"`
		} `json:"active_deployment"`
	} `json:"app"`
}

// NewDODeployer creates a new DigitalOcean deployer instance
func NewDODeployer(appName, repoURL, branch string) (*DODeployer, error) {
	token := os.Getenv("DIGITALOCEAN_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("DIGITALOCEAN_TOKEN environment variable not set")
	}

	return &DODeployer{
		Token:   token,
		AppName: appName,
		RepoURL: repoURL,
		Branch:  branch,
		client:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Deploy creates or updates a DO App Platform app
func (d *DODeployer) Deploy(isStatic bool, envVars map[string]string) (*DOAppResponse, error) {
	// Check if app already exists
	existing, err := d.getApp()
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return nil, fmt.Errorf("checking existing app: %w", err)
	}

	// Build app spec
	spec := d.buildSpec(isStatic, envVars)

	if existing != nil {
		// Update existing app
		return d.updateApp(existing.App.ID, spec)
	}

	// Create new app
	return d.createApp(spec)
}

func (d *DODeployer) buildSpec(isStatic bool, envVars map[string]string) *DOAppSpec {
	// Parse GitHub repo from URL
	// Expected format: github.com/owner/repo or https://github.com/owner/repo
	repoPath := strings.TrimPrefix(d.RepoURL, "https://")
	repoPath = strings.TrimPrefix(repoPath, "github.com/")
	repoPath = strings.TrimSuffix(repoPath, ".git")

	github := &DOGitHub{
		Repo:         repoPath,
		Branch:       d.Branch,
		DeployOnPush: true,
	}

	// Convert env vars
	var envs []DOEnvVar
	for k, v := range envVars {
		envType := "GENERAL"
		if strings.Contains(strings.ToLower(k), "secret") ||
			strings.Contains(strings.ToLower(k), "key") ||
			strings.Contains(strings.ToLower(k), "password") ||
			strings.Contains(strings.ToLower(k), "token") {
			envType = "SECRET"
		}
		envs = append(envs, DOEnvVar{Key: k, Value: v, Type: envType})
	}

	spec := &DOAppSpec{
		Name:   d.AppName,
		Region: "nyc", // Default to NYC, could be configurable
	}

	if isStatic {
		spec.StaticSites = []DOStaticSite{{
			Name:         d.AppName,
			GitHub:       github,
			BuildCommand: "npm run build",
			OutputDir:    "dist",
			Envs:         envs,
		}}
	} else {
		spec.Services = []DOService{{
			Name:             d.AppName,
			GitHub:           github,
			Dockerfile:       "Dockerfile",
			SourceDir:        "/",
			HTTPPort:         3000,
			InstanceCount:    1,
			InstanceSizeSlug: "basic-xxs",
			Envs:             envs,
		}}
	}

	return spec
}

func (d *DODeployer) createApp(spec *DOAppSpec) (*DOAppResponse, error) {
	req, err := newJSONRequest(http.MethodPost, doAppsURL(), map[string]interface{}{"spec": spec})
	if err != nil {
		return nil, err
	}

	return d.doRequest(req)
}

func (d *DODeployer) updateApp(appID string, spec *DOAppSpec) (*DOAppResponse, error) {
	reqURL, err := doAppURL(appID)
	if err != nil {
		return nil, err
	}

	req, err := newJSONRequest(http.MethodPut, reqURL, map[string]interface{}{"spec": spec})
	if err != nil {
		return nil, err
	}

	return d.doRequest(req)
}

func (d *DODeployer) getApp() (*DOAppResponse, error) {
	req, err := newJSONRequest(http.MethodGet, doAppsURL(), nil)
	if err != nil {
		return nil, err
	}

	body, err := executeRequest(d.client, req, doAPIBaseURL.Host, d.withAuthHeaders)
	if err != nil {
		return nil, err
	}

	var result struct {
		Apps []struct {
			ID   string `json:"id"`
			Spec struct {
				Name string `json:"name"`
			} `json:"spec"`
		} `json:"apps"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	for _, app := range result.Apps {
		if app.Spec.Name == d.AppName {
			// Fetch full app details
			return d.getAppByID(app.ID)
		}
	}

	return nil, fmt.Errorf("app not found: %s", d.AppName)
}

func (d *DODeployer) getAppByID(id string) (*DOAppResponse, error) {
	reqURL, err := doAppURL(id)
	if err != nil {
		return nil, err
	}

	req, err := newJSONRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	return d.doRequest(req)
}

func (d *DODeployer) doRequest(req *http.Request) (*DOAppResponse, error) {
	return executeJSONRequest[DOAppResponse](d.client, req, doAPIBaseURL.Host, d.withAuthHeaders)
}

func (d *DODeployer) withAuthHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+d.Token)
	req.Header.Set("Content-Type", "application/json")
}

func doAppsURL() *url.URL {
	return mustJoinURLPath(doAPIBaseURL, "apps")
}

func doAppURL(appID string) (*url.URL, error) {
	if appID == "" {
		return nil, fmt.Errorf("app ID cannot be empty")
	}

	return mustJoinURLPath(doAPIBaseURL, "apps", appID), nil
}

// WaitForDeployment polls until deployment completes or fails
func (d *DODeployer) WaitForDeployment(appID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		app, err := d.getAppByID(appID)
		if err != nil {
			return err
		}

		phase := app.App.ActiveDeployment.Phase
		switch phase {
		case "ACTIVE":
			return nil
		case "ERROR", "CANCELED":
			return fmt.Errorf("deployment failed: %s", phase)
		}

		fmt.Printf("  Deployment status: %s\n", phase)
		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("deployment timed out after %v", timeout)
}

// GetLogs fetches deployment logs (simplified)
func (d *DODeployer) GetLogs(appID, deploymentID string) (string, error) {
	url := fmt.Sprintf("%s/apps/%s/deployments/%s/logs", doAPIBase, appID, deploymentID)
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+d.Token)

	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
