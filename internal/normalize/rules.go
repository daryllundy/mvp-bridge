package normalize

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mvpbridge/internal/detect"
)

type Rule struct {
	Name        string
	Description string
	Check       func(root string) bool
	Apply       func(root string, dryRun bool) error
}

type Normalizer struct {
	Root      string
	DryRun    bool
	Framework detect.Framework
	Rules     []Rule
}

func New(root string, fw detect.Framework, dryRun bool) *Normalizer {
	n := &Normalizer{
		Root:      root,
		DryRun:    dryRun,
		Framework: fw,
	}

	// Add universal rules
	n.Rules = append(n.Rules, universalRules()...)

	// Add framework-specific rules
	switch fw {
	case detect.Vite:
		n.Rules = append(n.Rules, viteRules()...)
	case detect.NextJS:
		n.Rules = append(n.Rules, nextjsRules()...)
	}

	return n
}

func (n *Normalizer) Run() error {
	for i, rule := range n.Rules {
		// Check if rule needs to be applied
		if rule.Check(n.Root) {
			fmt.Printf("[%d/%d] %s - already satisfied ✓\n", i+1, len(n.Rules), rule.Name)
			continue
		}

		fmt.Printf("[%d/%d] %s\n", i+1, len(n.Rules), rule.Name)

		if err := rule.Apply(n.Root, n.DryRun); err != nil {
			fmt.Printf("      → Error: %v\n", err)
			return err
		}

		if !n.DryRun {
			// Commit the change
			if err := gitCommit(n.Root, rule.Description); err != nil {
				fmt.Printf("      → Commit error: %v\n", err)
			} else {
				fmt.Printf("      → Committed: [mvpbridge] %s\n", rule.Description)
			}
		} else {
			fmt.Printf("      → Would commit: [mvpbridge] %s\n", rule.Description)
		}
		fmt.Println()
	}

	return nil
}

// Universal rules apply to all frameworks

func universalRules() []Rule {
	return []Rule{
		{
			Name:        "Pin Node version",
			Description: "Pin Node version to 20",
			Check: func(root string) bool {
				return fileExists(filepath.Join(root, ".nvmrc"))
			},
			Apply: func(root string, dryRun bool) error {
				if dryRun {
					return nil
				}
				return os.WriteFile(filepath.Join(root, ".nvmrc"), []byte("20\n"), 0600)
			},
		},
		{
			Name:        "Add .env.example",
			Description: "Add .env.example template",
			Check: func(root string) bool {
				return fileExists(filepath.Join(root, ".env.example"))
			},
			Apply: func(root string, dryRun bool) error {
				if dryRun {
					return nil
				}
				return createEnvExample(root)
			},
		},
		{
			Name:        "Update .gitignore",
			Description: "Update .gitignore with standard entries",
			Check:       gitignoreComplete,
			Apply: func(root string, dryRun bool) error {
				if dryRun {
					return nil
				}
				return updateGitignore(root)
			},
		},
		{
			Name:        "Add GitHub Actions workflow",
			Description: "Add deployment workflow",
			Check: func(root string) bool {
				return fileExists(filepath.Join(root, ".github/workflows/deploy.yml"))
			},
			Apply: func(root string, dryRun bool) error {
				if dryRun {
					return nil
				}
				return createGitHubWorkflow(root)
			},
		},
	}
}

// Vite-specific rules

func viteRules() []Rule {
	return []Rule{
		{
			Name:        "Add Vite Dockerfile",
			Description: "Add production Dockerfile for Vite",
			Check: func(root string) bool {
				return fileExists(filepath.Join(root, "Dockerfile"))
			},
			Apply: func(root string, dryRun bool) error {
				if dryRun {
					return nil
				}
				return os.WriteFile(filepath.Join(root, "Dockerfile"), []byte(viteDockerfile), 0600)
			},
		},
		{
			Name:        "Add nginx config",
			Description: "Add nginx.conf for SPA routing",
			Check: func(root string) bool {
				return fileExists(filepath.Join(root, "nginx.conf"))
			},
			Apply: func(root string, dryRun bool) error {
				if dryRun {
					return nil
				}
				return os.WriteFile(filepath.Join(root, "nginx.conf"), []byte(nginxConfig), 0600)
			},
		},
	}
}

// Next.js-specific rules

