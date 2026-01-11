.PHONY: build test clean install run-tests lint help

# Build the binary
build:
	go build -o mvpbridge ./main.go

# Run all tests
test:
	go test ./... -v

# Run tests with coverage
test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out

# Clean build artifacts
clean:
	rm -f mvpbridge
	rm -f coverage.out

# Install the binary to $GOPATH/bin
install:
	go install

# Run tests for a specific package
test-detect:
	go test ./internal/detect -v

test-config:
	go test ./internal/config -v

test-normalize:
	go test ./internal/normalize -v

test-deploy:
	go test ./internal/deploy -v

# Lint the code
lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Run: brew install golangci-lint" && exit 1)
	golangci-lint run

# Format the code
fmt:
	go fmt ./...

# Check formatting
fmt-check:
	@test -z $$(gofmt -l .) || (echo "Code not formatted. Run 'make fmt'" && exit 1)

# Run CI checks locally
ci: fmt-check lint test
	@echo "✅ All CI checks passed"

# Security scan
security:
	@which gosec > /dev/null || (echo "gosec not installed. Run: go install github.com/securego/gosec/v2/cmd/gosec@latest" && exit 1)
	gosec ./...

# Run the binary (for quick testing)
run:
	go run main.go

# Display help
help:
	@echo "Available targets:"
	@echo "  build          - Build the mvpbridge binary"
	@echo "  test           - Run all tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  clean          - Remove build artifacts"
	@echo "  install        - Install binary to GOPATH/bin"
	@echo "  lint           - Run linter"
	@echo "  fmt            - Format code"
	@echo "  run            - Run the binary"
	@echo "  help           - Display this help message"
