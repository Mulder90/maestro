# Maestro Architecture

A minimal, idiomatic Go CLI load-testing tool using an actor-based model with goroutines and context.Context.

## Overview

Maestro executes HTTP workflows with configurable concurrency patterns. It supports two modes:

1. **Classic mode**: Fixed number of actors for a set duration
2. **Profile mode**: Dynamic actor scaling with phases (ramp up, steady state, ramp down) and rate limiting

## Component Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                           main.go                                │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────────────┐  │
│  │   Config    │    │  Collector  │    │    Coordinator      │  │
│  │   Loader    │    │  (Reporter) │    │                     │  │
│  └──────┬──────┘    └──────▲──────┘    └──────────┬──────────┘  │
└─────────┼──────────────────┼─────────────────────┼──────────────┘
          │                  │                     │
          ▼                  │                     ▼
   ┌─────────────┐           │              ┌─────────────┐
   │ LoadProfile │           │              │   Actors    │
   │   (YAML)    │           │              │ (goroutines)│
   └──────┬──────┘           │              └──────┬──────┘
          │                  │                     │
          ▼                  │                     ▼
   ┌─────────────┐           │              ┌─────────────┐
   │PhaseManager │           │              │ HTTPWorkflow│
   └──────┬──────┘           │              └──────┬──────┘
          │                  │                     │
          ▼                  │                     ▼
   ┌─────────────┐           │              ┌─────────────┐
   │ RateLimiter │◀──────────┼──────────────│  HTTP Steps │
   └─────────────┘           │              └──────┬──────┘
                             │                     │
                             └─────────────────────┘
                                  Report(Event)
```

## Components

### Core Types (`internal/core/interfaces.go`)

```go
type Event struct {
    ActorID   int
    Timestamp time.Time
    Step      string
    Duration  time.Duration
    Success   bool
    Error     string
}

