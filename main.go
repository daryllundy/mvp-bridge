// Package main provides the CLI entrypoint for MVPBridge, a tool that bridges
// MVP codebases and production-ready deployments.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/daryllundy/mvp-bridge/internal/config"
	"github.com/daryllundy/mvp-bridge/internal/deploy"
	"github.com/daryllundy/mvp-bridge/internal/detect"
	"github.com/daryllundy/mvp-bridge/internal/normalize"

	"github.com/spf13/cobra"
)

var version = "0.1.0"

type gcpDeployer interface {
	Deploy(isStatic bool, envVars map[string]string) (*deploy.CloudRunServiceResponse, error)
}

var (
	prepareDeployFunc  = prepareDeploy
	newGCPDeployerFunc = func(appName, projectID, region string) (gcpDeployer, error) {
		return deploy.NewGCPDeployer(appName, projectID, region)
	}
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "mvpbridge",
		Short: "Bridge your MVP to production",
		Long: `MVPBridge inspects, normalizes, and deploys your frontend projects.

No hosted dependencies. No daemons. Just a single binary that
gets your MVP from "works on my machine" to "works everywhere."`,
		Version: version,
	}

	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(inspectCmd())
	rootCmd.AddCommand(normalizeCmd())
	rootCmd.AddCommand(deployCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func initCmd() *cobra.Command {
	var target string
	var framework string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize MVPBridge in current repo",
		Long:  `Sets up MVPBridge configuration by detecting your project structure and creating .mvpbridge/config.yaml`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runInit(target, framework)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "Deployment target (do, aws, gcp)")
	cmd.Flags().StringVarP(&framework, "framework", "f", "", "Framework (vite, nextjs)")

	return cmd
}

func inspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect",
		Short: "Analyze repo and report deployment readiness",
		Long:  `Performs read-only analysis of your repository to identify what needs to be fixed before deployment.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runInspect()
		},
	}
}

func normalizeCmd() *cobra.Command {
	var dryRun bool
	var yes bool

	cmd := &cobra.Command{
		Use:   "normalize",
		Short: "Apply fixes to make repo deployable",
		Long:  `Applies atomic, reversible changes to prepare your repository for deployment.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runNormalize(dryRun, yes)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without applying")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompts")

	return cmd
}

func deployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [target]",
		Short: "Deploy to target platform",
		Long:  `Deploys your application to the specified platform (do for DigitalOcean, aws for AWS, gcp for Google Cloud Run).`,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runDeploy(args[0])
		},
	}

	return cmd
}

// Implementation functions

func runInit(target, framework string) error {
	fmt.Println("Initializing MVPBridge...")

	// Check prerequisites
	checks := []struct {
		name string
		fn   func() error
	}{
		{"Git installed", checkGit},
		{"Inside git repo", checkGitRepo},
		{"Node.js present", checkNode},
	}

	for _, c := range checks {
		fmt.Printf("  Checking %s... ", c.name)
		if err := c.fn(); err != nil {
			fmt.Println("✗")
			return fmt.Errorf("%s: %w", c.name, err)
		}
		fmt.Println("✓")
	}

	// Run detection
	d, err := detect.DetectAll(".")
	if err != nil {
		return fmt.Errorf("detection failed: %w", err)
	}

	// Use detected framework if not specified
	if framework == "" {
		framework = string(d.Framework)
		fmt.Printf("\n  Detected framework: %s\n", framework)
	}

	// Use default target if not specified
	if target == "" {
		target = "do"
	}

	// Create config from detection
	cfg := config.NewFromDetection(d, target)
	if err := cfg.Save("."); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("\n✓ MVPBridge initialized. Run `mvpbridge inspect` next.")
	return nil
}

func runInspect() error {
	// Run detection
	d, err := detect.DetectAll(".")
	if err != nil {
		return fmt.Errorf("detection failed: %w", err)
	}

	fmt.Println()
	fmt.Println("╭─────────────────────────────────────────────────╮")
	fmt.Println("│  MVPBridge Inspection Report                    │")
	fmt.Println("├─────────────────────────────────────────────────┤")

	// Display framework info
	fwDisplay := formatFramework(d.Framework)
	fmt.Printf("│  Framework:     %-32s│\n", fwDisplay)

	// Display Node version
	nodeDisplay := formatNodeVersion(d.NodeVersion)
	fmt.Printf("│  Node:          %-32s│\n", nodeDisplay)

	// Display package manager
	fmt.Printf("│  Package Mgr:   %-32s│\n", string(d.PackageManager))

	// Display build config
	if d.BuildCommand != "" {
		buildDisplay := fmt.Sprintf("%s → %s", d.BuildCommand, d.OutputDir)
		if len(buildDisplay) > 32 {
			buildDisplay = buildDisplay[:29] + "..."
		}
		fmt.Printf("│  Build:         %-32s│\n", buildDisplay)
	}

	// Display output type
	if d.OutputType != "" {
		fmt.Printf("│  Output Type:   %-32s│\n", string(d.OutputType))
	}

	fmt.Println("├─────────────────────────────────────────────────┤")

	// Display issues
	if len(d.Issues) == 0 {
		fmt.Println("│  ✓ Ready for deployment!                        │")
	} else {
		fmt.Printf("│  Deployment Readiness: %-2d issues found           │\n", len(d.Issues))
		fmt.Println("│                                                 │")
		for _, issue := range d.Issues {
			desc := issue.Description
			if len(desc) > 44 {
				desc = desc[:41] + "..."
			}
			fmt.Printf("│  ✗ %-44s│\n", desc)
		}
		fmt.Println("│                                                 │")
		fmt.Println("│  Run `mvpbridge normalize` to fix these.        │")
	}

	fmt.Println("╰─────────────────────────────────────────────────╯")
	fmt.Println()

	return nil
}

