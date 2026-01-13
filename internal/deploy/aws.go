package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const awsAmplifyAPIBase = "https://amplify.%s.amazonaws.com"

// AWSDeployer handles deployments to AWS Amplify
type AWSDeployer struct {
	AccessKey string
	SecretKey string
	Region    string
	AppName   string
	RepoURL   string
	Branch    string
	client    *http.Client
}

// AmplifyApp represents an AWS Amplify application configuration
type AmplifyApp struct {
	Name                 string            `json:"name"`
	Repository           string            `json:"repository"`
	Platform             string            `json:"platform"` // WEB
	IAMServiceRole       string            `json:"iamServiceRole,omitempty"`
	EnvironmentVariables map[string]string `json:"environmentVariables,omitempty"`
	BuildSpec            string            `json:"buildSpec,omitempty"`
	CustomRules          []AmplifyRule     `json:"customRules,omitempty"`
}

// AmplifyRule represents a custom routing rule in AWS Amplify
type AmplifyRule struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Status    string `json:"status"`
	Condition string `json:"condition,omitempty"`
}

// AmplifyBranch represents a branch configuration in AWS Amplify
type AmplifyBranch struct {
	BranchName           string            `json:"branchName"`
	EnableAutoBuild      bool              `json:"enableAutoBuild"`
	Stage                string            `json:"stage"` // PRODUCTION, DEVELOPMENT
	EnvironmentVariables map[string]string `json:"environmentVariables,omitempty"`
}

// AmplifyAppResponse represents the API response when creating or getting an Amplify app
type AmplifyAppResponse struct {
	App struct {
		AppID         string `json:"appId"`
		Name          string `json:"name"`
		DefaultDomain string `json:"defaultDomain"`
		Repository    string `json:"repository"`
	} `json:"app"`
}

// AmplifyBranchResponse represents the API response when creating or getting a branch
type AmplifyBranchResponse struct {
	Branch struct {
		BranchName string `json:"branchName"`
	} `json:"branch"`
}

// NewAWSDeployer creates a new AWS Amplify deployer instance
func NewAWSDeployer(appName, repoURL, branch, region string) (*AWSDeployer, error) {
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables must be set")
	}

	if region == "" {
		region = "us-east-1" // Default region
	}

	return &AWSDeployer{
		AccessKey: accessKey,
		SecretKey: secretKey,
		Region:    region,
		AppName:   appName,
		RepoURL:   repoURL,
		Branch:    branch,
		client:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Deploy creates or updates an AWS Amplify app
func (d *AWSDeployer) Deploy(isStatic bool, envVars map[string]string, buildCommand, outputDir string) (*AmplifyAppResponse, error) {
	// Check if app exists
	existing, err := d.getApp()
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return nil, fmt.Errorf("checking existing app: %w", err)
	}

	if existing != nil {
		// Update existing app
		return d.updateApp(existing.App.AppID, envVars, buildCommand, outputDir)
	}

	// Create new app
	return d.createApp(envVars, buildCommand, outputDir, isStatic)
}

func (d *AWSDeployer) createApp(envVars map[string]string, buildCommand, outputDir string, isStatic bool) (*AmplifyAppResponse, error) {
	// Parse GitHub token from environment or URL
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable required for AWS Amplify")
	}

	// Build the app spec
	app := AmplifyApp{
		Name:                 d.AppName,
		Repository:           d.RepoURL,
		Platform:             "WEB",
		EnvironmentVariables: envVars,
		BuildSpec:            d.buildSpec(buildCommand, outputDir),
	}

	// Add SPA redirect rules for static apps
	if isStatic {
		app.CustomRules = []AmplifyRule{
			{
				Source:    "/<*>",
				Target:    "/index.html",
				Status:    "404-200",
				Condition: "",
			},
		}
	}

	body := map[string]interface{}{
		"name":                 app.Name,
		"repository":           app.Repository,
		"platform":             app.Platform,
		"environmentVariables": app.EnvironmentVariables,
		"buildSpec":            app.BuildSpec,
		"customRules":          app.CustomRules,
		"accessToken":          githubToken,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf(awsAmplifyAPIBase, d.Region) + "/apps"
	req, err := http.NewRequestWithContext(context.Background(), "POST", endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	result, err := d.doRequest(req)
	if err != nil {
		return nil, err
	}

	// Create branch
	if err := d.createBranch(result.App.AppID, envVars); err != nil {
		return nil, fmt.Errorf("creating branch: %w", err)
	}

	return result, nil
}

func (d *AWSDeployer) updateApp(appID string, envVars map[string]string, buildCommand, outputDir string) (*AmplifyAppResponse, error) {
	body := map[string]interface{}{
		"environmentVariables": envVars,
		"buildSpec":            d.buildSpec(buildCommand, outputDir),
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf(awsAmplifyAPIBase, d.Region) + "/apps/" + appID
	req, err := http.NewRequestWithContext(context.Background(), "POST", endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	return d.doRequest(req)
}

func (d *AWSDeployer) createBranch(appID string, envVars map[string]string) error {
	branch := AmplifyBranch{
		BranchName:           d.Branch,
		EnableAutoBuild:      true,
		Stage:                "PRODUCTION",
		EnvironmentVariables: envVars,
	}

	jsonBody, err := json.Marshal(branch)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf(awsAmplifyAPIBase, d.Region) + "/apps/" + appID + "/branches"
	req, err := http.NewRequestWithContext(context.Background(), "POST", endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	d.signRequest(req)

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("API error %d (failed to read body: %w)", resp.StatusCode, err)
		}
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (d *AWSDeployer) getApp() (*AmplifyAppResponse, error) {
	endpoint := fmt.Sprintf(awsAmplifyAPIBase, d.Region) + "/apps"
	req, err := http.NewRequestWithContext(context.Background(), "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	d.signRequest(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", resp.Status)
	}

	var result struct {
		Apps []struct {
			AppID string `json:"appId"`
			Name  string `json:"name"`
		} `json:"apps"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Find app by name
	for _, app := range result.Apps {
		if app.Name == d.AppName {
			return d.getAppByID(app.AppID)
		}
	}

	return nil, fmt.Errorf("app not found: %s", d.AppName)
}

func (d *AWSDeployer) getAppByID(appID string) (*AmplifyAppResponse, error) {
	endpoint := fmt.Sprintf(awsAmplifyAPIBase, d.Region) + "/apps/" + appID
	req, err := http.NewRequestWithContext(context.Background(), "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	return d.doRequest(req)
}

func (d *AWSDeployer) doRequest(req *http.Request) (*AmplifyAppResponse, error) {
	d.signRequest(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var result AmplifyAppResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &result, nil
}

// signRequest adds AWS Signature Version 4 authentication
// Note: This is a simplified version. For production, use AWS SDK
func (d *AWSDeployer) signRequest(req *http.Request) {
	// For simplicity, we'll use basic auth headers
	// In production, you should use proper AWS SigV4 signing or the AWS SDK
	req.Header.Set("X-Amz-Date", time.Now().UTC().Format("20060102T150405Z"))
	// Add proper SigV4 signing here or use AWS SDK
}

func (d *AWSDeployer) buildSpec(buildCommand, outputDir string) string {
	if buildCommand == "" {
		buildCommand = "npm run build"
	}
	if outputDir == "" {
		outputDir = "dist"
	}

	return fmt.Sprintf(`version: 1
frontend:
  phases:
    preBuild:
      commands:
        - npm ci
    build:
      commands:
        - %s
  artifacts:
    baseDirectory: %s
    files:
      - '**/*'
  cache:
    paths:
      - node_modules/**/*
`, buildCommand, outputDir)
}
