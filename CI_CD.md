# CI/CD Documentation

This document describes the Continuous Integration and Continuous Deployment (CI/CD) setup for MVPBridge.

## Overview

MVPBridge uses GitHub Actions for automated testing, building, and releasing. The CI/CD pipeline ensures code quality, cross-platform compatibility, and streamlined releases.

## Workflows

### 1. CI Workflow (`.github/workflows/ci.yml`)

**Triggers:**
- Push to `main` or `develop` branches
- Pull requests to `main` or `develop` branches

**Jobs:**

#### Test Job
- **Platforms:** Ubuntu, macOS, Windows
- **Go Versions:** 1.21, 1.22
- **Steps:**
  1. Check out code
  2. Set up Go with caching
  3. Download and verify dependencies
  4. Run tests with race detection
  5. Generate coverage report
  6. Upload coverage to Codecov (Ubuntu + Go 1.22 only)

**Command:**
```bash
go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...
```

#### Build Job
- **Platforms:** Ubuntu, macOS, Windows
- **Steps:**
  1. Build binary for the platform
  2. Verify binary runs correctly (`mvpbridge --version`)

#### Lint Job
- **Platform:** Ubuntu
- **Tool:** golangci-lint
- **Steps:**
  1. Run comprehensive linting checks
  2. Check code style, security, and best practices

**Enabled Linters:**
- errcheck - Unchecked errors
- gosimple - Code simplification
- govet - Go vet analysis
- staticcheck - Advanced static analysis
- gosec - Security issues
- gocyclo - Cyclomatic complexity
- And more (see `.golangci.yml`)

#### Security Job
- **Platform:** Ubuntu
- **Tool:** Gosec
- **Steps:**
  1. Scan for security vulnerabilities
  2. Report potential security issues

### 2. Release Workflow (`.github/workflows/release.yml`)

**Triggers:**
- Push of version tags (e.g., `v0.1.0`, `v1.2.3`)

**Jobs:**

#### Release Job
- **Steps:**
  1. Run full test suite
  2. Build binaries for multiple platforms:
     - Linux (amd64, arm64)
     - macOS (Intel, Apple Silicon)
     - Windows (amd64)
  3. Generate SHA256 checksums
  4. Create GitHub Release with:
     - Release notes
     - All platform binaries
     - Checksums file
  5. Publish as non-draft release

**Build Platforms:**
```bash
GOOS=linux GOARCH=amd64    # Linux x86-64
GOOS=linux GOARCH=arm64    # Linux ARM64
GOOS=darwin GOARCH=amd64   # macOS Intel
GOOS=darwin GOARCH=arm64   # macOS Apple Silicon
GOOS=windows GOARCH=amd64  # Windows x86-64
```

#### Publish Docker Job
- **Steps:**
  1. Build multi-platform Docker image
  2. Push to GitHub Container Registry (ghcr.io)
  3. Tag with version and `latest`

**Docker Tags:**
- `ghcr.io/daryllundy/mvp-bridge:latest`
- `ghcr.io/daryllundy/mvp-bridge:0.1.0`
- `ghcr.io/daryllundy/mvp-bridge:0.1`
- `ghcr.io/daryllundy/mvp-bridge:0`

## Configuration Files

### `.golangci.yml`

Configures the golangci-lint linter with:
- 20+ enabled linters
- Custom settings for each linter
- Test file exclusions
- Complexity thresholds

**Key Settings:**
- Cyclomatic complexity: max 15
- Code duplication: min 100 lines
- Constants: min 3 occurrences

### `.github/dependabot.yml`

Automated dependency updates:
- **Go modules:** Weekly on Mondays
- **GitHub Actions:** Weekly on Mondays
- Maximum 5 open PRs per ecosystem
- Auto-labeled with `dependencies`

### `Dockerfile.cli`

Multi-stage Docker build:
1. **Builder stage:** Compile Go binary
2. **Runtime stage:** Minimal Alpine image with binary

**Features:**
- Small image size (Alpine-based)
- Includes git and CA certificates
- Statically compiled binary
- Non-root execution

## Running CI Checks Locally

### Prerequisites

Install required tools:

```bash
# macOS
brew install golangci-lint
go install github.com/securego/gosec/v2/cmd/gosec@latest

# Linux
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
go install github.com/securego/gosec/v2/cmd/gosec@latest
```

### Run All CI Checks

```bash
# Run full CI suite
make ci

# Or run individually
make fmt-check  # Check formatting
make lint       # Run linter
make test       # Run tests
make security   # Security scan
```

### Format Code

```bash
# Format all Go files
make fmt

# Verify formatting
make fmt-check
```

### Run Tests with Coverage

```bash
# All tests with coverage
go test ./... -cover

# Specific package
go test ./internal/detect -v -cover

# With race detection (like CI)
go test ./... -race -coverprofile=coverage.txt
```

### Run Linter

```bash
# Run all linters
make lint

# Run specific linter
golangci-lint run --disable-all --enable=errcheck

# Auto-fix issues
golangci-lint run --fix
```

### Security Scan

```bash
# Scan for security issues
make security

# Or directly
gosec ./...

# With JSON output
gosec -fmt=json -out=results.json ./...
```

## Creating a Release

### 1. Prepare Release

