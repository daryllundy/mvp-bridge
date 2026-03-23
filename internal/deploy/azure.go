package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

const defaultAzureRegion = "eastus"

var azLookPath = exec.LookPath
var azDefaultRunner azureRunner = defaultAzRunner

type azureRunner func(ctx context.Context, workdir string, env []string, args ...string) ([]byte, error)

// AzureDeployer handles deployments to Azure Container Apps through the az CLI.
type AzureDeployer struct {
	AppName        string
	ResourceGroup  string
	Environment    string
	Region         string
	SubscriptionID string
	runCommand     azureRunner
}

// AzureContainerAppResponse represents the metadata returned after deployment.
type AzureContainerAppResponse struct {
	App struct {
		Name           string `json:"name"`
		URL            string `json:"url"`
		FQDN           string `json:"fqdn"`
		Region         string `json:"region"`
		SubscriptionID string `json:"subscriptionId"`
		ResourceGroup  string `json:"resourceGroup"`
		Environment    string `json:"environment"`
	} `json:"app"`
	ConsoleURL string `json:"consoleUrl"`
}

// NewAzureDeployer creates a new Azure Container Apps deployer instance.
func NewAzureDeployer(appName, resourceGroup, environment, region, subscriptionID string) (*AzureDeployer, error) {
	if _, err := azLookPath("az"); err != nil {
		return nil, fmt.Errorf("az CLI not found in PATH")
	}

	if region == "" {
		region = defaultAzureRegion
	}

	sanitizedName := sanitizeAzureName(appName)
	if sanitizedName == "" {
		return nil, fmt.Errorf("unable to derive Container App name from app name %q", appName)
	}

	if resourceGroup == "" {
		resourceGroup = fmt.Sprintf("mvpbridge-%s-rg", sanitizedName)
	}
	if environment == "" {
		environment = fmt.Sprintf("mvpbridge-%s-env", sanitizedName)
	}

	deployer := &AzureDeployer{
		AppName:        sanitizedName,
		ResourceGroup:  resourceGroup,
		Environment:    environment,
		Region:         region,
		SubscriptionID: strings.TrimSpace(subscriptionID),
		runCommand:     azDefaultRunner,
	}

	if _, err := deployer.run(context.Background(), ".", "account", "show", "--output", "json"); err != nil {
		return nil, fmt.Errorf("azure CLI is not authenticated - run 'az login': %w", err)
	}

	if err := deployer.ensureContainerAppExtension(context.Background()); err != nil {
		return nil, err
	}

	return deployer, nil
}

// Deploy creates or updates an Azure Container App from the current directory.
func (d *AzureDeployer) Deploy(isStatic bool, envVars map[string]string) (*AzureContainerAppResponse, error) {
	if d.runCommand == nil {
		d.runCommand = azDefaultRunner
	}

	if _, err := os.Stat("Dockerfile"); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("Dockerfile not found - run 'mvpbridge normalize' first")
		}
		return nil, fmt.Errorf("checking Dockerfile: %w", err)
	}

	upArgs := []string{
		"containerapp", "up",
		"--name", d.AppName,
		"--resource-group", d.ResourceGroup,
		"--environment", d.Environment,
		"--location", d.Region,
		"--ingress", "external",
		"--target-port", d.containerAppPort(isStatic),
		"--source", ".",
		"--output", "json",
	}
	if _, err := d.run(context.Background(), ".", upArgs...); err != nil {
		return nil, fmt.Errorf("deploying Container App: %w", err)
	}

	if len(envVars) > 0 {
		updateArgs := []string{
			"containerapp", "update",
			"--name", d.AppName,
			"--resource-group", d.ResourceGroup,
			"--set-env-vars",
		}
		updateArgs = append(updateArgs, formatAzureEnvVars(envVars)...)
		if _, err := d.run(context.Background(), ".", updateArgs...); err != nil {
			return nil, fmt.Errorf("updating Container App env vars: %w", err)
		}
	}

	return d.showContainerApp(context.Background())
}

