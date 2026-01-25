package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"maestro/internal/collector"
	"maestro/internal/config"
	"maestro/internal/coordinator"
	httpworkflow "maestro/internal/http"
	"maestro/internal/progress"
	"maestro/internal/ratelimit"
)

const (
	ExitSuccess         = 0
	ExitThresholdFailed = 1
	ExitError           = 2
)

func main() {
	configPath := flag.String("config", "", "path to YAML config file (required)")
	actors := flag.Int("actors", 5, "number of initial actors to spawn")
	duration := flag.Duration("duration", 10*time.Second, "test duration")
	output := flag.String("output", "text", "output format: text, json")
	quiet := flag.Bool("quiet", false, "suppress progress output during test")
	verbose := flag.Bool("verbose", false, "enable debug output (request/response logging)")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config is required")
		flag.Usage()
		os.Exit(ExitError)
	}

	if *output != "text" && *output != "json" {
		fmt.Fprintf(os.Stderr, "error: --output must be 'text' or 'json', got %q\n", *output)
		os.Exit(ExitError)
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(ExitError)
	}

	coll := collector.NewCollector()
	coord := coordinator.NewCoordinator(coll)

	var debugLogger *httpworkflow.DebugLogger
	if *verbose {
		debugLogger = httpworkflow.NewDebugLogger(os.Stderr)
	}

	workflow := &httpworkflow.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
		Debug: debugLogger,
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	interrupted := false
	go func() {
		<-sigCh
		interrupted = true
		if !*quiet {
			fmt.Fprintln(os.Stderr, "\nReceived interrupt signal, shutting down...")
		}
		cancel()
	}()

	prog := progress.NewProgress(coll, *quiet)

	if cfg.LoadProfile != nil && len(cfg.LoadProfile.Phases) > 0 {
		runWithProfile(ctx, cfg, coord, workflow, coll, prog)
	} else {
		runClassic(ctx, cfg, coord, workflow, coll, prog, *actors, *duration)
	}

	prog.Stop()

	metrics := coll.Compute()

	var thresholdResults *collector.ThresholdResults
	if cfg.Thresholds != nil {
		thresholdResults = cfg.Thresholds.Check(metrics)
	}

	if *output == "json" {
		coll.PrintJSON(os.Stdout, metrics, thresholdResults)
	} else {
		coll.PrintText(os.Stdout, metrics, thresholdResults)
	}

	if interrupted {
		os.Exit(ExitSuccess)
	}

	if thresholdResults != nil && !thresholdResults.Passed {
		if *output == "text" {
			fmt.Fprintln(os.Stderr, "\nThreshold check failed!")
		}
		os.Exit(ExitThresholdFailed)
	}

	os.Exit(ExitSuccess)
}

func runClassic(ctx context.Context, cfg *config.Config, coord *coordinator.Coordinator, workflow *httpworkflow.Workflow, coll *collector.Collector, prog *progress.Progress, actors int, duration time.Duration) {
	if actors < 1 {
		fmt.Fprintln(os.Stderr, "error: --actors must be >= 1")
		os.Exit(ExitError)
	}

	prog.Printf("Maestro starting: %d actors, duration %v, workflow %q",
		actors, duration, cfg.Workflow.Name)

	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	prog.Start()
	coord.Spawn(ctx, actors, workflow)
	coord.Wait()
	coll.Close()
}

func runWithProfile(ctx context.Context, cfg *config.Config, coord *coordinator.Coordinator, workflow *httpworkflow.Workflow, coll *collector.Collector, prog *progress.Progress) {
	profile := cfg.LoadProfile

	prog.Printf("Maestro starting with load profile, workflow %q", cfg.Workflow.Name)

	// Find first non-zero RPS to initialize rate limiter
	var rateLimiter *ratelimit.RateLimiter
	for _, phase := range profile.Phases {
		if phase.RPS > 0 {
			rateLimiter = ratelimit.NewRateLimiter(phase.RPS)
			break
		}
	}

	workflow.RateLimiter = rateLimiter

	totalDuration := profile.TotalDuration() + 5*time.Second
	ctx, cancel := context.WithTimeout(ctx, totalDuration)
	defer cancel()

	prog.Start()
	coord.RunWithProfile(ctx, profile, workflow, rateLimiter, prog)
	coord.Wait()
	coll.Close()
}