type Workflow interface {
    Run(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error
}

type Coordinator interface {
    Spawn(ctx context.Context, count int, workflow Workflow)
}

type Reporter interface {
    Report(Event)
}
```

### Component Responsibilities

| Component | File | Responsibility |
|-----------|------|----------------|
| **Config** | `internal/config/config.go` | Parse YAML config files, load profiles |
| **Coordinator** | `internal/coordinator/coordinator.go` | Spawn/terminate actors, manage lifecycle |
| **Collector** | `internal/collector/collector.go` | Aggregate events, compute statistics |
| **HTTPWorkflow** | `internal/http/workflow.go` | Execute HTTP request sequences |
| **PhaseManager** | `internal/ratelimit/phase.go` | Track phases, calculate target actor count |
| **RateLimiter** | `internal/ratelimit/limiter.go` | Token bucket rate limiting |
| **Progress** | `internal/progress/progress.go` | Real-time progress display |

## Execution Modes

### Classic Mode

When no `loadProfile` is defined in the config:

```
main.go
   │
   ├── LoadConfig(path)
   ├── NewCollector()
   ├── NewCoordinator(collector)
   ├── ctx = WithTimeout(duration)
   │
   └── coordinator.Spawn(ctx, actors, workflow)
              │
              └── [actors] goroutines run workflow.Run() in loop
                      │
                      └── Report events until ctx.Done()
```

### Profile Mode

When `loadProfile` is defined:

```
main.go
   │
   ├── LoadConfig(path)  ─────────────────────────┐
   ├── NewCollector()                             │
   ├── NewCoordinator(collector)                  ▼
   ├── NewRateLimiter(rps)              ┌─────────────────┐
   │                                    │   LoadProfile   │
   └── coordinator.RunWithProfile()     │  ┌───────────┐  │
              │                         │  │  Phase 1  │  │
              ▼                         │  │  Phase 2  │  │
       ┌─────────────┐                  │  │  Phase 3  │  │
       │PhaseManager │◀─────────────────│  └───────────┘  │
       └──────┬──────┘                  └─────────────────┘
              │
              │ every 100ms:
              ├── CurrentPhase() → get active phase
              ├── TargetActors() → calculate target count
              ├── spawn/stop actors to match target
              └── update RateLimiter with CurrentRPS()
```

## Data Flow

### Event Reporting

```
Actor (goroutine)
    │
    ├── workflow.Run()
    │       │
    │       ├── RateLimiter.Wait()  ← blocks if rate exceeded
    │       │
    │       └── for each step:
    │               ├── http.Do(request)
    │               └── reporter.Report(Event)
    │                          │
    │                          ▼
    │                   ┌─────────────┐
    │                   │  Collector  │
    │                   │  (channel)  │
    │                   └──────┬──────┘
    │                          │
    │                          ▼
    │                   ┌─────────────┐
    │                   │   events[]  │
    │                   └─────────────┘
    │
    └── repeat until ctx.Done() or stopped
```

### Phase Transitions (Profile Mode)

```
Time ──────────────────────────────────────────────────────▶

     Phase: ramp_up          Phase: steady         Phase: ramp_down
     ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
     │                 │    │                 │    │                 │
     │  actors: 1→50   │    │  actors: 50     │    │  actors: 50→0   │
     │  duration: 30s  │    │  duration: 2m   │    │  duration: 15s  │
     │                 │    │  rps: 100       │    │                 │
     └─────────────────┘    └─────────────────┘    └─────────────────┘

     PhaseManager.TargetActors() returns interpolated value for ramp phases
```

## Rate Limiting

The `RateLimiter` uses a token bucket algorithm (`golang.org/x/time/rate`):

- **Bucket size** = RPS (allows burst up to RPS)
- **Refill rate** = RPS tokens per second
- Shared across all actors for global rate limiting
- `Wait(ctx)` blocks until a token is available

```go
// Before each workflow iteration
if rateLimiter != nil {
    rateLimiter.Wait(ctx)  // blocks if rate exceeded
}
```

## Actor Lifecycle

### Classic Mode
```
Spawn() ──▶ goroutine starts ──▶ workflow.Run() loop ──▶ ctx.Done() ──▶ exit
```

### Profile Mode
```
                    ┌──────────────────────────────────┐
                    │                                  │
spawnWithStop() ──▶ goroutine starts ──▶ workflow.Run() loop
                    │                                  │
                    │    ┌─────────┐                   │
                    └───▶│ stopCh  │◀── stopActors() ──┘
                         └────┬────┘
                              │
                              ▼
                            exit
```

## Project Structure

```
maestro/
├── cmd/
│   ├── maestro/
│   │   └── main.go              # CLI entry point, flag parsing, wiring
│   └── testserver/
│       └── main.go              # Test server CLI
├── internal/
│   ├── collector/
│   │   ├── collector.go         # Event aggregation and summary
│   │   ├── metrics.go           # Metrics computation (percentiles)
│   │   └── thresholds.go        # Threshold checking
│   ├── config/
│   │   └── config.go            # YAML config parsing
│   ├── coordinator/
│   │   └── coordinator.go       # Actor spawning and lifecycle
│   ├── core/
│   │   ├── interfaces.go        # Core interfaces (Workflow, Reporter, etc.)
│   │   └── step.go              # Step interface for multi-protocol support
│   ├── http/
│   │   ├── workflow.go          # HTTP workflow execution
│   │   ├── step.go              # HTTP step implementation
│   │   └── debug.go             # Request/response debugging
│   ├── progress/
│   │   └── progress.go          # Real-time progress display
│   └── ratelimit/
│       ├── limiter.go           # Token bucket rate limiter
│       └── phase.go             # Load profile phase management
├── testserver/
│   └── server.go                # Configurable test server
├── docs/
│   ├── ARCHITECTURE.md          # This file
│   └── ROADMAP.md               # Future plans
├── examples/
│   ├── simple/                  # Basic workflow examples
│   ├── workflows/               # Multi-step workflow examples
│   ├── stress/                  # High-load examples
│   ├── profiles/                # Load profile examples
│   ├── thresholds/              # Threshold examples
│   └── local/                   # Local test server examples
├── integration_test.go          # Integration tests
├── go.mod
└── README.md
```

## Design Principles

1. **No shared mutable state** — actors communicate only via Reporter (channel)
2. **Context is king** — all cancellation flows through `context.Context`
3. **Coordinator is dumb** — it spawns/stops goroutines, doesn't understand workflows
4. **Collector is passive** — it collects events but never controls actors
5. **Workflows are self-contained** — each workflow manages its own HTTP logic
6. **Rate limiting is global** — single limiter shared by all actors
7. **Backward compatible** — configs without `loadProfile` work with classic mode

## Configuration Schema

```yaml
workflow:
  name: string
  steps:
    - name: string
      method: string        # GET, POST, PUT, DELETE, etc.
      url: string
      headers:              # optional
        Header-Name: value
      body: string          # optional

loadProfile:                # optional - enables profile mode
  phases:
    - name: string
      duration: duration    # e.g., 30s, 2m, 1h
      actors: int           # for steady phases
      startActors: int      # for ramp phases
      endActors: int        # for ramp phases
      rps: int              # optional rate limit
```

## Thread Safety

- **Collector**: Uses buffered channel (1000) + mutex for event slice
- **Coordinator**: Uses `sync.WaitGroup` and `atomic.Int32/Int64` for counters
- **RateLimiter**: Thread-safe (from `golang.org/x/time/rate`)
- **PhaseManager**: Read-only after creation (immutable phases)

All components pass race detection (`go test -race`).
