// Package detect provides framework detection and project analysis capabilities
// for MVPBridge. It identifies frontend frameworks, build configurations, and
// deployment readiness issues.
package detect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Framework represents a supported frontend framework type
type Framework string

const (
	// Vite represents a Vite-based project
	Vite Framework = "vite"
	// NextJS represents a Next.js project
	NextJS Framework = "nextjs"
	// Unknown represents an unrecognized framework
	Unknown Framework = "unknown"
)

// OutputType represents the output type of a build (static or server-side rendered)
type OutputType string

const (
	// Static represents a static site build output
	Static OutputType = "static"
	// SSR represents a server-side rendered application
	SSR OutputType = "ssr"
)

// PackageManager represents the package manager used by the project
type PackageManager string

const (
	// NPM represents the npm package manager
	NPM PackageManager = "npm"
	// Yarn represents the Yarn package manager
	Yarn PackageManager = "yarn"
	// PNPM represents the pnpm package manager
	PNPM PackageManager = "pnpm"
)

// Detection holds all detected project information and deployment issues
type Detection struct {
	Framework      Framework
	OutputType     OutputType
	PackageManager PackageManager
	NodeVersion    string
	BuildCommand   string
	OutputDir      string
	Issues         []Issue
}

// Issue represents a deployment readiness issue that was detected
type Issue struct {
	Code        string
	Description string
	Fixable     bool
}

type packageJSON struct {
	Name            string            `json:"name"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Engines         struct {
		Node string `json:"node"`
	} `json:"engines"`
}

// DetectAll runs all detection logic and returns a complete report
func DetectAll(root string) (*Detection, error) {
	d := &Detection{
		Issues: make([]Issue, 0),
	}

	// Detect framework
	fw, err := DetectFramework(root)
	if err != nil {
		d.Framework = Unknown
		d.Issues = append(d.Issues, Issue{
			Code:        "UNKNOWN_FRAMEWORK",
			Description: "Could not detect framework",
			Fixable:     false,
		})
	} else {
		d.Framework = fw
	}

	// Detect package manager
	d.PackageManager = DetectPackageManager(root)

	// Detect Node version
	d.NodeVersion = DetectNodeVersion(root)
	if d.NodeVersion == "" {
		d.Issues = append(d.Issues, Issue{
			Code:        "NODE_NOT_PINNED",
			Description: "Node version not pinned",
			Fixable:     true,
		})
	}

	// Detect build command and output
	d.BuildCommand, d.OutputDir = DetectBuildConfig(root, d.Framework)

	// Detect output type
	d.OutputType = DetectOutputType(root, d.Framework)

	// Check for missing files
	d.Issues = append(d.Issues, CheckMissingFiles(root)...)

	return d, nil
}

// DetectFramework determines which framework the project uses
func DetectFramework(root string) (Framework, error) {
	// Check for Next.js first (more specific)
	nextConfigs := []string{"next.config.js", "next.config.mjs", "next.config.ts"}
	for _, cfg := range nextConfigs {
		if fileExists(filepath.Join(root, cfg)) {
			return NextJS, nil
		}
	}

	// Check for Vite
	viteConfigs := []string{"vite.config.js", "vite.config.ts", "vite.config.mjs"}
	for _, cfg := range viteConfigs {
		if fileExists(filepath.Join(root, cfg)) {
			return Vite, nil
		}
	}

	// Fallback: check package.json dependencies
	pkg, err := readPackageJSON(root)
	if err == nil {
		if _, hasNext := pkg.Dependencies["next"]; hasNext {
			return NextJS, nil
		}
		if _, hasVite := pkg.DevDependencies["vite"]; hasVite {
			return Vite, nil
		}
	}

	return Unknown, fmt.Errorf("no framework detected")
}

// DetectPackageManager determines npm, yarn, or pnpm
func DetectPackageManager(root string) PackageManager {
	if fileExists(filepath.Join(root, "pnpm-lock.yaml")) {
		return PNPM
	}
	if fileExists(filepath.Join(root, "yarn.lock")) {
		return Yarn
	}
	return NPM
}

// DetectNodeVersion finds pinned Node version
func DetectNodeVersion(root string) string {
	// Check .nvmrc first
	nvmrc := filepath.Join(root, ".nvmrc")
	if data, err := os.ReadFile(nvmrc); err == nil {
		return strings.TrimSpace(string(data))
	}

	// Check package.json engines
	pkg, err := readPackageJSON(root)
	if err == nil && pkg.Engines.Node != "" {
		return pkg.Engines.Node
	}

	return ""
}

// DetectBuildConfig returns build command and output directory
func DetectBuildConfig(root string, fw Framework) (buildCmd, outputDir string) {
	pkg, err := readPackageJSON(root)
	if err != nil {
		return "", ""
	}

	// Get build command from scripts
	if cmd, ok := pkg.Scripts["build"]; ok {
		buildCmd = cmd
	}

	// Determine output directory based on framework
	switch fw {
	case Vite:
		outputDir = "dist"
	case NextJS:
		if strings.Contains(buildCmd, "export") {
			outputDir = "out"
		} else {
			outputDir = ".next"
		}
	}

	return buildCmd, outputDir
}

// DetectOutputType determines if output is static or SSR
func DetectOutputType(root string, fw Framework) OutputType {
	switch fw {
	case Vite:
		return Static // Vite is always static by default
	case NextJS:
		// Check next.config for output: 'export'
		for _, cfg := range []string{"next.config.js", "next.config.mjs"} {
			path := filepath.Join(root, cfg)
			if data, err := os.ReadFile(path); err == nil {
				content := string(data)
				if strings.Contains(content, `output: "export"`) ||
					strings.Contains(content, `output: 'export'`) {
					return Static
				}
			}
		}
		return SSR
	}
	return Static
}

// CheckMissingFiles returns issues for missing deployment files
func CheckMissingFiles(root string) []Issue {
	var issues []Issue

	checks := []struct {
		path        string
		code        string
		description string
	}{
		{"Dockerfile", "MISSING_DOCKERFILE", "Missing Dockerfile"},
		{".env.example", "MISSING_ENV_EXAMPLE", "No .env.example"},
		{".github/workflows", "MISSING_GHA", "No GitHub Actions workflow"},
		{".gitignore", "MISSING_GITIGNORE", "No .gitignore"},
	}

	for _, c := range checks {
		if !fileExists(filepath.Join(root, c.path)) {
			issues = append(issues, Issue{
				Code:        c.code,
				Description: c.description,
				Fixable:     true,
			})
		}
	}

	return issues
}

// Helper functions

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readPackageJSON(root string) (*packageJSON, error) {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil, err
	}

	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	return &pkg, nil
}
