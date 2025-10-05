.PHONY: all test fmt vet build clean coverage help

# Default target
all: fmt vet test

# Run all tests
test:
	@echo "Running tests..."
	@go test -v -race ./...

# Run tests with coverage
coverage:
	@echo "Running tests with coverage..."
	@go test -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Check for formatting issues
fmt-check:
	@echo "Checking code formatting..."
	@test -z "$$(gofmt -l .)" || (echo "The following files need formatting:" && gofmt -l . && exit 1)

# Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...

# Build the project
build:
	@echo "Building..."
	@go build ./...

# Tidy up dependencies
tidy:
	@echo "Tidying dependencies..."
	@go mod tidy

# Clean build artifacts and coverage files
clean:
	@echo "Cleaning..."
	@rm -f coverage.out coverage.html
	@go clean

# Run all checks (formatting, vetting, testing)
check: fmt-check vet test
	@echo "All checks passed!"

# Show help
help:
	@echo "Available targets:"
	@echo "  make all        - Format, vet, and test (default)"
	@echo "  make test       - Run all tests with race detection"
	@echo "  make coverage   - Run tests with coverage report"
	@echo "  make fmt        - Format all Go files"
	@echo "  make fmt-check  - Check if files need formatting (CI mode)"
	@echo "  make vet        - Run go vet"
	@echo "  make build      - Build the project"
	@echo "  make tidy       - Tidy up dependencies"
	@echo "  make clean      - Remove build artifacts"
	@echo "  make check      - Run all checks (format check, vet, test)"
	@echo "  make help       - Show this help message"
