# OmniLLM Makefile
# Wraps common development and build operations.
#
# Windows Note: If 'make' is not in your PATH, add it:
#   set PATH=C:\Program Files (x86)\GnuWin32\bin;%PATH%
# Or install via: winget install GnuWin32.Make

# ── Configuration ─────────────────────────────────────────────────────────────

SERVER_PORT   ?= 5000
FRONTEND_PORT ?= 5080
HOST          ?= 127.0.0.1
REBUILD       ?=

GO            := go
BUN           := bun
DEPS          := .build/.bun-install.stamp

INSTALL_DIR   := $(HOME)/.local/bin
BUILD_DIR     := .build/bin

ifeq ($(OS),Windows_NT)
  EXE         := .exe
  INSTALL_DIR := $(USERPROFILE)/.local/bin
  PRINT_BLANK := @powershell -NoProfile -Command "Write-Host ''"
  define MKDIR_TARGET
	if not exist "$(1)" mkdir "$(1)"
  endef
else
  EXE         :=
  PRINT_BLANK := @printf '\n'
  define MKDIR_TARGET
	mkdir -p "$(1)"
  endef
endif

OMNILLM_BIN   := $(INSTALL_DIR)/omnillm$(EXE)
OMNIPROXY_BIN := $(INSTALL_DIR)/omniproxy$(EXE)

# ── Phony targets ─────────────────────────────────────────────────────────────

.PHONY: all install deps build build-go build-frontend build-all \
        start stop restart restart-rebuild status logs logs-follow \
        dev dev-frontend \
        test test-frontend lint typecheck \
        release-patch release-minor release-major \
        docker-build docker-run \
        help

# ── Default ───────────────────────────────────────────────────────────────────

all: help

# ── Install ───────────────────────────────────────────────────────────────────

## install: Build all Go binaries and install to ~/.local/bin
install:
	$(call MKDIR_TARGET,$(INSTALL_DIR))
	$(GO) build -o "$(OMNILLM_BIN)" .
	$(GO) build -o "$(OMNIPROXY_BIN)" ./cmd/omniproxy
	@echo Installed omnillm$(EXE) to $(OMNILLM_BIN)
	@echo Installed omniproxy$(EXE) to $(OMNIPROXY_BIN)

## deps: Install Node.js dependencies with Bun
deps: $(DEPS)

$(DEPS): bun.lock package.json
	$(call MKDIR_TARGET,$(BUILD_DIR))
	$(BUN) install
	@touch "$(DEPS)"

## build: Build the Go backend binary and install to ~/.local/bin
build: build-go

## build-go: Compile the Go backend and install to ~/.local/bin
build-go:
	$(call MKDIR_TARGET,$(INSTALL_DIR))
	$(GO) build -o "$(OMNILLM_BIN)" .
	@echo Built omnillm$(EXE) to $(OMNILLM_BIN)

## build-frontend: Build the frontend assets (outputs to pages/)
build-frontend: $(DEPS)
	$(BUN) run build

## build-all: Build both the Go backend and the frontend assets
build-all: build-go build-frontend

# ── Dev / Start ───────────────────────────────────────────────────────────────

## start: Build the Go backend and start all services in the background
start: $(DEPS)
	$(BUN) run omni start \
	  --server-port $(SERVER_PORT) \
	  --frontend-port $(FRONTEND_PORT) \
	  --host $(HOST)

## stop: Stop all background services
stop: $(DEPS)
	$(BUN) run omni stop

## restart: Restart background services (no rebuild)
restart: $(DEPS)
	$(BUN) run omni restart $(REBUILD) \
	  --server-port $(SERVER_PORT) \
	  --frontend-port $(FRONTEND_PORT) \
	  --host $(HOST)

## restart-rebuild: Rebuild everything and restart background services
restart-rebuild: $(DEPS)
	$(BUN) run omni restart --rebuild \
	  --server-port $(SERVER_PORT) \
	  --frontend-port $(FRONTEND_PORT) \
	  --host $(HOST)

## status: Show running service status and ports
status: $(DEPS)
	$(BUN) run omni status

## logs: Print the last 50 lines of service logs
logs: $(DEPS)
	$(BUN) run omni logs

## logs-follow: Stream service logs in real time
logs-follow: $(DEPS)
	$(BUN) run omni logs --follow

