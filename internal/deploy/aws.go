package deploy

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
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

	reqURL, err := d.amplifyAppsURL()
	if err != nil {
		return nil, err
	}

	req, err := newJSONRequest(http.MethodPost, reqURL, json.RawMessage(jsonBody))
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

	reqURL, err := d.amplifyAppURL(appID)
	if err != nil {
		return nil, err
	}

	req, err := newJSONRequest(http.MethodPost, reqURL, json.RawMessage(jsonBody))
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

	reqURL, err := d.amplifyAppBranchesURL(appID)
	if err != nil {
		return err
	}

	req, err := newJSONRequest(http.MethodPost, reqURL, json.RawMessage(jsonBody))
	if err != nil {
		return err
	}

	_, err = executeRequest(d.client, req, d.amplifyHost(), d.withSignedJSONHeaders)
	return err
}

func (d *AWSDeployer) getApp() (*AmplifyAppResponse, error) {
	reqURL, err := d.amplifyAppsURL()
	if err != nil {
		return nil, err
	}

	req, err := newJSONRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	body, err := executeRequest(d.client, req, d.amplifyHost(), d.withSignedJSONHeaders)
	if err != nil {
		return nil, err
	}

	var result struct {
		Apps []struct {
			AppID string `json:"appId"`
			Name  string `json:"name"`
		} `json:"apps"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
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
	reqURL, err := d.amplifyAppURL(appID)
	if err != nil {
		return nil, err
	}

	req, err := newJSONRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	return d.doRequest(req)
}

func (d *AWSDeployer) doRequest(req *http.Request) (*AmplifyAppResponse, error) {
	return executeJSONRequest[AmplifyAppResponse](d.client, req, d.amplifyHost(), d.withSignedJSONHeaders)
}

func (d *AWSDeployer) withSignedJSONHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	d.signRequest(req)
}

func (d *AWSDeployer) amplifyBaseURL() (*url.URL, error) {
	if d.Region == "" {
		return nil, fmt.Errorf("region cannot be empty")
	}

	return url.Parse(fmt.Sprintf(awsAmplifyAPIBase, d.Region))
}

func (d *AWSDeployer) amplifyAppsURL() (*url.URL, error) {
	baseURL, err := d.amplifyBaseURL()
	if err != nil {
		return nil, err
	}

	return mustJoinURLPath(baseURL, "apps"), nil
}

func (d *AWSDeployer) amplifyAppURL(appID string) (*url.URL, error) {
	if appID == "" {
		return nil, fmt.Errorf("app ID cannot be empty")
	}

	baseURL, err := d.amplifyBaseURL()
	if err != nil {
		return nil, err
	}

	return mustJoinURLPath(baseURL, "apps", appID), nil
}

func (d *AWSDeployer) amplifyAppBranchesURL(appID string) (*url.URL, error) {
	baseURL, err := d.amplifyBaseURL()
	if err != nil {
		return nil, err
	}

	if appID == "" {
		return nil, fmt.Errorf("app ID cannot be empty")
	}

	return mustJoinURLPath(baseURL, "apps", appID, "branches"), nil
}

func (d *AWSDeployer) amplifyHost() string {
	return fmt.Sprintf("amplify.%s.amazonaws.com", d.Region)
}

// signRequest adds AWS Signature Version 4 authentication
func (d *AWSDeployer) signRequest(req *http.Request) {
	d.signRequestWithTime(req, time.Now().UTC())
}

// signRequestWithTime signs the request with a specific timestamp (for testing)
func (d *AWSDeployer) signRequestWithTime(req *http.Request, t time.Time) {
	const service = "amplify"
	const algorithm = "AWS4-HMAC-SHA256"

	// Format timestamps
	amzDate := t.Format("20060102T150405Z")
	dateStamp := t.Format("20060102")

	// Set required headers before signing
	req.Header.Set("X-Amz-Date", amzDate)
	if req.Header.Get("Host") == "" {
		req.Header.Set("Host", req.URL.Host)
	}

	// Get payload hash
	payloadHash := d.getPayloadHash(req)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	// Create canonical request
	canonicalRequest, signedHeaders := d.createCanonicalRequest(req, payloadHash)

	// Create credential scope
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, d.Region, service)

	// Create string to sign
	stringToSign := d.createStringToSign(algorithm, amzDate, credentialScope, canonicalRequest)

	// Calculate signing key
	signingKey := d.deriveSigningKey(d.SecretKey, dateStamp, d.Region, service)

	// Calculate signature
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	// Create authorization header
	authHeader := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, d.AccessKey, credentialScope, signedHeaders, signature)

	req.Header.Set("Authorization", authHeader)
}

// getPayloadHash returns the SHA256 hash of the request body
func (d *AWSDeployer) getPayloadHash(req *http.Request) string {
	if req.Body == nil {
		return sha256Hash([]byte(""))
	}

	// Read the body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return sha256Hash([]byte(""))
	}

	// Restore the body for later use
	req.Body = io.NopCloser(bytes.NewBuffer(body))

	return sha256Hash(body)
}

// createCanonicalRequest builds the canonical request string per AWS SigV4 spec
func (d *AWSDeployer) createCanonicalRequest(req *http.Request, payloadHash string) (string, string) {
	// Canonical URI (URL-encoded path)
	canonicalURI := req.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	// Canonical query string (sorted by parameter name)
	canonicalQueryString := d.getCanonicalQueryString(req.URL.Query())

	// Canonical headers (lowercase, sorted, trimmed)
	canonicalHeaders, signedHeaders := d.getCanonicalHeaders(req)

	// Build canonical request
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	return canonicalRequest, signedHeaders
}

// getCanonicalQueryString returns the canonical query string (sorted parameters)
func (d *AWSDeployer) getCanonicalQueryString(query url.Values) string {
	if len(query) == 0 {
		return ""
	}

	// Get sorted keys
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build canonical query string
	var pairs []string
	for _, k := range keys {
		values := query[k]
		sort.Strings(values)
		for _, v := range values {
			// AWS SigV4 requires RFC 3986 URI encoding (%20 for spaces, not +)
			pairs = append(pairs, uriEncode(k, false)+"="+uriEncode(v, false))
		}
	}

	return strings.Join(pairs, "&")
}

// uriEncode performs RFC 3986 URI encoding as required by AWS SigV4
func uriEncode(s string, encodeSlash bool) string {
	var result strings.Builder
	for _, b := range []byte(s) {
		if isUnreserved(b) {
			result.WriteByte(b)
		} else if b == '/' && !encodeSlash {
			result.WriteByte(b)
		} else {
			result.WriteString(fmt.Sprintf("%%%02X", b))
		}
	}
	return result.String()
}

// isUnreserved returns true if the byte is an unreserved character per RFC 3986
func isUnreserved(b byte) bool {
	return (b >= 'A' && b <= 'Z') ||
		(b >= 'a' && b <= 'z') ||
		(b >= '0' && b <= '9') ||
		b == '-' || b == '_' || b == '.' || b == '~'
}

// getCanonicalHeaders returns canonical headers and signed header list
func (d *AWSDeployer) getCanonicalHeaders(req *http.Request) (string, string) {
	// Headers to sign (must include host and x-amz-date at minimum)
	headers := make(map[string]string)

	for key, values := range req.Header {
		lowerKey := strings.ToLower(key)
		// Include host, content-type, and all x-amz-* headers
		if lowerKey == "host" || lowerKey == "content-type" || strings.HasPrefix(lowerKey, "x-amz-") {
			// Trim whitespace and combine multiple values
			var trimmedValues []string
			for _, v := range values {
				trimmedValues = append(trimmedValues, strings.TrimSpace(v))
			}
			headers[lowerKey] = strings.Join(trimmedValues, ",")
		}
	}

	// Sort header names
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build canonical headers string
	var canonicalHeaders strings.Builder
	for _, k := range keys {
		canonicalHeaders.WriteString(k)
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(headers[k])
		canonicalHeaders.WriteString("\n")
	}

	signedHeaders := strings.Join(keys, ";")

	return canonicalHeaders.String(), signedHeaders
}

// createStringToSign creates the string to sign per AWS SigV4 spec
func (d *AWSDeployer) createStringToSign(algorithm, amzDate, credentialScope, canonicalRequest string) string {
	return strings.Join([]string{
		algorithm,
		amzDate,
		credentialScope,
		sha256Hash([]byte(canonicalRequest)),
	}, "\n")
}

// deriveSigningKey derives the signing key per AWS SigV4 spec
func (d *AWSDeployer) deriveSigningKey(secretKey, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}

// hmacSHA256 computes HMAC-SHA256
func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// sha256Hash returns the hex-encoded SHA256 hash of data
func sha256Hash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (d *AWSDeployer) buildSpec(buildCommand, outputDir string) string {
	if buildCommand == "" {
		buildCommand = DefaultBuildCommand
	}
	if outputDir == "" {
		outputDir = DefaultOutputDir
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
