# Maestro

An idiomatic Go CLI load-testing tool using a goroutiner actor-based model and YAML-configured workflows.

## Installation

```bash
go install ./cmd/maestro
```

Or build locally:

```bash
go build -o maestro ./cmd/maestro
```

## Quick Start

```bash
# Run a simple health check with 5 actors for 10 seconds
maestro --config=examples/simple/health-check.yaml --actors=5 --duration=10s
```

## Usage

```
maestro --config=<path> [options]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | (required) | Path to YAML workflow config |
| `--actors` | 5 | Number of concurrent actors |
| `--duration` | 10s | Test duration (e.g., 30s, 1m, 5m) |
| `--output` | text | Output format: `text` or `json` |
| `--quiet` | false | Suppress progress output during test |
| `--verbose` | false | Enable debug output (request/response logging) |
| `--max-iterations` | 0 | Max iterations per actor (0 = unlimited) |
| `--warmup` | 0 | Warmup iterations before collecting metrics (per-actor) |

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Test passed (all thresholds met) |
| 1 | Threshold failed |
| 2 | Error (config error, etc.) |

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

## Thresholds

Define pass/fail criteria for CI/CD integration:

```yaml
workflow:
  name: "API Test"
  steps:
    - name: "health"
      method: GET
      url: "https://api.example.com/health"

loadProfile:
  phases:
    - name: "steady"
      duration: 1m
      actors: 10
      rps: 100

thresholds:
  http_req_duration:
    p95: 500ms    # p95 must be < 500ms
    p99: 1s       # p99 must be < 1s
  http_req_failed:
    rate: 1%      # error rate must be < 1%
```

### Threshold Fields

**http_req_duration** - Response time limits:
| Field | Description |
|-------|-------------|
| `avg` | Average response time |
| `p50` | 50th percentile (median) |
| `p90` | 90th percentile |
| `p95` | 95th percentile |
| `p99` | 99th percentile |

**http_req_failed** - Error rate limit:
| Field | Description |
|-------|-------------|
| `rate` | Maximum allowed error rate (e.g., `1%`, `0.5%`) |

## Execution Control

Control iteration-level execution for deterministic testing, warmup phases, and CI/CD integration:

```yaml
workflow:
  name: "Deterministic Test"
  steps:
    - name: "api"
      method: GET
      url: "https://api.example.com/health"

execution:
  max_iterations: 20      # Each actor runs exactly 20 iterations
  warmup_iterations: 5    # First 5 iterations per actor excluded from metrics
```

### Execution Fields

| Field | Description |
|-------|-------------|
| `max_iterations` | Maximum iterations per actor (0 = unlimited, run until duration) |
| `warmup_iterations` | Iterations to run before collecting metrics (per-actor) |

### Use Cases

**Deterministic Testing**: Run exactly N iterations for reproducible results in CI/CD:
```bash
maestro --config=test.yaml --max-iterations=100 --actors=5
# Runs exactly 500 requests (5 actors * 100 iterations)
```

**Warmup Phase**: Exclude initial iterations from metrics (JVM warmup, connection pooling, cache warming):
```bash
maestro --config=test.yaml --warmup=10 --duration=1m
# First 10 iterations per actor not counted in metrics
```

**Combined with Load Profiles**: When both `execution` and `loadProfile` are set, the test stops when EITHER limit is reached (whichever comes first):
```yaml
loadProfile:
  phases:
    - name: "steady"
      duration: 1m
      actors: 10

execution:
  max_iterations: 50    # Safety cap: stop after 50 iterations even if time remains
  warmup_iterations: 5  # Each actor warms up independently
```

CLI flags (`--max-iterations`, `--warmup`) override config file values.

## Output Formats

### Text Output (default)

```
Maestro - Load Test Results
==============================

Duration:       1m30s
Total Requests: 4,523
Success Rate:   99.2% (4,487 / 4,523)
Requests/sec:   50.3

Response Times:
  Min:    12ms
  Avg:    145ms
  P50:    132ms
  P90:    245ms
  P95:    312ms
  P99:    487ms
  Max:    1.2s

By Step:
  health          4,523 reqs   avg=145ms  p95=312ms  p99=487ms

Thresholds:
  ✓ http_req_duration.p95 < 500ms (actual: 312ms)
  ✓ http_req_duration.p99 < 1s (actual: 487ms)
  ✓ http_req_failed.rate < 1% (actual: 0.8%)