## dev: Start both backend and frontend in the foreground (Ctrl+C to stop)
dev: $(DEPS)
	$(BUN) run dev \
	  --server-port $(SERVER_PORT) \
	  --frontend-port $(FRONTEND_PORT) \
	  --host $(HOST)

## dev-frontend: Start only the Vite frontend dev server
dev-frontend: $(DEPS)
	$(BUN) run dev:frontend

# ── Test ──────────────────────────────────────────────────────────────────────

## test: Run the full test suite
test: $(DEPS)
	$(BUN) run test

## test-frontend: Run frontend tests only
test-frontend: $(DEPS)
	$(BUN) run test:frontend

# ── Code Quality ──────────────────────────────────────────────────────────────

## lint: Run ESLint on changed files
lint: $(DEPS)
	$(BUN) run lint

## typecheck: Run TypeScript type checking on the frontend
typecheck: $(DEPS)
	$(BUN) run typecheck

# ── Release ───────────────────────────────────────────────────────────────────

## release-patch: Bump patch version, commit, tag, and push
release-patch: $(DEPS)
	$(BUN) run scripts/release.ts patch

## release-minor: Bump minor version, commit, tag, and push
release-minor: $(DEPS)
	$(BUN) run scripts/release.ts minor

## release-major: Bump major version, commit, tag, and push
release-major: $(DEPS)
	$(BUN) run scripts/release.ts major

# ── Docker ────────────────────────────────────────────────────────────────────

## docker-build: Build the Docker image tagged as omnillm
docker-build:
	docker build -t omnillm .

## docker-run: Run the Docker image on port 5002
docker-run:
	docker run -p $(SERVER_PORT):5002 omnillm

# ── Help ──────────────────────────────────────────────────────────────────────

## help: List all available targets with descriptions
help:
	@echo OmniLLM Development Tasks
	@echo Usage: make [target] [VARIABLE=value ...]
	$(PRINT_BLANK)
	@echo VARIABLES:
	@echo   SERVER_PORT=5000       Backend API port - default 5000
	@echo   FRONTEND_PORT=5080     Frontend dev server port - default 5080
	@echo   HOST=127.0.0.1         Bind address - default 127.0.0.1
	@echo   REBUILD=--rebuild      Add --rebuild flag to restart target
	$(PRINT_BLANK)
	@echo QUICK START:
	@echo   start                Build the Go backend and start all services in the background
	@echo   stop                 Stop all background services
	@echo   dev                  Start both backend and frontend in the foreground - Ctrl+C to stop
	@echo   status               Show running service status and ports
	$(PRINT_BLANK)
	@echo BUILD:
	@echo   deps                 Install Node.js dependencies with Bun
	@echo   install              Build all Go binaries and install to ~/.local/bin
	@echo   build                Build the Go backend binary and install to ~/.local/bin
	@echo   build-go             Compile the Go backend and install to ~/.local/bin
	@echo   build-frontend       Build the frontend assets - outputs to pages/
	@echo   build-all            Build both the Go backend and the frontend assets
	$(PRINT_BLANK)
	@echo DEVELOPMENT:
	@echo   dev-frontend         Start only the Vite frontend dev server
	@echo   restart              Restart background services - no rebuild
	@echo   restart-rebuild      Rebuild everything and restart background services
	$(PRINT_BLANK)
	@echo MONITORING:
	@echo   logs                 Print the last 50 lines of service logs
	@echo   logs-follow          Stream service logs in real time
	$(PRINT_BLANK)
	@echo TESTING and QUALITY:
	@echo   test                 Run the full test suite
	@echo   test-frontend        Run frontend tests only
	@echo   lint                 Run ESLint on changed files
	@echo   typecheck            Run TypeScript type checking on the frontend
	$(PRINT_BLANK)
	@echo RELEASE:
	@echo   release-patch        Bump patch version, commit, tag, and push
	@echo   release-minor        Bump minor version, commit, tag, and push
	@echo   release-major        Bump major version, commit, tag, and push
	$(PRINT_BLANK)
	@echo DOCKER:
	@echo   docker-build         Build the Docker image tagged as omnillm
	@echo   docker-run           Run the Docker image on port 5000
	$(PRINT_BLANK)
	@echo EXAMPLES:
	@echo   make deps
	@echo   make dev
	@echo   make start SERVER_PORT=5000 FRONTEND_PORT=5080
	@echo   make restart REBUILD=--rebuild
	@echo   make logs-follow
