# Contributing to Maestro

## Development Setup

```bash
# Clone the repo
git clone https://github.com/Mulder90/maestro.git
cd maestro

# Install dependencies
go mod download

# Build
go build ./cmd/maestro

# Install locally
go install ./cmd/maestro
```

## Running Tests

```bash
# All tests
go test ./...

# With verbose output
go test -v ./...

# With race detection
go test -race ./...

# With coverage
go test -cover ./...

# Specific package
go test ./internal/collector

# Specific test
go test -v -run TestComputeMetrics ./internal/collector
```

### Test Categories

```bash
# Unit tests for metrics computation
go test ./internal/collector -v -run ComputeMetrics

# Unit tests for output formatting
go test ./internal/collector -v -run Format

# Integration tests (end-to-end)
go test -v -run Integration
```

### Coverage Report

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Local Test Server

For testing without external dependencies:

```bash
# Start the test server
go run ./cmd/testserver &

# Run tests against it
maestro --config=examples/local/health-check.yaml --duration=5s

# Stop the server
pkill testserver
```

Test server endpoints:

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Returns `{"status":"ok"}` |
| `GET /status/{code}` | Returns specified HTTP status |
| `GET /delay/{ms}` | Delays response by N milliseconds |
| `POST /echo` | Echoes request body |
| `GET /random-delay?min=X&max=Y` | Random delay |
| `GET /fail-rate?rate=N` | Fails N% of requests |

## Project Structure

```
maestro/
├── cmd/
│   ├── maestro/          # CLI entry point
│   └── testserver/       # Local test server
├── internal/
│   ├── collector/        # Metrics collection and formatting
│   ├── config/           # YAML config parsing
│   ├── coordinator/      # Actor lifecycle management
│   ├── core/             # Interfaces and types
│   ├── http/             # HTTP workflow execution
│   ├── progress/         # Progress display
│   └── ratelimit/        # Rate limiting and phases
├── examples/             # Example configs
├── docs/                 # Documentation
└── integration_test.go   # End-to-end tests
```

## Code Style

- Follow standard Go conventions
- Run `go fmt` before committing
- Run `go vet` to catch issues

```bash
go fmt ./...
go vet ./...
```

## Making Changes

1. Create a branch for your changes
2. Write tests for new functionality
3. Ensure all tests pass: `go test ./...`
4. Ensure no race conditions: `go test -race ./...`
5. Submit a pull request

## Architecture Notes

See [ARCHITECTURE.md](ARCHITECTURE.md) for design details.

Key principles:
- **Pure functions** for computation (`ComputeMetrics`)
- **Separation of concerns** (collection, computation, formatting)
- **No shared mutable state** between actors
- **Context-based cancellation** throughout