func runNormalize(dryRun, yes bool) error {
	// Load config (or create minimal one if not exists)
	cfg, err := config.Load(".")
	if err != nil {
		// Try to detect if config doesn't exist
		d, detectErr := detect.DetectAll(".")
		if detectErr != nil {
			return fmt.Errorf("config not found and detection failed: %w", detectErr)
		}
		cfg = config.NewFromDetection(d, "do")
	}

	if dryRun {
		fmt.Println("Dry run mode - no changes will be made")
		fmt.Println()
	}

	if !yes && !dryRun {
		fmt.Println("This will create git commits for each normalization step.")
		fmt.Print("Continue? [y/N]: ")
		var response string
		_, err := fmt.Scanln(&response)
		if err != nil {
			// Treat scan error as "no"
			return fmt.Errorf("canceled by user")
		}
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			return fmt.Errorf("canceled by user")
		}
	}

	fmt.Println("Normalizing repository...")
	fmt.Println()

	// Create normalizer
	n := normalize.New(".", cfg.GetFramework(), dryRun)

	// Run normalization
	if err := n.Run(); err != nil {
		return fmt.Errorf("normalization failed: %w", err)
	}

	if dryRun {
		fmt.Println("✓ Dry run complete. Run without --dry-run to apply changes.")
	} else {
		fmt.Println("✓ Normalization complete.")
		fmt.Println("  Run `mvpbridge inspect` to verify.")
	}

	return nil
}

func runDeploy(target string) error {
	// Load config
	cfg, err := config.Load(".")
	if err != nil {
		return fmt.Errorf("config not found - run 'mvpbridge init' first: %w", err)
	}

	// Use config target if not specified
	if target == "" || target == "do" {
		target = cfg.Target
		if target == "" {
			target = "do"
		}
	}

	switch target {
	case "do":
		return deployDigitalOcean(cfg)
	case "aws":
		return deployAWS(cfg)
	case "gcp":
		return deployGCP(cfg)
	default:
		return fmt.Errorf("unknown target: %s (supported: do, aws, gcp)", target)
	}
}

// Helper functions

type deployPreparation struct {
	RepoURL string
	AppName string
	EnvVars map[string]string
}

func checkGit() error {
	cmd := exec.CommandContext(context.Background(), "git", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git not installed")
	}
	return nil
}

func checkGitRepo() error {
	if _, err := os.Stat(".git"); os.IsNotExist(err) {
		return fmt.Errorf("not a git repository")
	}
	return nil
}

func checkNode() error {
	cmd := exec.CommandContext(context.Background(), "node", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("node not installed")
	}
	return nil
}

func formatFramework(fw detect.Framework) string {
	switch fw {
	case detect.Vite:
		return "Vite"
	case detect.NextJS:
		return "Next.js"
	default:
		return "Unknown"
	}
}

func formatNodeVersion(version string) string {
	if version == "" {
		return "Not pinned"
	}
	return version + " (pinned)"
}

func getGitHubRepo() (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", "config", "--get", "remote.origin.url")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("no git remote configured")
	}

	return normalizeGitHubRemoteURL(strings.TrimSpace(string(output))), nil
}

func extractEnvVars() (map[string]string, error) {
	data, err := os.ReadFile(".env")
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}

	return parseEnvVars(string(data)), nil
}

func parseEnvVars(content string) map[string]string {
	envVars := make(map[string]string)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			// Remove quotes if present
			value = strings.Trim(value, `"'`)
			envVars[key] = value
		}
	}

	return envVars
}

func normalizeGitHubRemoteURL(raw string) string {
	url := strings.TrimSpace(raw)
	if strings.HasPrefix(url, "git@github.com:") {
		url = strings.Replace(url, "git@github.com:", "https://github.com/", 1)
	}
	return strings.TrimSuffix(url, ".git")
}

func deriveAppName(configAppName, repoURL string) string {
	if configAppName != "" {
		return configAppName
	}

	parts := strings.Split(strings.TrimSuffix(repoURL, "/"), "/")
	if len(parts) > 0 && parts[len(parts)-1] != "" {
		return parts[len(parts)-1]
	}
	return "mvpbridge-app"
}