func (d *AzureDeployer) ensureContainerAppExtension(ctx context.Context) error {
	_, err := d.run(ctx, ".", "extension", "show", "--name", "containerapp", "--output", "json")
	if err == nil {
		return nil
	}

	if _, installErr := d.run(ctx, ".", "extension", "add", "--name", "containerapp", "--upgrade", "--output", "json"); installErr != nil {
		return fmt.Errorf("ensuring Azure containerapp CLI extension: %w", installErr)
	}

	return nil
}

func (d *AzureDeployer) showContainerApp(ctx context.Context) (*AzureContainerAppResponse, error) {
	output, err := d.run(ctx, ".", "containerapp", "show",
		"--name", d.AppName,
		"--resource-group", d.ResourceGroup,
		"--output", "json",
	)
	if err != nil {
		return nil, fmt.Errorf("describing Container App: %w", err)
	}

	var result struct {
		Name       string `json:"name"`
		Location   string `json:"location"`
		Properties struct {
			Configuration struct {
				Ingress struct {
					FQDN string `json:"fqdn"`
				} `json:"ingress"`
			} `json:"configuration"`
			ManagedEnvironmentID string `json:"managedEnvironmentId"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parsing Container App response: %w", err)
	}

	response := &AzureContainerAppResponse{
		ConsoleURL: d.consoleURL(),
	}
	response.App.Name = result.Name
	if response.App.Name == "" {
		response.App.Name = d.AppName
	}
	response.App.FQDN = result.Properties.Configuration.Ingress.FQDN
	if response.App.FQDN != "" {
		response.App.URL = "https://" + response.App.FQDN
	}
	response.App.Region = result.Location
	if response.App.Region == "" {
		response.App.Region = d.Region
	}
	response.App.SubscriptionID = d.SubscriptionID
	response.App.ResourceGroup = d.ResourceGroup
	response.App.Environment = d.environmentName(result.Properties.ManagedEnvironmentID)

	return response, nil
}

func (d *AzureDeployer) environmentName(managedEnvironmentID string) string {
	parts := strings.Split(strings.TrimSpace(managedEnvironmentID), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return d.Environment
}

func (d *AzureDeployer) consoleURL() string {
	if d.SubscriptionID == "" {
		return fmt.Sprintf(
			"https://portal.azure.com/#view/HubsExtension/Resources/resourceType/Microsoft.App%%2FcontainerApps/resourceGroup/%s/name/%s",
			d.ResourceGroup,
			d.AppName,
		)
	}

	return fmt.Sprintf(
		"https://portal.azure.com/#@/resource/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/containerApps/%s",
		d.SubscriptionID,
		d.ResourceGroup,
		d.AppName,
	)
}

func (d *AzureDeployer) containerAppPort(isStatic bool) string {
	if isStatic {
		return "80"
	}
	return "3000"
}

func (d *AzureDeployer) run(ctx context.Context, workdir string, args ...string) ([]byte, error) {
	if d.SubscriptionID != "" {
		args = append(args, "--subscription", d.SubscriptionID)
	}

	return d.runCommand(ctx, workdir, nil, args...)
}

func defaultAzRunner(ctx context.Context, workdir string, env []string, args ...string) ([]byte, error) {
	// #nosec G204 -- command is fixed ("az"), args are structured flags, and no shell is used.
	cmd := exec.CommandContext(ctx, "az", args...)
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

func sanitizeAzureName(appName string) string {
	return sanitizeDashedName(appName, 32)
}

func formatAzureEnvVars(envVars map[string]string) []string {
	formatted := make([]string, 0, len(envVars))
	keys := make([]string, 0, len(envVars))
	for key := range envVars {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := envVars[key]
		formatted = append(formatted, fmt.Sprintf("%s=%s", key, value))
	}
	return formatted
}
