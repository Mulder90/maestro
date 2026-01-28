# Maestro

A simple, fast HTTP load testing tool written in Go.

## Installation

```bash
# From source (in project directory)
go install ./cmd/maestro

# Or directly from GitHub
go install github.com/Mulder90/maestro/cmd/maestro@latest
```

## Quick Start

1. Create a config file `test.yaml`:

```yaml
workflow:
  name: "My API Test"
  steps:
    - name: "health"
      method: GET
      url: "https://api.example.com/health"
```

2. Run the test:

```bash
maestro --config=test.yaml --actors=10 --duration=30s
```

3. See results:

```
Maestro - Load Test Results
==============================

Duration:       30s
Total Requests: 4,523
Success Rate:   99.2% (4,487 / 4,523)
Requests/sec:   150.8

Response Times:
  Min:    12ms
  Avg:    65ms
  P95:    120ms
  P99:    180ms
  Max:    312ms
```

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | required | Path to YAML config |
| `--actors` | 5 | Number of concurrent actors |
| `--duration` | 10s | Test duration |
| `--max-iterations` | 0 | Stop after N iterations per actor (0 = unlimited) |
| `--warmup` | 0 | Warmup iterations excluded from metrics |
| `--output` | text | Output format: `text` or `json` |
| `--quiet` | false | Suppress progress output |
| `--verbose` | false | Log requests/responses |

## Configuration

### Basic Workflow

```yaml
workflow:
  name: "API Test"
  steps:
    - name: "login"
      method: POST
      url: "${env:API_BASE}/auth/login"
      headers:
        Content-Type: "application/json"
      body: '{"user": "test", "pass": "secret"}'
      extract:
        token: "$.auth.token"

    - name: "get_profile"
      method: GET
      url: "${env:API_BASE}/users/me"
      headers:
        Authorization: "Bearer ${token}"
```

Variables use `${var}` syntax. Extract values from JSON responses with `$.path` (JSONPath).
Environment variables use `${env:VAR}`. Built-in functions: `${uuid()}`, `${random(1,100)}`, `${random_string(8)}`, `${timestamp()}`, `${date(2006-01-02)}`.

### Thresholds (CI/CD)

Fail the test if metrics exceed limits:

```yaml
workflow:
  name: "API Test"
  steps:
    - name: "health"
      method: GET
      url: "https://api.example.com/health"

thresholds:
  http_req_duration:
    p95: 200ms
    p99: 500ms
  http_req_failed:
    rate: 1%
```

Exit codes: `0` = passed, `1` = threshold failed, `2` = error

### Load Profiles

Define phases for ramp-up/down patterns:

```yaml
workflow:
  name: "Load Test"
  steps:
    - name: "api"
      method: GET
      url: "https://api.example.com/data"

loadProfile:
  phases:
    - name: "ramp_up"
      duration: 30s
      startActors: 1
      endActors: 50

    - name: "steady"
      duration: 2m
      actors: 50
      rps: 100          # rate limit (optional)

    - name: "ramp_down"
      duration: 30s
      startActors: 50
      endActors: 0
```

### Execution Control

Run exact iterations for deterministic tests:

```yaml
execution:
  max_iterations: 100    # each actor runs exactly 100 iterations
  warmup_iterations: 10  # first 10 excluded from metrics
```

Or via CLI: `--max-iterations=100 --warmup=10`

## Examples

See the `examples/` folder for ready-to-run configs:

```bash
# Start test server (optional, for local testing)
go run ./cmd/testserver &

# Simple tests
maestro --config=examples/local/health-check.yaml --duration=10s

# Load profiles
maestro --config=examples/profiles/ramp-up-down.yaml

# Thresholds
maestro --config=examples/thresholds/passing.yaml
```

## Documentation

- [Contributing](docs/CONTRIBUTING.md) - Development setup, running tests
- [Architecture](docs/ARCHITECTURE.md) - Design and internals
- [Roadmap](docs/ROADMAP.md) - Future plans

## License

MIT
