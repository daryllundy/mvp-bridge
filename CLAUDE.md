# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

MVPBridge is a single-binary CLI tool written in Go that bridges the gap between MVP codebases and production-ready deployments. It inspects, normalizes, and deploys frontend projects (Vite, Next.js) to cloud platforms (DigitalOcean, AWS) with zero hosted dependencies.

**Philosophy**: Single binary, no daemon, no hosted dependencies, no accounts. Every change is a reversible git commit.

## Development Commands

```bash
# Build the binary
go build -o mvpbridge ./main.go

# Run tests
go test ./...

# Run specific package tests
go test ./internal/detect
go test ./internal/normalize
go test ./internal/deploy

# Run locally (without installing)
go run main.go <command>

# Install locally for testing
go install
```

## Project Architecture

### Package Structure

```
mvpbridge/
├── main.go                           # CLI entrypoint with Cobra commands
├── internal/
│   ├── detect/detect.go              # Framework & environment detection
│   ├── normalize/rules.go            # Normalization rules engine
│   ├── config/config.go              # Config loading/saving (.mvpbridge/config.yaml)
│   └── deploy/digitalocean.go        # DigitalOcean App Platform API client
```

### Core Workflow

1. **Detection Phase** (`internal/detect`): Scans project to identify framework (Vite/Next.js), package manager (npm/yarn/pnpm), Node version, build config, and deployment issues
2. **Normalization Phase** (`internal/normalize`): Applies atomic, reversible fixes as git commits (Dockerfile, .nvmrc, GitHub Actions, etc.)
3. **Deployment Phase** (`internal/deploy`): Deploys to cloud platforms via API (DigitalOcean App Platform, AWS planned)

### Key Architectural Patterns

**Rule-Based Normalization**: `normalize.Rule` struct contains:
- `Check()`: Boolean function to test if rule already satisfied
- `Apply()`: Function to apply the fix (creates/modifies files)
- Each rule creates one atomic git commit with `[mvpbridge]` prefix

**Detection Results**: `detect.Detection` struct aggregates all project info and issues. Used by both `inspect` command (read-only) and `normalize` command (applies fixes).

**Config Management**: `.mvpbridge/config.yaml` stores user choices (framework, target platform) and detected values (build command, output dir). Created by `init`, consumed by `normalize` and `deploy`.

## Command Implementation Flow

### `mvpbridge init`
1. Validates prerequisites (git repo, Node.js installed)
2. Calls `detect.DetectFramework()` to identify Vite/Next.js
3. Creates `.mvpbridge/config.yaml` with `config.NewFromDetection()`
4. User can override with `--framework` and `--target` flags

### `mvpbridge inspect`
1. Loads config from `.mvpbridge/config.yaml` (if exists)
2. Runs `detect.DetectAll()` to scan project
3. Displays formatted report with framework, Node version, package manager, and deployment issues
4. Read-only operation, no file modifications

### `mvpbridge normalize`
1. Loads config to determine framework
2. Creates `normalize.Normalizer` with universal + framework-specific rules
3. For each unsatisfied rule:
   - Applies fix (creates/modifies files)
   - Runs `git add -A && git commit -m "[mvpbridge] <description>"`
4. Supports `--dry-run` to preview changes without applying

### `mvpbridge deploy do`
1. Validates `DIGITALOCEAN_TOKEN` environment variable
2. Creates `deploy.DODeployer` client
3. Builds App Spec (static site vs service based on `config.IsStatic()`)
4. Calls DO API to create/update app
5. Returns app URL and dashboard link

## Important Implementation Details

### Framework Detection Logic
Detection is deterministic and checks in order:
1. Config files (most reliable): `next.config.*` → Next.js, `vite.config.*` → Vite
2. Fallback to `package.json` dependencies

### Output Type Detection (Next.js)
- Checks `next.config.js` for `output: "export"` → Static
- Otherwise → SSR
- Determines whether to use nginx+static or Node.js Dockerfile

### Dockerfile Templates
Located in `normalize/rules.go` as constants:
- `viteDockerfile`: Multi-stage build with nginx for static serving
- `nextStaticDockerfile`: Similar to Vite (uses `/app/out`)
- `nextSSRDockerfile`: Node.js runtime with standalone output

### Git Commit Strategy
Every normalization rule creates one atomic commit:
- Message format: `[mvpbridge] <rule.Description>`
- Makes all changes reversible via `git revert`
- Implemented in `normalize.gitCommit()`

## Environment Variables

- `DIGITALOCEAN_TOKEN`: Required for `deploy do` command
- `GITHUB_TOKEN`: Optional, for private repos (not yet implemented)

## Common Patterns

### Adding New Framework Support
1. Add constant to `detect.Framework` enum
2. Add detection logic to `detect.DetectFramework()`
3. Create framework-specific rules function (e.g., `svelteRules()`)
4. Add Dockerfile template constant
5. Update `normalize.New()` to include new rules

### Adding New Deployment Target
1. Create new file in `internal/deploy/` (e.g., `aws.go`)
2. Implement deployer struct with `Deploy()` method
3. Add case to `runDeploy()` in `main.go`
4. Update config validation in `config.Validate()`

### Adding New Normalization Rule
Create `normalize.Rule` struct with:
- `Name`: Display name for progress output
- `Description`: Git commit message
- `Check`: Returns `true` if already satisfied
- `Apply`: Creates/modifies files (respects `dryRun` param)

## Testing Notes

Tests are currently minimal (project in early development). When adding tests:
- Use temp directories for file operations (`t.TempDir()`)
- Mock git operations to avoid requiring git in test env
- Test detection logic with fixture directories containing different project types
