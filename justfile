# PocketBase MCP Go — https://github.com/casey/just

set shell := ["bash", "-eu", "-o", "pipefail", "-c"]
set dotenv-load := true

binary := "pocketbase-mcp-go"
local_dir := ".local"
pb_version := env_var_or_default("PB_VERSION", "0.39.3")
pb_addr := env_var_or_default("PB_ADDR", "127.0.0.1:18090")
mcp_addr := env_var_or_default("MCP_HTTP_ADDR", ":8080")

default:
    @just --list --unsorted

# Build release binary to project root.
[group("build")]
build:
    go build -o {{binary}} .

# Remove build artifacts.
[group("build")]
clean:
    rm -f {{binary}}
    go clean -testcache

# Remove build artifacts and local PocketBase runtime (.local/).
[group("build")]
clean-all: clean
    rm -rf {{local_dir}}

# Format all Go source files.
[group("dev")]
fmt:
    gofmt -w .
    go fmt ./...

# Run go vet.
[group("dev")]
vet:
    go vet ./...

# Tidy module dependencies.
[group("dev")]
tidy:
    go mod tidy

# Format, vet, and run unit tests.
[group("dev")]
check: fmt vet test

# Run unit and mock integration tests.
[group("test")]
test:
    go test ./...

# Run integration tests against POCKETBASE_URL (existing instance).
[group("test")]
test-live:
    : "${POCKETBASE_URL:?set POCKETBASE_URL}"
    : "${POCKETBASE_ADMIN_EMAIL:?set POCKETBASE_ADMIN_EMAIL}"
    : "${POCKETBASE_ADMIN_PASSWORD:?set POCKETBASE_ADMIN_PASSWORD}"
    go test -tags=integration -count=1 -v -run 'TestLive' .

# Full live acceptance: download PocketBase, start temp instance, run tests.
[group("test")]
accept:
    ./scripts/run-live-acceptance.sh

# Download PocketBase binary to .local/ (for local dev).
[group("pocketbase")]
pb-download:
    ./scripts/run-live-acceptance.sh --download-only

# Start a local PocketBase instance on PB_ADDR (default 127.0.0.1:18090).
[group("pocketbase")]
pb-serve:
    #!/usr/bin/env bash
    set -euo pipefail
    pb_bin="{{justfile_directory()}}/{{local_dir}}/pocketbase"
    pb_data="{{justfile_directory()}}/{{local_dir}}/pb_data"
    admin_email="${POCKETBASE_ADMIN_EMAIL:-admin@example.com}"
    admin_password="${POCKETBASE_ADMIN_PASSWORD:-testpassword123}"
    if [[ ! -x "${pb_bin}" ]]; then
      just pb-download
    fi
    mkdir -p "${pb_data}"
    if curl -fsS "http://{{pb_addr}}/api/health" >/dev/null 2>&1; then
      echo "PocketBase already running at http://{{pb_addr}}"
      exit 0
    fi
    echo "Starting PocketBase on http://{{pb_addr}}"
    "${pb_bin}" serve --http="{{pb_addr}}" --dir="${pb_data}" &
    for _ in $(seq 1 60); do
      if curl -fsS "http://{{pb_addr}}/api/health" >/dev/null 2>&1; then
        "${pb_bin}" superuser upsert "${admin_email}" "${admin_password}" --dir="${pb_data}" >/dev/null 2>&1 || true
        echo "PocketBase ready: http://{{pb_addr}}"
        echo "Admin: ${admin_email}"
        wait
        exit 0
      fi
      sleep 0.5
    done
    echo "PocketBase failed to start" >&2
    exit 1

# Start MCP server (HTTP, default transport).
[group("run")]
run: run-http

# Start MCP server in HTTP mode.
[group("run")]
run-http: build
    : "${POCKETBASE_URL:?set POCKETBASE_URL}"
    ./{{binary}} -transport=http -http-addr="{{mcp_addr}}"

# Start MCP server in stdio mode (for MCP clients).
[group("run")]
run-stdio: build
    : "${POCKETBASE_URL:?set POCKETBASE_URL}"
    exec ./{{binary}} -transport=stdio

# Probe MCP HTTP /health (server must already be running).
[group("run")]
health url="http://127.0.0.1:8080":
    curl -fsS "{{url}}/health"
    @echo

# Build and print binary path (useful for MCP client config).
[group("run")]
path: build
    @echo "{{justfile_directory()}}/{{binary}}"
