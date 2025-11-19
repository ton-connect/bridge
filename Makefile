GIT_REVISION=`git rev-parse --short HEAD`
BRIDGE_VERSION=`git describe --tags --abbrev=0`
LDFLAGS=-ldflags "-X github.com/ton-connect/bridge/internal.GitRevision=${GIT_REVISION} -X github.com/ton-connect/bridge/internal.BridgeVersion=${BRIDGE_VERSION}"
GOFMT_FILES?=$$(find . -name '*.go' | grep -v vendor | grep -v yacc | grep -v .git)

.PHONY: all imports fmt test test-unit test-bench test-bridge-sdk run stop clean logs status help

STORAGE ?= memory
DOCKER_COMPOSE_FILE = docker/docker-compose.memory.yml

# Set compose file based on storage type
ifeq ($(STORAGE),memory)
    DOCKER_COMPOSE_FILE = docker/docker-compose.memory.yml
else ifeq ($(STORAGE),postgres)
    DOCKER_COMPOSE_FILE = docker/docker-compose.postgres.yml
endif

all: imports fmt test

build:
	go build -mod=mod ${LDFLAGS} -o bridge ./cmd/bridge

build3:
	go build -mod=mod ${LDFLAGS} -o bridge3 ./cmd/bridge3

fmt:
	gofmt -w $(GOFMT_FILES)

fmtcheck:
	@sh -c "'$(CURDIR)/scripts/gofmtcheck.sh'"

lint:
	golangci-lint run --timeout=10m --color=always

test: test-unit test-bench

test-unit:
	go test $$(go list ./... | grep -v vendor | grep -v test) -race -coverprofile cover.out

test-bench:
	go test -race -count 10 -timeout 15s -bench=BenchmarkConnectionCache -benchmem ./internal/v1/handler

test-gointegration:
	go test -v -p 10 -v -run TestBridge ./test/gointegration

test-bridge-sdk:
	@./scripts/test-bridge-sdk.sh

run:
	@echo "Starting bridge environment with $(STORAGE) storage using $(DOCKER_COMPOSE_FILE)..."
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose -f $(DOCKER_COMPOSE_FILE) up --build -d; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose -f $(DOCKER_COMPOSE_FILE) up --build -d; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		echo "Please install Docker Desktop from https://www.docker.com/products/docker-desktop"; \
		exit 1; \
	fi
	@echo "Environment started! Access the bridge at http://localhost:8081"
	@echo "Use 'make logs' to view logs, 'make stop' to stop services"

stop:
	@echo "Stopping bridge environment using $(DOCKER_COMPOSE_FILE)..."
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose -f $(DOCKER_COMPOSE_FILE) down; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose -f $(DOCKER_COMPOSE_FILE) down; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		exit 1; \
	fi

clean:
	@echo "Cleaning up bridge environment and volumes using $(DOCKER_COMPOSE_FILE)..."
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose -f $(DOCKER_COMPOSE_FILE) down -v --rmi local; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose -f $(DOCKER_COMPOSE_FILE) down -v --rmi local; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		exit 1; \
	fi
	@if command -v docker >/dev/null 2>&1; then \
		docker system prune -f; \
	fi

logs:
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose -f $(DOCKER_COMPOSE_FILE) logs -f; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose -f $(DOCKER_COMPOSE_FILE) logs -f; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		exit 1; \
	fi

status:
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose -f $(DOCKER_COMPOSE_FILE) ps; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose -f $(DOCKER_COMPOSE_FILE) ps; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		exit 1; \
	fi

help:
	@echo "TON Connect Bridge - Available Commands"
	@echo "======================================"
	@echo ""
	@echo "Development Commands:"
	@echo "  all           - Run imports, format, and test (default target)"
	@echo "  build         - Build the bridge binary"
	@echo "  fmt           - Format Go source files"
	@echo "  fmtcheck      - Check Go source file formatting"
	@echo "  lint          - Run golangci-lint"
	@echo "  test          - Run unit tests with race detection and coverage"
	@echo "  test-unit     - Run unit tests only (same as test)"
	@echo "  test-bench    - Run ConnectionCache benchmark tests"
	@echo "  test-bridge-sdk - Run bridge SDK integration tests"
	@echo ""
	@echo "Docker Environment Commands:"
	@echo "  run           - Start bridge environment (use STORAGE=memory|postgres)"
	@echo "  stop          - Stop bridge environment"
	@echo "  clean         - Stop environment and remove volumes/images"
	@echo "  logs          - Follow logs from running containers"
	@echo "  status        - Show status of running containers"
	@echo ""
	@echo "Storage Options:"
	@echo "  STORAGE=memory   - Use in-memory storage (default, no persistence)"
	@echo "  STORAGE=postgres - Use PostgreSQL storage (persistent)"
	@echo ""
	@echo "Usage Examples:"
	@echo "  make run                    # Start with memory storage"
	@echo "  make STORAGE=postgres run   # Start with PostgreSQL storage"
	@echo "  make test                   # Run all unit tests"
	@echo "  make test-bench             # Run ConnectionCache benchmarks"
	@echo "  make clean                  # Clean up everything"
	@echo ""
	@echo "Bridge will be available at: http://localhost:8081"