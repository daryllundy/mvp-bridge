// Package detect provides framework detection and project analysis capabilities
// for MVPBridge. It identifies frontend frameworks, build configurations, and
// deployment readiness issues.
package detect

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/daryllundy/mvp-bridge/internal/projectfs"
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

// PersistenceKind describes the best-effort persistence backend inferred from the repo.
type PersistenceKind string

const (
	// PersistenceNone means no persistence backend was confidently inferred.
	PersistenceNone PersistenceKind = "none"
	// PersistencePostgres means the repo appears to use PostgreSQL.
	PersistencePostgres PersistenceKind = "postgres"
	// PersistenceSQLite means the repo appears to use SQLite.
	PersistenceSQLite PersistenceKind = "sqlite"
)

// Persistence stores the inferred persistence backend and why it was chosen.
type Persistence struct {
	Kind   PersistenceKind
	Reason string
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

type scanContext struct {
	root      string
	pkg       *packageJSON
	pkgErr    error
	pkgLoaded bool

	fileData map[string][]byte
	fileErr  map[string]error
}

func newScanContext(root string) *scanContext {
	return &scanContext{
		root:     root,
		fileData: make(map[string][]byte),
		fileErr:  make(map[string]error),
	}
}

func (s *scanContext) readPackageJSON() (*packageJSON, error) {
	if s.pkgLoaded {
		return s.pkg, s.pkgErr
	}

	s.pkgLoaded = true
	s.pkg, s.pkgErr = readPackageJSON(s.root)
	return s.pkg, s.pkgErr
}

func (s *scanContext) readFile(rel string) ([]byte, error) {
	if data, ok := s.fileData[rel]; ok {
		return data, nil
	}
	if err, ok := s.fileErr[rel]; ok {
		return nil, err
	}

	data, err := readFileInRoot(s.root, rel)
	if err != nil {
		s.fileErr[rel] = err
		return nil, err
	}

	s.fileData[rel] = data
	return data, nil
}

// DetectAll runs all detection logic and returns a complete report
func DetectAll(root string) (*Detection, error) {
	scan := newScanContext(root)
	d := &Detection{
		Issues: make([]Issue, 0),
	}

	// Detect framework
	fw, err := detectFramework(scan)
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
	d.NodeVersion = detectNodeVersion(scan)
	if d.NodeVersion == "" {
		d.Issues = append(d.Issues, Issue{
			Code:        "NODE_NOT_PINNED",
			Description: "Node version not pinned",
			Fixable:     true,
		})
	}

	// Detect build command and output
	d.BuildCommand, d.OutputDir = detectBuildConfig(scan, d.Framework)

	// Detect output type
	d.OutputType = detectOutputType(scan, d.Framework)

	// Check for missing files
	d.Issues = append(d.Issues, CheckMissingFiles(root)...)

	return d, nil
}

// DetectFramework determines which framework the project uses
func DetectFramework(root string) (Framework, error) {
	return detectFramework(newScanContext(root))
}

func detectFramework(scan *scanContext) (Framework, error) {
	// Check for Next.js first (more specific)
	nextConfigs := []string{"next.config.js", "next.config.mjs", "next.config.ts"}
	for _, cfg := range nextConfigs {
		if projectfs.Exists(filepath.Join(scan.root, cfg)) {
			return NextJS, nil
		}
	}

	// Check for Vite
	viteConfigs := []string{"vite.config.js", "vite.config.ts", "vite.config.mjs"}
	for _, cfg := range viteConfigs {
		if projectfs.Exists(filepath.Join(scan.root, cfg)) {
			return Vite, nil
		}
	}

	// Fallback: check package.json dependencies
	pkg, err := scan.readPackageJSON()
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
	if projectfs.Exists(filepath.Join(root, "pnpm-lock.yaml")) {
		return PNPM
	}
	if projectfs.Exists(filepath.Join(root, "yarn.lock")) {
		return Yarn
	}
	return NPM
}

// DetectNodeVersion finds pinned Node version
func DetectNodeVersion(root string) string {
	return detectNodeVersion(newScanContext(root))
}

func detectNodeVersion(scan *scanContext) string {
	// Check .nvmrc first
	if data, err := scan.readFile(".nvmrc"); err == nil {
		return strings.TrimSpace(string(data))
	}

	// Check package.json engines
	pkg, err := scan.readPackageJSON()
	if err == nil && pkg.Engines.Node != "" {
		return pkg.Engines.Node
	}

	return ""
}

// DetectBuildConfig returns build command and output directory
func DetectBuildConfig(root string, fw Framework) (buildCmd, outputDir string) {
	return detectBuildConfig(newScanContext(root), fw)
}

func detectBuildConfig(scan *scanContext, fw Framework) (buildCmd, outputDir string) {
	pkg, err := scan.readPackageJSON()
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

// DetectPersistence infers whether the current repo expects PostgreSQL or SQLite.
func DetectPersistence(root string) Persistence {
	scan := newScanContext(root)
	postScore := 0
	sqliteScore := 0
	var reasons []string

	pkg, err := scan.readPackageJSON()
	if err == nil {
		postDeps := []string{"pg", "postgres", "@neondatabase/serverless", "postgresql", "pg-pool"}
		sqliteDeps := []string{"sqlite3", "better-sqlite3", "@libsql/client", "sql.js"}

		for _, dep := range postDeps {
			if hasDependency(pkg, dep) {
				postScore += 2
				reasons = append(reasons, "dependency:"+dep)
			}
		}
		for _, dep := range sqliteDeps {
			if hasDependency(pkg, dep) {
				sqliteScore += 2
				reasons = append(reasons, "dependency:"+dep)
			}
		}
	}

	for _, rel := range likelyPersistenceFiles(root) {
		data, err := scan.readFile(rel)
		if err != nil {
			continue
		}
		content := strings.ToLower(string(data))

		if strings.Contains(content, `provider = "postgresql"`) ||
			strings.Contains(content, "postgresql://") ||
			strings.Contains(content, "postgres://") ||
			strings.Contains(content, "pg_host") ||
			strings.Contains(content, "postgres_user") {
			postScore += 2
			reasons = append(reasons, rel)
		}

		if strings.Contains(content, `provider = "sqlite"`) ||
			strings.Contains(content, ".sqlite") ||
			strings.Contains(content, ".db") ||
			strings.Contains(content, "file:./") ||
			strings.Contains(content, "better-sqlite3") {
			sqliteScore += 2
			reasons = append(reasons, rel)
		}

		if strings.Contains(content, "database_url") || strings.Contains(content, "pghost") || strings.Contains(content, "postgres_") {
			postScore++
		}
	}

	switch {
	case postScore >= 2 && postScore > sqliteScore:
		return Persistence{Kind: PersistencePostgres, Reason: strings.Join(reasons, ", ")}
	case sqliteScore >= 2 && sqliteScore > postScore:
		return Persistence{Kind: PersistenceSQLite, Reason: strings.Join(reasons, ", ")}
	default:
		return Persistence{Kind: PersistenceNone}
	}
}

func hasDependency(pkg *packageJSON, name string) bool {
	if pkg == nil {
		return false
	}
	if _, ok := pkg.Dependencies[name]; ok {
		return true
	}
	if _, ok := pkg.DevDependencies[name]; ok {
		return true
	}
	return false
}

func likelyPersistenceFiles(root string) []string {
	paths := []string{".env", ".env.example", "prisma/schema.prisma", "knexfile.js", "knexfile.ts", "drizzle.config.js", "drizzle.config.ts"}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		seen[path] = struct{}{}
	}

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", ".next", "out", "local-deploy":
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()
		switch name {
		case ".env", ".env.example", "schema.prisma", "knexfile.js", "knexfile.ts", "drizzle.config.js", "drizzle.config.ts":
			rel, relErr := filepath.Rel(root, path)
			if relErr == nil {
				rel = filepath.ToSlash(rel)
				if _, ok := seen[rel]; !ok {
					seen[rel] = struct{}{}
					paths = append(paths, rel)
				}
			}
		}

		return nil
	})

	return paths
}

// DetectOutputType determines if output is static or SSR
func DetectOutputType(root string, fw Framework) OutputType {
	return detectOutputType(newScanContext(root), fw)
}

func detectOutputType(scan *scanContext, fw Framework) OutputType {
	switch fw {
	case Vite:
		return Static // Vite is always static by default
	case NextJS:
		// Check next.config for output: 'export'
		for _, cfg := range []string{"next.config.js", "next.config.mjs"} {
			if data, err := scan.readFile(cfg); err == nil {
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
		if !projectfs.Exists(filepath.Join(root, c.path)) {
			issues = append(issues, Issue{
				Code:        c.code,
				Description: c.description,
				Fixable:     true,
			})
		}
	}

	return issues
}

func readPackageJSON(root string) (*packageJSON, error) {
	data, err := readFileInRoot(root, "package.json")
	if err != nil {
		return nil, err
	}

	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	return &pkg, nil
}

func readFileInRoot(root, rel string) ([]byte, error) {
	return projectfs.ReadFileInRoot(root, rel)
}
