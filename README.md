# MVPBridge

[![CI](https://github.com/daryllundy/mvp-bridge/actions/workflows/ci.yml/badge.svg)](https://github.com/daryllundy/mvp-bridge/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/daryllundy/mvp-bridge)](https://goreportcard.com/report/github.com/daryllundy/mvp-bridge)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

MVPBridge is a single-binary CLI for taking a frontend repo from "works locally" to "deployable with reversible changes."

It inspects a project, applies opinionated normalization steps as atomic git commits, and deploys to a supported cloud target using your existing credentials.

```bash
go install github.com/daryllundy/mvp-bridge@latest

mvpbridge init
mvpbridge inspect
mvpbridge normalize
mvpbridge deploy do
```

## Project Status

- Active early-stage project (`v0.2.x` line of development)
- Current CLI commands: `init`, `inspect`, `normalize`, `deploy [do|aws|gcp|azure|local]`
- Framework support today: Vite and Next.js
- Primary deploy path: DigitalOcean App Platform
- Additional supported targets: AWS Amplify, Google Cloud Run, Azure Container Apps, local Docker Compose workspace generation
- Current test suite passes with `go test ./...`

## Why Builders May Care

- Reversible by default: each normalization step becomes its own `[mvpbridge] ...` git commit
- Uses the repo and cloud credentials you already have
- No hosted control plane, daemon, or account system
- Useful when a side project or MVP needs to become deployable without introducing a lot of tooling

## What Happens When You Run It

1. `init` detects the framework and creates `.mvpbridge/config.yaml`
2. `inspect` reports deployment readiness, build details, and obvious gaps
3. `normalize` adds the missing deployment scaffolding as atomic commits
4. `deploy <target>` pushes the normalized repo to a supported cloud target

## Quick Start

### Install

```bash
go install github.com/daryllundy/mvp-bridge@latest
```

### Run the default workflow

```bash
mvpbridge init
mvpbridge inspect
mvpbridge normalize
export DIGITALOCEAN_TOKEN=your_token_here
mvpbridge deploy do
```

DigitalOcean is the clearest default path right now. AWS, GCP, Azure, and a local Docker Compose workspace target are also supported if you want to use an existing cloud setup or generate a local deployment workspace.

## Current Capabilities

### Detection

MVPBridge currently detects:

- Framework: Vite or Next.js
- Package manager: npm, yarn, or pnpm
- Node version from `.nvmrc` or `package.json`
- Build command from `package.json`
- Output type: static or SSR

Framework detection prefers config files first:

- `next.config.js|mjs|ts` -> Next.js
- `vite.config.js|ts|mjs` -> Vite

### Normalization

`mvpbridge normalize` applies opinionated fixes only when they are missing.

Current rules include:

- Add `.nvmrc` pinned to Node `20`
- Create `.env.example`
- Update `.gitignore` with standard project entries
- Add `.github/workflows/deploy.yml`
- Add a production `Dockerfile`
- Add `nginx.conf` for Vite projects

Each applied rule is committed separately so you can inspect or revert it with normal git workflows.

### Deployment Targets

Primary path:

- DigitalOcean App Platform via `mvpbridge deploy do`

Also supported:

- AWS Amplify via `mvpbridge deploy aws`
- Google Cloud Run via `mvpbridge deploy gcp`
- Azure Container Apps via `mvpbridge deploy azure`
- Local Docker Compose workspace generation via `mvpbridge deploy local`

## Example Config

`mvpbridge init` creates `.mvpbridge/config.yaml` based on detection results.

```yaml
version: 1
framework: vite
target: do
detected:
  package_manager: npm
  build_command: npm run build
  output_dir: dist
  node_version: "20"
  output_type: static
deploy:
  app_name: my-app
  region: nyc
```

The exact detected values depend on the repo. Additional deploy settings such as `project_id`, `subscription_id`, `resource_group`, and `environment` are used for GCP and Azure flows when needed.

## Installation

### Go Install

```bash
go install github.com/daryllundy/mvp-bridge@latest
```

### Build From Source

```bash
git clone https://github.com/daryllundy/mvp-bridge
cd mvp-bridge
go build -o mvpbridge ./main.go
```

### Run Tests

```bash
go test ./...
```

## Usage

### `mvpbridge init`

Initializes MVPBridge in the current repo:

```bash
mvpbridge init
```

Optional flags:

- `--target`, `-t`: `do`, `aws`, `gcp`, `azure`, `local`
- `--framework`, `-f`: `vite`, `nextjs`

If no target is provided, MVPBridge defaults to `do`.

### `mvpbridge inspect`

Runs read-only detection and prints a deployment-readiness report:

```bash
mvpbridge inspect
```

Typical output includes:

- framework
- package manager
- pinned Node version
- build command and output directory
- output type
- missing deployment artifacts

### `mvpbridge normalize`

Applies missing normalization rules and creates git commits:

```bash
mvpbridge normalize
```

Useful flags:

- `--dry-run`: preview the steps without editing files
- `--yes`, `-y`: skip confirmation prompts

### `mvpbridge deploy`

Deploy to a supported target:

```bash
mvpbridge deploy do
mvpbridge deploy aws
mvpbridge deploy gcp
mvpbridge deploy azure
mvpbridge deploy local
```

## Environment Variables

### DigitalOcean

- `DIGITALOCEAN_TOKEN`

### AWS Amplify

- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`
- `GITHUB_TOKEN`
- `AWS_REGION` optional, defaults to `us-east-1`

### Google Cloud Run

- `GOOGLE_APPLICATION_CREDENTIALS`
- `GCP_PROJECT_ID`

Config default region if unset: `us-central1`

### Azure Container Apps

- Azure CLI login via `az login`

Config default region if unset: `eastus`

For provider-specific setup details:

- [AWS deployment guide](./docs/AWS_DEPLOYMENT.md)
- [GCP deployment guide](./docs/GCP_DEPLOYMENT.md)
- [Azure deployment guide](./docs/AZURE_DEPLOYMENT.md)
- [CI/CD guide](./docs/CI_CD.md)

## Philosophy

MVPBridge is intentionally opinionated:

- single binary
- no daemon
- no hosted dependency for the tool itself
- repo-first workflow
- git-based reversibility over hidden automation

## Current Limitations

- Framework support is currently limited to Vite and Next.js
- Normalization is intentionally opinionated, not a general-purpose scaffolding engine
- The platform matrix is broader than the level of polish on every deploy path
- The tool assumes a git-based workflow and is most useful when your repo is already buildable locally

## Readability Suggestions For Future README Iterations

- Add one short real-world walkthrough using a sample Vite repo once the command outputs settle
- Include a compact architecture diagram only if it helps explain detection -> normalization -> deploy
- Add a small comparison table against "manual setup" only if it stays factual and restrained
- Keep the front page focused on the default path and move cloud-specific depth into `docs/`
