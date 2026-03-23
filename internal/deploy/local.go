package deploy

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/daryllundy/mvp-bridge/internal/detect"
)

const (
	localDeployDirName = "local-deploy"
	defaultWebPort     = "8080"
)

// LocalWorkspaceOptions configures generated local Docker Compose assets.
type LocalWorkspaceOptions struct {
	Root           string
	AppName        string
	Framework      detect.Framework
	OutputType     detect.OutputType
	PackageManager detect.PackageManager
	BuildCommand   string
	OutputDir      string
	EnvVars        map[string]string
}

// LocalWorkspaceResult summarizes generated assets.
type LocalWorkspaceResult struct {
	OutputDir    string
	Persistence  detect.Persistence
	OriginalRoot string
}

// GenerateLocalWorkspace writes a self-contained Docker Compose workspace under local-deploy.
func GenerateLocalWorkspace(opts LocalWorkspaceOptions) (*LocalWorkspaceResult, error) {
	root := opts.Root
	if root == "" {
		root = "."
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}

	outputDir := filepath.Join(absRoot, localDeployDirName)
	if err := os.RemoveAll(outputDir); err != nil {
		return nil, fmt.Errorf("reset local workspace: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(outputDir, "app"), 0o750); err != nil {
		return nil, fmt.Errorf("create local workspace: %w", err)
	}

	if err := copyProjectSnapshot(absRoot, filepath.Join(outputDir, "app")); err != nil {
		return nil, err
	}

	if opts.BuildCommand == "" {
		opts.BuildCommand = "npm run build"
	}
	if opts.OutputDir == "" {
		opts.OutputDir = "dist"
	}

	persistence := detect.DetectPersistence(absRoot)
	envVars := buildLocalEnvVars(opts.EnvVars, persistence)

	if err := os.WriteFile(filepath.Join(outputDir, ".env.app"), []byte(renderEnvFile(envVars)), 0o600); err != nil {
		return nil, fmt.Errorf("write app env file: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, ".env.example"), []byte(renderEnvTemplate(envVars)), 0o600); err != nil {
		return nil, fmt.Errorf("write env example: %w", err)
	}

	if err := os.WriteFile(filepath.Join(outputDir, "nginx.prod.conf"), []byte(renderNginxProdConfig(opts.OutputType)), 0o600); err != nil {
		return nil, fmt.Errorf("write prod nginx config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "nginx.dev.conf"), []byte(renderNginxDevConfig()), 0o600); err != nil {
		return nil, fmt.Errorf("write dev nginx config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "Dockerfile.web"), []byte(renderWebDockerfile(opts)), 0o600); err != nil {
		return nil, fmt.Errorf("write web Dockerfile: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "Dockerfile.app"), []byte(renderAppDockerfile(opts.PackageManager)), 0o600); err != nil {
		return nil, fmt.Errorf("write app Dockerfile: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "docker-compose.yml"), []byte(renderComposeProd(opts, persistence)), 0o600); err != nil {
		return nil, fmt.Errorf("write compose file: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "docker-compose.dev.yml"), []byte(renderComposeDev(opts, persistence)), 0o600); err != nil {
		return nil, fmt.Errorf("write dev compose file: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "README.md"), []byte(renderLocalReadme(opts, persistence)), 0o600); err != nil {
		return nil, fmt.Errorf("write README: %w", err)
	}

	return &LocalWorkspaceResult{
		OutputDir:    outputDir,
		Persistence:  persistence,
		OriginalRoot: absRoot,
	}, nil
}

func copyProjectSnapshot(srcRoot, dstRoot string) error {
	return filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		name := d.Name()
		if d.IsDir() && shouldSkipSnapshotDir(name) {
			return filepath.SkipDir
		}
		if !d.IsDir() && shouldSkipSnapshotFile(name) {
			return nil
		}

		dstPath := filepath.Join(dstRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o750)
		}

		return copyFile(path, dstPath)
	})
}

func shouldSkipSnapshotDir(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", ".next", "out", localDeployDirName:
		return true
	default:
		return false
	}
}

func shouldSkipSnapshotFile(name string) bool {
	return name == ".DS_Store"
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = srcFile.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer func() { _ = dstFile.Close() }()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}

	return nil
}

func buildLocalEnvVars(input map[string]string, persistence detect.Persistence) map[string]string {
	result := make(map[string]string, len(input)+4)
	for key, value := range input {
		result[key] = value
	}

	switch persistence.Kind {
	case detect.PersistencePostgres:
		result["POSTGRES_DB"] = "app"
		result["POSTGRES_USER"] = "postgres"
		result["POSTGRES_PASSWORD"] = "postgres"
		result["DATABASE_URL"] = "postgresql://postgres:postgres@db:5432/app"
	case detect.PersistenceSQLite:
		if _, ok := result["DATABASE_URL"]; !ok {
			result["DATABASE_URL"] = "file:/data/app.sqlite"
		}
		result["SQLITE_PATH"] = "/data/app.sqlite"
	}

	return result
}

func renderEnvFile(envVars map[string]string) string {
	keys := sortedKeys(envVars)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%s\n", key, envVars[key])
	}
	return b.String()
}

func renderEnvTemplate(envVars map[string]string) string {
	keys := sortedKeys(envVars)
	var b strings.Builder
	b.WriteString("# Generated by mvpbridge for local Compose use.\n")
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=\n", key)
	}
	return b.String()
}

func renderNginxProdConfig(outputType detect.OutputType) string {
	if outputType == detect.SSR {
		return `events {}
http {
    server {
        listen 80;
        server_name _;

        location / {
            proxy_pass http://app:3000;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }
    }
}
`
	}

	return `events {}
http {
    server {
        listen 80;
        server_name _;
        root /usr/share/nginx/html;
        index index.html;

        location / {
            try_files $uri $uri/ /index.html;
        }
    }
}
`
}

func renderNginxDevConfig() string {
	return `events {}
http {
    server {
        listen 80;
        server_name _;

        location / {
            proxy_pass http://app:3000;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
            proxy_set_header Host $host;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        }
    }
}
`
}

func renderWebDockerfile(opts LocalWorkspaceOptions) string {
	switch opts.OutputType {
	case detect.SSR:
		return `FROM nginx:alpine
COPY nginx.prod.conf /etc/nginx/nginx.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
`
	default:
		return fmt.Sprintf(`FROM node:20-alpine AS builder
WORKDIR /app
COPY app/package*.json ./
RUN npm install
COPY app/ .
RUN %s

FROM nginx:alpine
COPY nginx.prod.conf /etc/nginx/nginx.conf
COPY --from=builder /app/%s /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
`, opts.BuildCommand, opts.OutputDir)
	}
}

func renderAppDockerfile(pm detect.PackageManager) string {
	installCmd := "npm install"
	switch pm {
	case detect.Yarn:
		installCmd = "yarn install"
	case detect.PNPM:
		installCmd = "npm install -g pnpm && pnpm install"
	}

	return fmt.Sprintf(`FROM node:20-alpine
WORKDIR /workspace
COPY app/ .
RUN %s
ENV NODE_ENV=production
EXPOSE 3000
CMD ["sh", "-lc", "%s"]
`, installCmd, shellCommandForStart(pm))
}

func renderComposeProd(opts LocalWorkspaceOptions, persistence detect.Persistence) string {
	var b strings.Builder
	b.WriteString("services:\n")
	if opts.OutputType == detect.SSR {
		b.WriteString("  app:\n")
		b.WriteString("    build:\n")
		b.WriteString("      context: .\n")
		b.WriteString("      dockerfile: Dockerfile.app\n")
		b.WriteString("    env_file:\n")
		b.WriteString("      - .env.app\n")
		b.WriteString("    expose:\n")
		b.WriteString("      - \"3000\"\n")
		if persistence.Kind == detect.PersistencePostgres {
			b.WriteString("    depends_on:\n")
			b.WriteString("      - db\n")
		}
		if persistence.Kind == detect.PersistenceSQLite {
			b.WriteString("    volumes:\n")
			b.WriteString("      - sqlite-data:/data\n")
		}
	}

	b.WriteString("  web:\n")
	b.WriteString("    build:\n")
	b.WriteString("      context: .\n")
	b.WriteString("      dockerfile: Dockerfile.web\n")
	b.WriteString("    ports:\n")
	b.WriteString("      - \"" + defaultWebPort + ":80\"\n")
	if opts.OutputType == detect.SSR {
		b.WriteString("    depends_on:\n")
		b.WriteString("      - app\n")
	}

	if persistence.Kind == detect.PersistencePostgres {
		b.WriteString("  db:\n")
		b.WriteString("    image: postgres:16-alpine\n")
		b.WriteString("    environment:\n")
		b.WriteString("      POSTGRES_DB: app\n")
		b.WriteString("      POSTGRES_USER: postgres\n")
		b.WriteString("      POSTGRES_PASSWORD: postgres\n")
		b.WriteString("    volumes:\n")
		b.WriteString("      - postgres-data:/var/lib/postgresql/data\n")
	}

	if persistence.Kind == detect.PersistencePostgres || persistence.Kind == detect.PersistenceSQLite {
		b.WriteString("volumes:\n")
		if persistence.Kind == detect.PersistencePostgres {
			b.WriteString("  postgres-data:\n")
		}
		if persistence.Kind == detect.PersistenceSQLite {
			b.WriteString("  sqlite-data:\n")
		}
	}

	return b.String()
}

func renderComposeDev(opts LocalWorkspaceOptions, persistence detect.Persistence) string {
	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  app:\n")
	b.WriteString("    build:\n")
	b.WriteString("      context: .\n")
	b.WriteString("      dockerfile: Dockerfile.app\n")
	b.WriteString("    command: sh -lc \"" + shellEscape(devCommand(opts.PackageManager)) + "\"\n")
	b.WriteString("    env_file:\n")
	b.WriteString("      - .env.app\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - ./app:/workspace\n")
	if persistence.Kind == detect.PersistenceSQLite {
		b.WriteString("      - sqlite-data:/data\n")
	}
	b.WriteString("    expose:\n")
	b.WriteString("      - \"3000\"\n")
	if persistence.Kind == detect.PersistencePostgres {
		b.WriteString("    depends_on:\n")
		b.WriteString("      - db\n")
	}

	b.WriteString("  web:\n")
	b.WriteString("    image: nginx:alpine\n")
	b.WriteString("    ports:\n")
	b.WriteString("      - \"" + defaultWebPort + ":80\"\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - ./nginx.dev.conf:/etc/nginx/nginx.conf:ro\n")
	b.WriteString("    depends_on:\n")
	b.WriteString("      - app\n")

	if persistence.Kind == detect.PersistencePostgres {
		b.WriteString("  db:\n")
		b.WriteString("    image: postgres:16-alpine\n")
		b.WriteString("    environment:\n")
		b.WriteString("      POSTGRES_DB: app\n")
		b.WriteString("      POSTGRES_USER: postgres\n")
		b.WriteString("      POSTGRES_PASSWORD: postgres\n")
		b.WriteString("    volumes:\n")
		b.WriteString("      - postgres-data:/var/lib/postgresql/data\n")
	}

	if persistence.Kind == detect.PersistencePostgres || persistence.Kind == detect.PersistenceSQLite {
		b.WriteString("volumes:\n")
		if persistence.Kind == detect.PersistencePostgres {
			b.WriteString("  postgres-data:\n")
		}
		if persistence.Kind == detect.PersistenceSQLite {
			b.WriteString("  sqlite-data:\n")
		}
	}

	return b.String()
}

func renderLocalReadme(opts LocalWorkspaceOptions, persistence detect.Persistence) string {
	note := "No persistence backend was inferred."
	switch persistence.Kind {
	case detect.PersistencePostgres:
		note = "Postgres was inferred and a db service was generated."
	case detect.PersistenceSQLite:
		note = "SQLite was inferred and a persistent sqlite-data volume was generated."
	}

	return fmt.Sprintf("# Local Deploy Workspace\n\n"+
"This directory was generated by mvpbridge for `%s`.\n\n"+
"## Run\n\n"+
"Production-like:\n\n"+
"```bash\n"+
"docker compose up --build\n"+
"```\n\n"+
"Dev variant:\n\n"+
"```bash\n"+
"docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build\n"+
"```\n\n"+
"## Notes\n\n"+
"- The app snapshot lives in `app/`.\n"+
"- The dev variant mounts `./app` into the app container for iteration inside the generated workspace.\n"+
"- %s\n",
		opts.AppName, note)
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func shellCommandForStart(pm detect.PackageManager) string {
	switch pm {
	case detect.Yarn:
		return "yarn start"
	case detect.PNPM:
		return "pnpm start"
	default:
		return "npm run start"
	}
}

func devCommand(pm detect.PackageManager) string {
	switch pm {
	case detect.Yarn:
		return "yarn dev --host 0.0.0.0 --port 3000"
	case detect.PNPM:
		return "pnpm dev -- --host 0.0.0.0 --port 3000"
	default:
		return "npm run dev -- --host 0.0.0.0 --port 3000"
	}
}

func shellEscape(value string) string {
	return strings.ReplaceAll(value, `"`, `\"`)
}
