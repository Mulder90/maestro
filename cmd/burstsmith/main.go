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

	"burstsmith"
)

// Exit codes
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

	cfg, err := burstsmith.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(ExitError)
	}

	// Wire components
	collector := burstsmith.NewCollector()
	coordinator := burstsmith.NewCoordinator(collector)

	// Create debug logger if verbose mode enabled
	var debugLogger *burstsmith.DebugLogger
	if *verbose {
		debugLogger = burstsmith.NewDebugLogger(os.Stderr)
	}

	// Create HTTP workflow with shared client
	workflow := &burstsmith.HTTPWorkflow{
		Config: cfg.Workflow,
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
		Debug: debugLogger,
	}

	// Set up graceful shutdown
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

	// Create progress indicator
	progress := burstsmith.NewProgress(collector, *quiet)

	// Check if load profile is defined
	if cfg.LoadProfile != nil && len(cfg.LoadProfile.Phases) > 0 {
		runWithProfile(ctx, cfg, coordinator, workflow, collector, progress)
	} else {
		runClassic(ctx, cfg, coordinator, workflow, collector, progress, *actors, *duration)
	}

	// Stop progress indicator
	progress.Stop()

	// Compute metrics
	metrics := collector.Compute()

	// Check thresholds
	var thresholdResults *burstsmith.ThresholdResults
	if cfg.Thresholds != nil {
		thresholdResults = cfg.Thresholds.Check(metrics)
	}

	// Output results
	if *output == "json" {
		collector.PrintJSON(os.Stdout, metrics, thresholdResults)
	} else {
		collector.PrintText(os.Stdout, metrics, thresholdResults)
	}

	// Determine exit code
	if interrupted {
		os.Exit(ExitSuccess) // Partial results are fine on interrupt
	}

	if thresholdResults != nil && !thresholdResults.Passed {
		if *output == "text" {
			fmt.Fprintln(os.Stderr, "\nThreshold check failed!")
		}
		os.Exit(ExitThresholdFailed)
	}

	os.Exit(ExitSuccess)
}

// runClassic executes the workflow with fixed actors and duration (original behavior).
func runClassic(ctx context.Context, cfg *burstsmith.Config, coordinator *burstsmith.DefaultCoordinator, workflow *burstsmith.HTTPWorkflow, collector *burstsmith.Collector, progress *burstsmith.Progress, actors int, duration time.Duration) {
	if actors < 1 {
		fmt.Fprintln(os.Stderr, "error: --actors must be >= 1")
		os.Exit(ExitError)
	}

	progress.Printf("BurstSmith starting: %d actors, duration %v, workflow %q",
		actors, duration, cfg.Workflow.Name)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	// Start progress indicator
	progress.Start()

	coordinator.Spawn(ctx, actors, workflow)

	// Wait for all actors to complete
	coordinator.Wait()

	// Stop collector
	collector.Close()
}

// runWithProfile executes the workflow according to the load profile.
func runWithProfile(ctx context.Context, cfg *burstsmith.Config, coordinator *burstsmith.DefaultCoordinator, workflow *burstsmith.HTTPWorkflow, collector *burstsmith.Collector, progress *burstsmith.Progress) {
	profile := cfg.LoadProfile

	progress.Printf("BurstSmith starting with load profile, workflow %q", cfg.Workflow.Name)

	// Determine initial RPS from first phase
	initialRPS := 0
	if len(profile.Phases) > 0 {
		initialRPS = profile.Phases[0].RPS
	}

	// Create rate limiter if any phase has RPS limit
	var rateLimiter *burstsmith.RateLimiter
	for _, phase := range profile.Phases {
		if phase.RPS > 0 {
			rateLimiter = burstsmith.NewRateLimiter(initialRPS)
			break
		}
	}

	// Attach rate limiter to workflow
	workflow.RateLimiter = rateLimiter

	// Create context with total profile duration plus buffer
	totalDuration := profile.TotalDuration() + 5*time.Second
	ctx, cancel := context.WithTimeout(ctx, totalDuration)
	defer cancel()

	// Start progress indicator
	progress.Start()

	// Run with profile-based actor management
	coordinator.RunWithProfile(ctx, profile, workflow, rateLimiter, progress)

	// Wait for all actors to finish
	coordinator.Wait()

	// Stop collector
	collector.Close()
}
