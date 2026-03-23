package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daryllundy/mvp-bridge/internal/detect"
)

func TestGenerateLocalWorkspaceStaticVite(t *testing.T) {
	root := t.TempDir()
	writeLocalTestFile(t, filepath.Join(root, "package.json"), `{"scripts":{"build":"npm run build","dev":"vite","start":"vite preview"},"devDependencies":{"vite":"^5.0.0"}}`)
	writeLocalTestFile(t, filepath.Join(root, "vite.config.js"), "export default {}")
	writeLocalTestFile(t, filepath.Join(root, "src", "main.ts"), "console.log('hello')")
	writeLocalTestFile(t, filepath.Join(root, ".env"), "API_URL=https://example.com\n")
	writeLocalTestFile(t, filepath.Join(root, "node_modules", "ignored"), "x")
	writeLocalTestFile(t, filepath.Join(root, ".git", "ignored"), "x")

	result, err := GenerateLocalWorkspace(LocalWorkspaceOptions{
		Root:           root,
		AppName:        "demo-app",
		Framework:      detect.Vite,
		OutputType:     detect.Static,
		PackageManager: detect.NPM,
		BuildCommand:   "npm run build",
		OutputDir:      "dist",
		EnvVars: map[string]string{
			"API_URL": "https://example.com",
		},
	})
	if err != nil {
		t.Fatalf("GenerateLocalWorkspace returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(result.OutputDir, "docker-compose.yml")); err != nil {
		t.Fatalf("expected docker-compose.yml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.OutputDir, "docker-compose.dev.yml")); err != nil {
		t.Fatalf("expected docker-compose.dev.yml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.OutputDir, "app", "src", "main.ts")); err != nil {
		t.Fatalf("expected copied app source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.OutputDir, "app", "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("expected node_modules to be excluded")
	}

	composeData := readLocalTestFile(t, filepath.Join(result.OutputDir, "docker-compose.yml"))
	if !strings.Contains(composeData, "web:") {
		t.Fatalf("expected web service in compose file")
	}
	if strings.Contains(composeData, "db:") {
		t.Fatalf("did not expect db service without inferred persistence")
	}

	webDockerfile := readLocalTestFile(t, filepath.Join(result.OutputDir, "Dockerfile.web"))
	if !strings.Contains(webDockerfile, "COPY --from=builder /app/dist /usr/share/nginx/html") {
		t.Fatalf("expected static web Dockerfile to copy dist, got %q", webDockerfile)
	}
}

func TestGenerateLocalWorkspaceSSRWithPostgres(t *testing.T) {
	root := t.TempDir()
	writeLocalTestFile(t, filepath.Join(root, "package.json"), `{"scripts":{"build":"next build","dev":"next dev","start":"next start"},"dependencies":{"next":"14.0.0","pg":"^8.0.0"}}`)
	writeLocalTestFile(t, filepath.Join(root, "next.config.js"), "module.exports = {}")
	writeLocalTestFile(t, filepath.Join(root, ".env"), "DATABASE_URL=postgresql://remote/db\n")

	result, err := GenerateLocalWorkspace(LocalWorkspaceOptions{
		Root:           root,
		AppName:        "next-app",
		Framework:      detect.NextJS,
		OutputType:     detect.SSR,
		PackageManager: detect.NPM,
		BuildCommand:   "next build",
		OutputDir:      ".next",
		EnvVars: map[string]string{
			"DATABASE_URL": "postgresql://remote/db",
		},
	})
	if err != nil {
		t.Fatalf("GenerateLocalWorkspace returned error: %v", err)
	}

	if result.Persistence.Kind != detect.PersistencePostgres {
		t.Fatalf("expected postgres persistence, got %s", result.Persistence.Kind)
	}

	composeData := readLocalTestFile(t, filepath.Join(result.OutputDir, "docker-compose.yml"))
	if !strings.Contains(composeData, "app:") {
		t.Fatalf("expected app service for SSR")
	}
	if !strings.Contains(composeData, "db:") {
		t.Fatalf("expected db service for postgres")
	}
	if !strings.Contains(composeData, "postgres-data") {
		t.Fatalf("expected postgres volume")
	}

	nginxData := readLocalTestFile(t, filepath.Join(result.OutputDir, "nginx.prod.conf"))
	if !strings.Contains(nginxData, "proxy_pass http://app:3000;") {
		t.Fatalf("expected nginx to proxy to app service")
	}

	envData := readLocalTestFile(t, filepath.Join(result.OutputDir, ".env.app"))
	if !strings.Contains(envData, "DATABASE_URL=postgresql://postgres:postgres@db:5432/app") {
		t.Fatalf("expected local DATABASE_URL override, got %q", envData)
	}
}

func TestGenerateLocalWorkspaceSQLite(t *testing.T) {
	root := t.TempDir()
	writeLocalTestFile(t, filepath.Join(root, "package.json"), `{"scripts":{"build":"next build","dev":"next dev","start":"next start"},"dependencies":{"next":"14.0.0","better-sqlite3":"^9.0.0"}}`)
	writeLocalTestFile(t, filepath.Join(root, "next.config.js"), "module.exports = {}")
	writeLocalTestFile(t, filepath.Join(root, "prisma", "schema.prisma"), `datasource db { provider = "sqlite" url = "file:./dev.db" }`)

	result, err := GenerateLocalWorkspace(LocalWorkspaceOptions{
		Root:           root,
		AppName:        "sqlite-app",
		Framework:      detect.NextJS,
		OutputType:     detect.SSR,
		PackageManager: detect.NPM,
		BuildCommand:   "next build",
		OutputDir:      ".next",
		EnvVars:        map[string]string{},
	})
	if err != nil {
		t.Fatalf("GenerateLocalWorkspace returned error: %v", err)
	}

	if result.Persistence.Kind != detect.PersistenceSQLite {
		t.Fatalf("expected sqlite persistence, got %s", result.Persistence.Kind)
	}

	composeData := readLocalTestFile(t, filepath.Join(result.OutputDir, "docker-compose.yml"))
	if strings.Contains(composeData, "db:") {
		t.Fatalf("did not expect postgres db service for sqlite")
	}
	if !strings.Contains(composeData, "sqlite-data") {
		t.Fatalf("expected sqlite volume")
	}
}

func writeLocalTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readLocalTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