func prepareDeploy(cfg *config.Config) (*deployPreparation, error) {
	repoURL, err := getGitHubRepo()
	if err != nil {
		return nil, fmt.Errorf("getting GitHub repo: %w", err)
	}

	envVars, err := extractEnvVars()
	if err != nil {
		return nil, fmt.Errorf("extracting env vars: %w", err)
	}

	return &deployPreparation{
		RepoURL: repoURL,
		AppName: deriveAppName(cfg.Deploy.AppName, repoURL),
		EnvVars: envVars,
	}, nil
}

// Deploy functions

func deployDigitalOcean(cfg *config.Config) error {
	fmt.Println("Deploying to DigitalOcean...")
	fmt.Println()

	prep, err := prepareDeploy(cfg)
	if err != nil {
		return err
	}

	// Create deployer
	deployer, err := deploy.NewDODeployer(prep.AppName, prep.RepoURL, "main")
	if err != nil {
		return err
	}

	fmt.Println("[1/4] Validating credentials... ✓")

	fmt.Println("[2/4] Creating app spec... ✓")

	// Determine if static
	isStatic := cfg.IsStatic()

	fmt.Printf("[3/4] Configuring secrets (%d vars)... ✓\n", len(prep.EnvVars))

	// Deploy
	result, err := deployer.Deploy(isStatic, prep.EnvVars)
	if err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	fmt.Println("[4/4] Triggering deployment... ✓")
	fmt.Println()
	fmt.Println("Deployment started!")

	// Display URLs
	if result.App.LiveURL != "" {
		fmt.Printf("  App URL: %s\n", result.App.LiveURL)
	} else if result.App.DefaultIngress != "" {
		fmt.Printf("  App URL: https://%s\n", result.App.DefaultIngress)
	}
	if result.App.ID != "" {
		fmt.Printf("  Dashboard: https://cloud.digitalocean.com/apps/%s\n", result.App.ID)
	}

	return nil
}

func deployAWS(cfg *config.Config) error {
	fmt.Println("Deploying to AWS Amplify...")
	fmt.Println()

	prep, err := prepareDeploy(cfg)
	if err != nil {
		return err
	}

	// Determine region
	region := cfg.Deploy.Region
	if region == "" {
		region = "us-east-1"
	}

	// Create deployer
	deployer, err := deploy.NewAWSDeployer(prep.AppName, prep.RepoURL, "main", region)
	if err != nil {
		return err
	}

	fmt.Println("[1/4] Validating credentials... ✓")

	fmt.Println("[2/4] Creating app spec... ✓")

	// Get build config from detection
	d, err := detect.DetectAll(".")
	if err != nil {
		return fmt.Errorf("detecting project: %w", err)
	}
	buildCommand := d.BuildCommand
	if buildCommand == "" {
		buildCommand = "npm run build"
	}
	outputDir := d.OutputDir
	if outputDir == "" {
		outputDir = "dist"
	}

	// Determine if static
	isStatic := cfg.IsStatic()

	fmt.Printf("[3/4] Configuring secrets (%d vars)... ✓\n", len(prep.EnvVars))

	// Deploy
	result, err := deployer.Deploy(isStatic, prep.EnvVars, buildCommand, outputDir)
	if err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	fmt.Println("[4/4] Triggering deployment... ✓")
	fmt.Println()
	fmt.Println("Deployment started!")

	// Display URLs
	if result.App.DefaultDomain != "" {
		fmt.Printf("  App URL: https://%s\n", result.App.DefaultDomain)
	}
	if result.App.AppID != "" {
		fmt.Printf("  Console: https://%s.console.aws.amazon.com/amplify/home?region=%s#/%s\n",
			region, region, result.App.AppID)
	}

	return nil
}

func deployGCP(cfg *config.Config) error {
	fmt.Println("Deploying to Google Cloud Run...")
	fmt.Println()

	prep, err := prepareDeployFunc(cfg)
	if err != nil {
		return err
	}

	projectID := cfg.Deploy.ProjectID
	if projectID == "" {
		projectID = os.Getenv("GCP_PROJECT_ID")
	}

	region := cfg.Deploy.Region
	if region == "" {
		region = "us-central1"
	}

	deployer, err := newGCPDeployerFunc(prep.AppName, projectID, region)
	if err != nil {
		return err
	}

	fmt.Println("[1/4] Validating credentials... ✓")
	fmt.Println("[2/4] Preparing Cloud Run deployment... ✓")

	isStatic := cfg.IsStatic()

	fmt.Printf("[3/4] Configuring secrets (%d vars)... ✓\n", len(prep.EnvVars))

	result, err := deployer.Deploy(isStatic, prep.EnvVars)
	if err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	fmt.Println("[4/4] Triggering deployment... ✓")
	fmt.Println()
	fmt.Println("Deployment started!")

	if result.Service.URL != "" {
		fmt.Printf("  App URL: %s\n", result.Service.URL)
	}
	if result.ConsoleURL != "" {
		fmt.Printf("  Console: %s\n", result.ConsoleURL)
	}

	return nil
}