Ensure all changes are committed and tests pass:

```bash
# Run CI checks locally
make ci

# Ensure clean working directory
git status
```

### 2. Create Version Tag

```bash
# Create and push tag
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

### 3. Monitor Release Workflow

1. Go to GitHub Actions tab
2. Watch the "Release" workflow
3. Verify all jobs complete successfully

### 4. Verify Release

1. Check GitHub Releases page
2. Verify all binaries are attached
3. Verify checksums file
4. Test download and run a binary

```bash
# Download and test (example for macOS)
curl -L https://github.com/daryllundy/mvp-bridge/releases/download/v0.1.0/mvpbridge-darwin-arm64 -o mvpbridge
chmod +x mvpbridge
./mvpbridge --version
```

## Docker Image Usage

### Pull and Run

```bash
# Pull latest image
docker pull ghcr.io/daryllundy/mvp-bridge:latest

# Run command
docker run --rm ghcr.io/daryllundy/mvp-bridge:latest --version

# Interactive use
docker run -it --rm -v $(pwd):/workspace -w /workspace \
  ghcr.io/daryllundy/mvp-bridge:latest init
```

### Build Locally

```bash
# Build image
docker build -f Dockerfile.cli -t mvpbridge:local .

# Run
docker run --rm mvpbridge:local --version
```

## Coverage Reporting

### Codecov Integration

Coverage reports are automatically uploaded to Codecov on:
- Push to main
- Pull requests
- Only from Ubuntu + Go 1.22 job (to avoid duplicates)

**Setup:**
1. Sign up at [codecov.io](https://codecov.io)
2. Add repository
3. No token needed for public repos

### View Coverage

```bash
# Generate coverage report
go test ./... -coverprofile=coverage.out

# View in terminal
go tool cover -func=coverage.out

# View in browser
go tool cover -html=coverage.out
```

## Troubleshooting

### CI Failures

#### Tests Fail on Specific Platform

```bash
# Test locally with race detection
go test ./... -race

# Test specific platform (cross-compile)
GOOS=windows GOARCH=amd64 go test ./...
```

#### Linter Fails

```bash
# Run locally to see issues
make lint

# Auto-fix what's possible
golangci-lint run --fix

# See specific linter output
golangci-lint run --verbose
```

#### Build Fails

```bash
# Verify dependencies
go mod tidy
go mod verify

# Clean cache
go clean -cache -modcache -testcache
```

### Release Issues

#### Tag Already Exists

```bash
# Delete local tag
git tag -d v0.1.0

# Delete remote tag
git push origin :refs/tags/v0.1.0

# Recreate
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

#### Binary Build Fails

```bash
# Test cross-compilation locally
GOOS=linux GOARCH=amd64 go build -o mvpbridge-linux-amd64 ./main.go
GOOS=darwin GOARCH=arm64 go build -o mvpbridge-darwin-arm64 ./main.go
```

## Best Practices

### Before Pushing

1. ✅ Run `make ci` locally
2. ✅ Ensure all tests pass
3. ✅ Check for security issues
4. ✅ Update documentation
5. ✅ Write clear commit messages

### Pull Request Checklist

- [ ] All CI checks pass
- [ ] Tests added for new features
- [ ] Documentation updated
- [ ] No sensitive data in commits
- [ ] Follows code style guidelines

### Release Checklist

- [ ] Version number follows semver
- [ ] CHANGELOG updated (if exists)
- [ ] All tests pass
- [ ] Documentation reflects new version
- [ ] Breaking changes documented

## Performance

### CI Execution Times

**Typical durations:**
- Test job: 2-4 minutes per platform/version
- Build job: 1-2 minutes per platform
- Lint job: 1-2 minutes
- Security job: 1-2 minutes

**Total CI time:** ~5-8 minutes

**Release time:** ~10-15 minutes

### Optimization

**Caching:**
- Go modules are cached
- Build cache is used
- Docker layers are cached

**Parallelization:**
- Tests run on 6 platform/version combinations
- Builds run on 3 platforms
- All jobs run in parallel

## Monitoring

### GitHub Actions

View workflow runs:
```
https://github.com/daryllundy/mvp-bridge/actions
```

### Status Badges

Add to README.md:

```markdown
[![CI](https://github.com/daryllundy/mvp-bridge/workflows/CI/badge.svg)](https://github.com/daryllundy/mvp-bridge/actions)
[![codecov](https://codecov.io/gh/daryllundy/mvp-bridge/branch/main/graph/badge.svg)](https://codecov.io/gh/daryllundy/mvp-bridge)
```

## Security

### Secrets Used

- `GITHUB_TOKEN` - Automatically provided by GitHub
- `CODECOV_TOKEN` - Optional, for private repos

### Dependency Updates

Dependabot automatically:
- Checks for updates weekly
- Creates PRs for outdated dependencies
- Tests changes via CI before merge

## Future Enhancements

- [ ] Add integration tests
- [ ] Add performance benchmarks
- [ ] Add mutation testing
- [ ] Add canary releases
- [ ] Add automatic changelog generation
- [ ] Add semantic release automation

## Support

For CI/CD issues:
1. Check GitHub Actions logs
2. Review this documentation
3. Open an issue with CI/CD logs attached
4. Tag with `ci/cd` label
