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
	golangci-lint run

# Format the code
fmt:
	go fmt ./...

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
