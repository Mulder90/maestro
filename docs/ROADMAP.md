# Maestro Roadmap

## Current State

Maestro is a functional HTTP load testing tool with:
- Actor-based concurrency model
- YAML-configured workflows
- Load profiles (ramp, steady, burst patterns)
- Rate limiting
- Percentile metrics (p50, p90, p95, p99)
- JSON output format
- Thresholds with CI/CD exit codes
- Progress indicator
- Graceful shutdown
- Local test server for offline testing
- Verbose mode for request/response debugging
- Step interface for multi-protocol extensibility
- Execution control (max iterations, warmup)
- **Clean architecture with separated concerns (pure functions for metrics computation)**

## Production Readiness Gaps

### 1. Metrics & Reporting

**Current**: Percentiles, JSON output, real-time progress indicator.

**Architecture**: Clean separation of concerns:
- `ComputeMetrics()` - pure function for metrics calculation
- `FormatText()`/`FormatJSON()` - standalone formatting functions
- Tests assert on `*Metrics` values directly (facts, not effects)

**Needed**:
- [x] Percentiles (p50, p90, p95, p99)
- [x] Pure function metrics computation
- [ ] Histogram of response times
- [x] Real-time metrics during test (progress indicator)
- [x] Export formats (JSON) - CSV, HTML still needed
- [ ] Integration with observability tools (Prometheus, InfluxDB, Grafana)

### 2. Assertions & Thresholds

**Current**: Duration and error rate thresholds with exit codes.

**Needed**:
- [x] Threshold definitions (e.g., `p99 < 500ms`, `error_rate < 1%`)
- [x] Exit code based on thresholds (for CI/CD)
- [ ] Per-step assertions
- [ ] Response body validation

### 3. Request Dynamism

**Current**: Variable extraction and substitution with JSONPath support.

**Needed**:
- [x] Variable extraction from responses (JSONPath)
- [x] Variable substitution in requests (`${variable}`)
- [x] Environment variable support (`${env:VAR}`)
- [ ] Data files (CSV, JSON) for parameterization
- [x] Built-in functions: `${uuid()}`, `${random(min,max)}`, `${random_string(len)}`, `${timestamp()}`, `${date(format)}`

### 4. Authentication

**Current**: Manual header configuration only.

**Needed**:
- [ ] OAuth2 client credentials flow
- [ ] JWT token refresh
- [ ] Basic auth helper
- [ ] Cookie jar / session handling

### 5. Operational

**Current**: Progress indicator, graceful shutdown, local test server, verbose mode.

**Needed**:
- [x] Graceful shutdown with partial results
- [x] Progress bar / live dashboard
- [x] Debug mode with request/response logging (`--verbose` flag)
- [ ] Distributed execution (multiple machines)

## Protocol Support

### Current Architecture Limitation

The `Workflow` interface is generic, but `HTTPWorkflow` is tightly coupled:

```go
type Workflow interface {
    Run(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error
}
```

This is actually flexible enough. The issue is:
1. Config parsing assumes HTTP steps
2. No abstraction for "step" or "client"

### Proposed Protocol Abstraction

```
┌─────────────────────────────────────────────────────────┐
│                      Workflow                            │
│  ┌─────────────────────────────────────────────────┐    │
│  │                    Steps[]                       │    │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐           │    │
│  │  │  Step   │ │  Step   │ │  Step   │           │    │
│  │  └────┬────┘ └────┬────┘ └────┬────┘           │    │
│  └───────┼───────────┼───────────┼─────────────────┘    │
└──────────┼───────────┼───────────┼──────────────────────┘
           │           │           │
           ▼           ▼           ▼
     ┌──────────┐ ┌──────────┐ ┌──────────┐
     │  HTTP    │ │  gRPC    │ │WebSocket │
     │ Executor │ │ Executor │ │ Executor │
     └──────────┘ └──────────┘ └──────────┘
```

### Step Interface

```go
type Step interface {
    Execute(ctx context.Context, vars Variables) (Result, error)
    Name() string
}

type Result struct {
    Duration time.Duration
    Success  bool
    Error    string
    Extract  map[string]any  // extracted variables
}

type Variables interface {
    Get(key string) (any, bool)
    Set(key string, value any)
}
```

### Protocol Implementations

| Protocol | Use Case | Complexity |
|----------|----------|------------|
| **HTTP/1.1** | REST APIs | ✅ Done |
| **HTTP/2** | Modern APIs, gRPC-web | Low (stdlib supports it) |
| **gRPC** | Microservices | Medium |
| **WebSocket** | Real-time apps, chat | Medium |
| **GraphQL** | Query APIs | Low (HTTP + query builder) |
| **TCP/UDP** | Raw protocols, games | Medium |
| **Kafka** | Event streaming | High |
| **AMQP** | Message queues | High |

