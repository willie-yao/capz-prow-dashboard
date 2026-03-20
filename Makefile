.PHONY: all build test lint clean fetch-data dev deploy help

# Default target
all: build

## ─── Go Backend ───────────────────────────────────────────────

# Build the data fetcher binary
build:
	cd backend && go build -o ../bin/fetcher ./cmd/fetcher/

# Run all Go tests
test:
	cd backend && go test ./... -count=1

# Run Go tests with verbose output
test-v:
	cd backend && go test ./... -count=1 -v

# Run Go linter (requires golangci-lint)
lint:
	cd backend && golangci-lint run ./...

# Format Go code
fmt:
	cd backend && gofmt -w .

# Tidy Go modules
tidy:
	cd backend && go mod tidy

## ─── Data Fetching ────────────────────────────────────────────

# Fetch fresh test data from GCS into frontend/public/data/
fetch-data: build
	./bin/fetcher -builds=15 -workers=5 -out=frontend/public/data -timeout=5m

# Fetch minimal data (3 builds per job, faster)
fetch-data-quick: build
	./bin/fetcher -builds=3 -workers=5 -out=frontend/public/data -timeout=3m

## ─── Frontend ─────────────────────────────────────────────────

# Install frontend dependencies
fe-install:
	cd frontend && npm ci

# Start the Vite dev server
dev: fe-install
	cd frontend && npm run dev

# Build the frontend for production
fe-build: fe-install
	cd frontend && npm run build

# TypeScript type check
fe-check:
	cd frontend && npx tsc --noEmit

## ─── Full Pipeline ────────────────────────────────────────────

# Build everything: Go binary + fetch data + frontend
dist: fetch-data fe-build

# Clean all build artifacts
clean:
	rm -rf bin/ frontend/dist frontend/public/data/dashboard.json frontend/public/data/jobs/

# Trigger GitHub Actions deploy workflow
deploy:
	gh workflow run deploy.yml

## ─── Help ─────────────────────────────────────────────────────

help:
	@echo "CAPZ Prow Dashboard — Make Targets"
	@echo ""
	@echo "  build           Build Go data fetcher binary"
	@echo "  test            Run Go tests"
	@echo "  test-v          Run Go tests (verbose)"
	@echo "  lint            Run golangci-lint"
	@echo "  fmt             Format Go code"
	@echo "  tidy            Tidy Go modules"
	@echo ""
	@echo "  fetch-data      Fetch fresh data from GCS (15 builds/job)"
	@echo "  fetch-data-quick  Fetch minimal data (3 builds/job)"
	@echo ""
	@echo "  fe-install      Install frontend npm dependencies"
	@echo "  dev             Start Vite dev server"
	@echo "  fe-build        Production build of frontend"
	@echo "  fe-check        TypeScript type check"
	@echo ""
	@echo "  dist            Full pipeline: build + fetch + frontend"
	@echo "  clean           Remove all build artifacts"
	@echo "  deploy          Trigger GitHub Actions deploy"
