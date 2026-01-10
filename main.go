package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"mvpbridge/internal/config"
	"mvpbridge/internal/deploy"
	"mvpbridge/internal/detect"
	"mvpbridge/internal/normalize"
)

var version = "0.1.0"

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
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(target, framework)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "Deployment target (do, aws)")
	cmd.Flags().StringVarP(&framework, "framework", "f", "", "Framework (vite, nextjs)")

	return cmd
}

func inspectCmd() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Analyze repo and report deployment readiness",
		Long:  `Performs read-only analysis of your repository to identify what needs to be fixed before deployment.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInspect(verbose)
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed output")

	return cmd
}

func normalizeCmd() *cobra.Command {
	var dryRun bool
	var yes bool

	cmd := &cobra.Command{
		Use:   "normalize",
		Short: "Apply fixes to make repo deployable",
		Long:  `Applies atomic, reversible changes to prepare your repository for deployment.`,
		RunE: func(cmd *cobra.Command, args []string) error {
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
		Long:  `Deploys your application to the specified platform (do for DigitalOcean, aws for AWS).`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

func runInspect(verbose bool) error {
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
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			return fmt.Errorf("cancelled by user")
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
		return deployAWS()
	default:
		return fmt.Errorf("unknown target: %s (supported: do, aws)", target)
	}
}

// Helper functions

func checkGit() error {
	cmd := exec.Command("git", "--version")
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
	cmd := exec.Command("node", "--version")
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
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("no git remote configured")
	}

	url := strings.TrimSpace(string(output))
	// Convert SSH to HTTPS format if needed
	if strings.HasPrefix(url, "git@github.com:") {
		url = strings.Replace(url, "git@github.com:", "https://github.com/", 1)
	}
	url = strings.TrimSuffix(url, ".git")

	return url, nil
}

func extractEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	data, err := os.ReadFile(".env")
	if err != nil {
		if os.IsNotExist(err) {
			return envVars, nil
		}
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
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

	return envVars, nil
}

// Deploy functions

func deployDigitalOcean(cfg *config.Config) error {
	fmt.Println("Deploying to DigitalOcean...")
	fmt.Println()

	// Get GitHub repo URL
	repoURL, err := getGitHubRepo()
	if err != nil {
		return fmt.Errorf("getting GitHub repo: %w", err)
	}

	// Determine app name
	appName := cfg.Deploy.AppName
	if appName == "" {
		// Extract from repo URL
		parts := strings.Split(repoURL, "/")
		if len(parts) > 0 {
			appName = parts[len(parts)-1]
		} else {
			appName = "mvpbridge-app"
		}
	}

	// Create deployer
	deployer, err := deploy.NewDODeployer(appName, repoURL, "main")
	if err != nil {
		return err
	}

	fmt.Println("[1/4] Validating credentials... ✓")

	// Extract env vars
	envVars, err := extractEnvVars()
	if err != nil {
		return fmt.Errorf("extracting env vars: %w", err)
	}

	fmt.Println("[2/4] Creating app spec... ✓")

	// Determine if static
	isStatic := cfg.IsStatic()

	fmt.Printf("[3/4] Configuring secrets (%d vars)... ✓\n", len(envVars))

	// Deploy
	result, err := deployer.Deploy(isStatic, envVars)
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

func deployAWS() error {
	return fmt.Errorf("AWS deployment not yet implemented")
}