### Config Evolution

Current (HTTP-only):
```yaml
workflow:
  steps:
    - name: "get_user"
      method: GET
      url: "https://api.example.com/users/1"
```

Proposed (multi-protocol):
```yaml
workflow:
  steps:
    - name: "get_user"
      protocol: http
      http:
        method: GET
        url: "https://api.example.com/users/${user_id}"
        extract:
          user_name: "$.name"

    - name: "stream_updates"
      protocol: websocket
      websocket:
        url: "wss://api.example.com/updates"
        send: '{"subscribe": "${user_name}"}'
        expect:
          count: 5
          timeout: 10s

    - name: "call_service"
      protocol: grpc
      grpc:
        address: "localhost:50051"
        service: "UserService"
        method: "GetProfile"
        message:
          user_id: "${user_id}"
```

## Implementation Phases

### Phase 1: Production-Ready HTTP (v1.0)
Focus: Make HTTP testing production-ready

1. **Metrics enhancement** ✅
   - [x] Add percentile calculations (p50, p90, p95, p99)
   - [x] JSON output format
   - [x] Exit codes based on thresholds

2. **Basic dynamism** ✅
   - [x] Variable extraction (JSONPath via gjson)
   - [x] Variable substitution in requests (`${var}`)
   - [x] Environment variable support (`${env:VAR}`)

3. **Operational** ✅
   - [x] Progress indicator
   - [x] Debug/verbose mode (`--verbose` flag)
   - [x] Graceful shutdown
   - [x] Local test server (bonus)

### Phase 2: Extensibility (v1.5) ✅
Focus: Prepare architecture for multi-protocol

1. **Refactor to Step interface** ✅
   - [x] Extract step execution from HTTPWorkflow → `HTTPStep` in `http_step.go`
   - [x] Create Step interface with `Execute()` and `Name()` → `step.go`
   - [x] Add Variables interface for state sharing between steps
   - [x] Update Event struct with Protocol, StatusCode, BytesSent, BytesRecv

2. **Enhanced data handling**
   - [ ] CSV data files
   - [x] Request/response logging (verbose mode)
   - [ ] Assertions framework

### Phase 3: Multi-Protocol (v2.0)
Focus: Add additional protocols

1. **gRPC support**
   - Proto file loading
   - Unary and streaming calls

2. **WebSocket support**
   - Connection lifecycle
   - Message sending/receiving
   - Assertions on messages

3. **GraphQL support**
   - Query/mutation builder
   - Variable injection

### Phase 4: Enterprise (v3.0)
Focus: Scale and integrate

1. **Distributed execution**
   - Controller/worker architecture
   - Result aggregation

2. **Observability integration**
   - Prometheus metrics endpoint
   - OpenTelemetry tracing
   - Grafana dashboard templates

3. **Advanced features**
   - Scenario scripting (JS/Lua)
   - Record & replay
   - Chaos injection

## Decision Points

### 1. Configuration Language
- **YAML** (current): Simple, readable, limited logic
- **HCL**: Terraform-style, better for complex configs
- **Scripting (JS/Lua)**: Maximum flexibility, higher complexity

**Recommendation**: Stay with YAML for v1.x, consider HCL or embedded scripting for v2+

### 2. Plugin Architecture
- **Compiled-in**: All protocols in main binary
- **Go plugins**: Dynamic loading, platform-limited
- **Subprocess**: External executors, language-agnostic

**Recommendation**: Compiled-in for v1-v2, consider subprocess model for v3

### 3. Distributed Model
- **Single binary**: One process does everything
- **Controller/Worker**: Separate coordination from execution
- **Mesh**: Peer-to-peer coordination

**Recommendation**: Single binary through v2, controller/worker for v3

## Competitive Landscape

| Tool | Strengths | Weaknesses |
|------|-----------|------------|
| **k6** | JS scripting, cloud service | Complex for simple tests |
| **Locust** | Python, distributed | Python dependency |
| **Gatling** | Scala DSL, detailed reports | JVM, steep learning curve |
| **wrk** | Extremely fast | Limited features, Lua scripting |
| **hey** | Simple, fast | No workflows, HTTP only |
| **vegeta** | Library + CLI, streaming | No workflows |

**Maestro positioning**:
- Simpler than k6/Gatling for common cases
- More capable than hey/wrk for workflows
- Go-native (single binary, no runtime)
- YAML-first configuration