func nextjsRules() []Rule {
	return []Rule{
		{
			Name:        "Add Next.js Dockerfile",
			Description: "Add production Dockerfile for Next.js",
			Check: func(root string) bool {
				return fileExists(filepath.Join(root, "Dockerfile"))
			},
			Apply: func(root string, dryRun bool) error {
				if dryRun {
					return nil
				}
				// Detect if static or SSR
				outputType := detect.DetectOutputType(root, detect.NextJS)
				if outputType == detect.Static {
					return os.WriteFile(filepath.Join(root, "Dockerfile"), []byte(nextStaticDockerfile), 0600)
				}
				return os.WriteFile(filepath.Join(root, "Dockerfile"), []byte(nextSSRDockerfile), 0600)
			},
		},
	}
}

// Helper functions

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func gitCommit(root, message string) error {
	// Stage all changes
	add := exec.Command("git", "add", "-A")
	add.Dir = root
	if err := add.Run(); err != nil {
		return err
	}

	// Commit
	// #nosec G204 - message is hardcoded in rules, not user input
	commit := exec.Command("git", "commit", "-m", fmt.Sprintf("[mvpbridge] %s", message))
	commit.Dir = root
	return commit.Run()
}

func createEnvExample(root string) error {
	envPath := filepath.Join(root, ".env")
	examplePath := filepath.Join(root, ".env.example")

	// If .env exists, extract keys
	if data, err := os.ReadFile(envPath); err == nil {
		lines := strings.Split(string(data), "\n")
		var example []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				example = append(example, line)
				continue
			}
			if idx := strings.Index(line, "="); idx > 0 {
				key := line[:idx]
				example = append(example, key+"=")
			}
		}
		return os.WriteFile(examplePath, []byte(strings.Join(example, "\n")+"\n"), 0600)
	}

	// Default template
	return os.WriteFile(examplePath, []byte("# Environment variables\n# Copy to .env and fill in values\n"), 0600)
}

func gitignoreComplete(root string) bool {
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	content := string(data)
	required := []string{"node_modules", ".env", "dist", ".next"}
	for _, r := range required {
		if !strings.Contains(content, r) {
			return false
		}
	}
	return true
}

func updateGitignore(root string) error {
	path := filepath.Join(root, ".gitignore")

	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}

	additions := []string{
		"node_modules/",
		".env",
		".env.local",
		"dist/",
		".next/",
		"out/",
		".mvpbridge/",
		"*.log",
	}

	var toAdd []string
	for _, entry := range additions {
		if !strings.Contains(existing, strings.TrimSuffix(entry, "/")) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	content := existing
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "\n# Added by mvpbridge\n"
	content += strings.Join(toAdd, "\n") + "\n"

	return os.WriteFile(path, []byte(content), 0600)
}

func createGitHubWorkflow(root string) error {
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "deploy.yml"), []byte(githubWorkflow), 0600)
}

// Templates

const viteDockerfile = `# Build stage
FROM node:20-alpine AS builder
WORKDIR /app

COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

# Production stage
FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/nginx.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
`

const nginxConfig = `server {
    listen 80;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;

    # SPA routing - serve index.html for all routes
    location / {
        try_files $uri $uri/ /index.html;
    }

    # Cache static assets
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2)$ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;

    # Gzip
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;
}
`

const nextStaticDockerfile = `# Build stage
FROM node:20-alpine AS builder
WORKDIR /app

COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

# Production stage
FROM nginx:alpine
COPY --from=builder /app/out /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
`

const nextSSRDockerfile = `# Build stage
FROM node:20-alpine AS builder
WORKDIR /app

COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

# Production stage
FROM node:20-alpine
WORKDIR /app

COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
COPY --from=builder /app/public ./public

ENV NODE_ENV=production
ENV PORT=3000
EXPOSE 3000

CMD ["node", "server.js"]
`

const githubWorkflow = `name: Deploy

on:
  push:
    branches: [main]

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'

      - name: Install dependencies
        run: npm ci

      - name: Build
        run: npm run build

      - name: Deploy to DigitalOcean
        uses: digitalocean/app_action@v1
        with:
          app_name: ${{ vars.DO_APP_NAME }}
          token: ${{ secrets.DIGITALOCEAN_TOKEN }}
`

const githubWorkflowAWS = `name: Deploy to AWS Amplify

on:
  push:
    branches: [main]

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'

      - name: Install dependencies
        run: npm ci

      - name: Build
        run: npm run build

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: ${{ vars.AWS_REGION || 'us-east-1' }}

      - name: Deploy to Amplify
        run: |
          # Amplify auto-deploys from GitHub
          # This workflow is for build validation
          echo "Build successful - Amplify will auto-deploy"
`
