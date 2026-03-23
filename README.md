# MVPBridge

[![CI](https://github.com/daryllundy/mvp-bridge/actions/workflows/ci.yml/badge.svg)](https://github.com/daryllundy/mvp-bridge/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/daryllundy/mvp-bridge)](https://goreportcard.com/report/github.com/daryllundy/mvp-bridge)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Bridge your MVP to production.**

MVPBridge is a single-binary CLI tool that inspects, normalizes, and deploys your frontend projects. No hosted dependencies. No daemons. No accounts.

```
Your MVP works locally.
MVPBridge makes it work everywhere else.
```

## Project Status

- `v0.x` active development
- Supports Vite and Next.js (static + SSR detection paths)
- Supports deployment to DigitalOcean App Platform and AWS Amplify
- Release artifacts are published on GitHub Releases

## Quick Start

```bash
# Install:
go install github.com/daryllundy/mvp-bridge@latest

# In your project directory
mvpbridge init
mvpbridge inspect
mvpbridge normalize
mvpbridge deploy do
```

**Time to first deployment: ~10 minutes**

## What It Does

| Command | Action |
|---------|--------|
| `init` | Detects your framework, creates config |
| `inspect` | Analyzes repo, reports what needs fixing |
| `normalize` | Adds Dockerfile, CI/CD, pins versions |
| `deploy do` | Ships to DigitalOcean App Platform |
| `deploy aws` | Ships to AWS Amplify |

## Supported Frameworks

- ✅ **Vite + React** (primary)
- ✅ **Next.js** (static export + SSR)

## Supported Platforms

- ✅ **DigitalOcean App Platform**
- ✅ **AWS Amplify**

## Philosophy

MVPBridge is deliberately simple:

- **Single binary** — No runtime, no dependencies
- **No daemon** — Runs when you need it, exits cleanly
- **No accounts** — Uses your existing GitHub + cloud credentials
- **Opinionated** — Fewer choices, faster results
- **Reversible** — Every change is a git commit you can revert

## Installation

### From Source

```bash
git clone https://github.com/daryllundy/mvp-bridge
cd mvp-bridge
go build -o mvpbridge ./main.go
```

### Go Install

```bash
go install github.com/daryllundy/mvp-bridge@latest
```

### Releases

Download from [GitHub Releases](https://github.com/daryllundy/mvp-bridge/releases).

## Usage

### Initialize

```bash
mvpbridge init
```

Creates `.mvpbridge/config.yaml` with detected settings:

```yaml
version: 1
framework: vite
target: digitalocean
detected:
  package_manager: npm
  build_command: npm run build
  output_dir: dist
```

### Inspect

```bash
mvpbridge inspect
```

Shows deployment readiness:

```
╭─────────────────────────────────────────────────╮
│  MVPBridge Inspection Report                    │
├─────────────────────────────────────────────────┤
│  Framework:     Vite + React                    │
│  Node:          20.11.0 (pinned)                │
│  Package Mgr:   npm                             │
│  Build:         npm run build → dist/           │
├─────────────────────────────────────────────────┤
│  Deployment Readiness: 2 issues found           │
│                                                 │
│  ✗ Missing Dockerfile                           │
│  ✗ No GitHub Actions workflow                   │
│                                                 │
│  Run `mvpbridge normalize` to fix these.        │
╰─────────────────────────────────────────────────╯
```

### Normalize

```bash
mvpbridge normalize
```

Applies fixes as atomic git commits:

```
[1/5] Adding .nvmrc
      → Committed: [mvpbridge] Pin Node version to 20

[2/5] Adding Dockerfile
      → Committed: [mvpbridge] Add production Dockerfile

[3/5] Adding nginx.conf
      → Committed: [mvpbridge] Add nginx.conf for SPA routing
```

Use `--dry-run` to preview changes without applying.

### Deploy

#### To DigitalOcean:

```bash
export DIGITALOCEAN_TOKEN=your_token_here
mvpbridge deploy do
```

#### To AWS Amplify:

```bash
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export GITHUB_TOKEN=your_github_token
mvpbridge deploy aws
```

Creates/updates an app and triggers deployment:

```
Deploying to DigitalOcean...

[1/4] Validating credentials... ✓
[2/4] Creating app spec... ✓
[3/4] Configuring secrets... ✓
[4/4] Triggering deployment... ✓

Deployment started!
  App URL: https://your-app-xxxxx.ondigitalocean.app
  Dashboard: https://cloud.digitalocean.com/apps/xxxxx
```

For detailed AWS setup instructions, see [AWS_DEPLOYMENT.md](./AWS_DEPLOYMENT.md)

## Environment Variables

| Variable | Required For | Description |
|----------|--------------|-------------|
| `DIGITALOCEAN_TOKEN` | DO deploy | API token from DO dashboard |
| `AWS_ACCESS_KEY_ID` | AWS deploy | AWS access key |
| `AWS_SECRET_ACCESS_KEY` | AWS deploy | AWS secret key |
| `GITHUB_TOKEN` | AWS deploy | GitHub personal access token |
| `AWS_REGION` | AWS deploy (optional) | AWS region (defaults to us-east-1) |

## How It Works

### Detection

MVPBridge identifies your framework by checking for config files:

- `vite.config.js/ts` → Vite
- `next.config.js/mjs/ts` → Next.js

It also detects:
- Package manager (npm/yarn/pnpm)
- Node version (from `.nvmrc` or `package.json`)
- Output type (static vs SSR)

### Normalization

Each fix is a separate git commit prefixed with `[mvpbridge]`:

1. **Node version pinning** — Creates `.nvmrc` and updates `package.json`
2. **Dockerfile** — Adds multi-stage build optimized for your framework
3. **nginx.conf** — For static sites, handles SPA routing
4. **.env.example** — Documents required env vars
5. **GitHub Actions** — Adds deployment workflow

### Deployment

For DigitalOcean:
1. Generates App Spec from your config
2. Creates or updates the App via API
3. Sets environment variables as secrets
4. Triggers deployment from your GitHub repo

## FAQ

**Why Go?**

Single static binary. No Node runtime, no Python deps. Fast startup. Serious infra tools (Terraform, Docker CLI) use Go.

**Why not just use Vercel/Netlify?**

You should! But some teams need:
- Self-hosted infrastructure
- Specific cloud providers
- More control over the deployment process

**Can I customize the templates?**

Not yet. Opinionated defaults first, customization later.

**Does this work with monorepos?**

Not yet. Single-app repos only for v1.

## Testing

MVPBridge includes tests across detection, deploy, config, and normalization packages:

```bash
# Run all tests
go test ./...

# Run with verbose output
go test ./... -v

# Run with coverage
go test ./... -cover

# Run specific package
go test ./internal/deploy -v
```

See [TESTING.md](./TESTING.md) for detailed testing documentation.

## Contributing

1. Fork the repo
2. Create a feature branch
3. Make your changes
4. Write tests: See [TESTING.md](./TESTING.md)
5. Run tests: `go test ./...`
6. Submit a PR

## License

MIT
