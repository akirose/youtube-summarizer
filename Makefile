.PHONY: run build test clean setup

# Default target
all: clean build

# Setup project
setup:
	@echo "Setting up project..."
	@cd backend && go mod tidy
	@if [ ! -f ./backend/.env ]; then \
		cp ./backend/.env.example ./backend/.env; \
		echo "Created .env file. Please edit backend/.env to add your API keys."; \
	fi

# Run the application
run: setup
	@echo "Starting YouTube Video Summarizer..."
	@cd backend && go run main.go

# Build the application
build: setup
	@echo "Building YouTube Video Summarizer..."
	@mkdir -p dist/backend
	@cd backend && GOOS=${GOOS} GOARCH=${GOARCH} go build -o ../dist/backend/
	@cp -r backend/.env.example dist/backend/
	@cp -r frontend dist/

# Run tests
test:
	@echo "Running tests..."
	@cd backend && go test ./...

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf dist
	@cd backend && go clean

# Help target
help:
	@echo "Available targets:"
	@echo "  make setup  - Setup the project (install dependencies, create .env file)"
	@echo "  make run    - Run the application (default)"
	@echo "  make build  - Build the application"
	@echo "  make test   - Run tests"
	@echo "  make clean  - Clean build artifacts"
	@echo "  make help   - Show this help message"
