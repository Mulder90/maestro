# BurstSmith

A minimal, idiomatic Go CLI load-testing tool using an actor-based model.

## Installation

```bash
go install ./cmd/burstsmith
```

Or build locally:

```bash
go build -o burstsmith ./cmd/burstsmith
```

## Quick Start

```bash
# Run a simple health check with 5 actors for 10 seconds
burstsmith --config=examples/simple/health-check.yaml --actors=5 --duration=10s
```

## Usage

```
burstsmith --config=<path> [--actors=N] [--duration=D]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | (required) | Path to YAML workflow config |
| `--actors` | 5 | Number of concurrent actors |
| `--duration` | 10s | Test duration (e.g., 30s, 1m, 5m) |

## Configuration

Workflows are defined in YAML:

```yaml
workflow:
  name: "My API Test"
  steps:
    - name: "get_users"
      method: GET
      url: "https://api.example.com/users"

    - name: "create_user"
      method: POST
      url: "https://api.example.com/users"
      headers:
        Content-Type: "application/json"
      body: '{"name": "test"}'
```

### Step Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Step identifier (shown in results) |
| `method` | Yes | HTTP method (GET, POST, PUT, DELETE, etc.) |
| `url` | Yes | Target URL |
| `headers` | No | Map of HTTP headers |
| `body` | No | Request body (string) |

## Load Profiles

For advanced load patterns, define a `loadProfile` with multiple phases:

```yaml
workflow:
  name: "API Load Test"
  steps:
    - name: "health"
      method: GET
      url: "https://httpbin.org/status/200"

loadProfile:
  phases:
    - name: "ramp_up"
      duration: 30s
      startActors: 1
      endActors: 50

    - name: "steady"
      duration: 2m
      actors: 50
      rps: 100

    - name: "ramp_down"
      duration: 15s
      startActors: 50
      endActors: 0
```

When `loadProfile` is present, the `--actors` and `--duration` flags are ignored.

### Phase Types

**Ramp phases** - Linearly scale actors between start and end values:
```yaml
- name: "ramp_up"
  duration: 30s
  startActors: 1
  endActors: 50
```

**Steady phases** - Fixed actor count with optional rate limiting:
```yaml
- name: "steady"
  duration: 2m
  actors: 50
  rps: 100  # requests per second (total across all actors)
```

### Phase Fields

| Field | Description |
|-------|-------------|
| `name` | Phase identifier (shown in output) |
| `duration` | Phase duration (e.g., 30s, 2m, 1h) |
| `actors` | Fixed actor count (for steady phases) |
| `startActors` | Starting actor count (for ramp phases) |
| `endActors` | Ending actor count (for ramp phases) |
| `rps` | Rate limit in requests per second (optional) |

## Examples

The `examples/` folder contains ready-to-run configurations:

### Simple
```bash
# Single GET request
burstsmith --config=examples/simple/health-check.yaml --actors=2 --duration=5s

# POST with JSON body
burstsmith --config=examples/simple/post-json.yaml --actors=2 --duration=5s

# Custom headers
burstsmith --config=examples/simple/with-headers.yaml --actors=2 --duration=5s
```

### Workflows
```bash
# CRUD operations flow
burstsmith --config=examples/workflows/crud-api.yaml --actors=3 --duration=10s

# Authentication flow
burstsmith --config=examples/workflows/auth-flow.yaml --actors=3 --duration=10s

# Multiple endpoints
burstsmith --config=examples/workflows/multi-endpoint.yaml --actors=3 --duration=10s
```

### Stress Tests
```bash
# Maximum throughput
burstsmith --config=examples/stress/rapid-fire.yaml --actors=10 --duration=30s

# Large payload
burstsmith --config=examples/stress/large-payload.yaml --actors=5 --duration=10s
```

### Load Profiles
```bash
# Ramp up, steady state, ramp down
burstsmith --config=examples/profiles/ramp-up-down.yaml

# Constant rate with RPS limiting
burstsmith --config=examples/profiles/steady-rate.yaml

# Burst/spike pattern
burstsmith --config=examples/profiles/burst.yaml
```

## Testing

Run all tests:

```bash
go test ./...
```

Run tests with verbose output:

```bash
go test -v ./...
```

Run tests with race detection:

```bash
go test -race ./...
```

Run tests with coverage:

```bash
go test -cover ./...
```

Generate coverage report:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Architecture

BurstSmith uses an actor-based concurrency model:

- **Actors**: Goroutines that execute workflows independently
- **Coordinator**: Spawns/terminates actors dynamically based on load profile
- **PhaseManager**: Tracks current phase and calculates target actor counts
- **RateLimiter**: Token bucket rate limiter for RPS control
- **Collector**: Aggregates metrics and prints summary
- **Reporter**: Thread-safe bridge for actors to report events

Each actor runs the complete workflow in a loop until stopped or the duration expires.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed design documentation.

## License

MIT