```

### JSON Output

Use `--output=json` for CI/CD integration:

```bash
maestro --config=config.yaml --output=json
```

```json
{
  "duration": "1m30s",
  "totalRequests": 4523,
  "successCount": 4487,
  "failureCount": 36,
  "successRate": 99.2,
  "requestsPerSec": 50.3,
  "durations": {
    "min": "12ms",
    "avg": "145ms",
    "p50": "132ms",
    "p90": "245ms",
    "p95": "312ms",
    "p99": "487ms",
    "max": "1.2s"
  },
  "steps": {
    "health": {
      "count": 4523,
      "success": 4487,
      "failed": 36,
      "successRate": 99.2,
      "durations": { ... }
    }
  },
  "thresholds": {
    "passed": true,
    "results": [
      {"name": "http_req_duration.p95", "passed": true, "threshold": "500ms", "actual": "312ms"}
    ]
  }
}
```

## Examples

The `examples/` folder contains ready-to-run configurations:

### Simple
```bash
# Single GET request
maestro --config=examples/simple/health-check.yaml --actors=2 --duration=5s

# POST with JSON body
maestro --config=examples/simple/post-json.yaml --actors=2 --duration=5s

# Custom headers
maestro --config=examples/simple/with-headers.yaml --actors=2 --duration=5s
```

### Workflows
```bash
# CRUD operations flow
maestro --config=examples/workflows/crud-api.yaml --actors=3 --duration=10s

# Authentication flow
maestro --config=examples/workflows/auth-flow.yaml --actors=3 --duration=10s

# Multiple endpoints
maestro --config=examples/workflows/multi-endpoint.yaml --actors=3 --duration=10s
```

### Stress Tests
```bash
# Large payload
maestro --config=examples/stress/large-payload.yaml --actors=5 --duration=10s
```

### Load Profiles
```bash
# Ramp up, steady state, ramp down
maestro --config=examples/profiles/ramp-up-down.yaml

# Constant rate with RPS limiting
maestro --config=examples/profiles/steady-rate.yaml

# Burst/spike pattern
maestro --config=examples/profiles/burst.yaml
```

### Execution Control
```bash
# Run exactly 10 iterations per actor (deterministic)
maestro --config=examples/execution/max-iterations.yaml --actors=5

# Warmup: exclude first 5 iterations from metrics
maestro --config=examples/execution/warmup.yaml --actors=3 --duration=30s

# Combined: warmup + exact iteration count
maestro --config=examples/execution/deterministic.yaml --actors=3

# With thresholds for CI/CD
maestro --config=examples/execution/with-thresholds.yaml --actors=5

# Combined with load profile
maestro --config=examples/execution/with-profile.yaml

# Override via CLI flags
maestro --config=examples/local/health-check.yaml --max-iterations=50 --warmup=5 --actors=3
```

### Thresholds
```bash
# Test with passing thresholds (exit code 0)
maestro --config=examples/thresholds/passing.yaml
echo $?  # 0

# Test with failing thresholds (exit code 1)
maestro --config=examples/thresholds/failing.yaml
echo $?  # 1

# JSON output for CI/CD
maestro --config=examples/thresholds/passing.yaml --output=json --quiet
```

### Local Test Server

Maestro includes a configurable test server for fast, offline testing:

```bash
# Build and start the test server
go build -o /tmp/testserver ./cmd/testserver
/tmp/testserver --port=8080

# In another terminal, run tests against it
maestro --config=examples/local/health-check.yaml --duration=10s
maestro --config=examples/local/load-profile.yaml
maestro --config=examples/local/with-thresholds.yaml
```

Test server endpoints:

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Health check, returns `{"status":"ok"}` |
| `GET /status/{code}` | Returns specified HTTP status code |
| `GET /delay/{ms}` | Delays response by specified milliseconds |
| `POST /echo` | Echoes request body with same content-type |
| `GET /random-delay?min=X&max=Y` | Random delay between min and max ms |
| `GET /fail-rate?rate=N` | Fails N% of requests with 500 status |
| `GET /json` | Returns JSON with id, timestamp, method, path |
| `GET /headers` | Returns request headers as JSON |

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

### Test Categories

```bash
# Unit tests for pure metrics computation
go test ./internal/collector -v -run ComputeMetrics

# Unit tests for output formatting
go test ./internal/collector -v -run Format

# Integration tests (end-to-end)
go test -v -run Integration
```

## Architecture

Maestro uses an actor-based concurrency model:

- **Actors**: Goroutines that execute workflows independently
- **Coordinator**: Spawns/terminates actors dynamically based on load profile
- **PhaseManager**: Tracks current phase and calculates target actor counts
- **RateLimiter**: Token bucket rate limiter for RPS control
- **Collector**: Thin wrapper for event collection and time tracking
- **ComputeMetrics**: Pure function for metrics calculation from events
- **FormatText/FormatJSON**: Standalone formatting functions for output
- **Reporter**: Thread-safe bridge for actors to report events

Each actor runs the complete workflow in a loop until stopped or the duration expires.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed design documentation and [docs/ROADMAP.md](docs/ROADMAP.md) for future plans.

## License

MIT
