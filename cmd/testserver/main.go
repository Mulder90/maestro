// Command testserver runs a configurable HTTP server for load testing.
//
// Usage:
//
//	testserver [flags]
//
// Flags:
//
//	-port    Port to listen on (default: 8080)
//	-host    Host to bind to (default: localhost)
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"maestro/testserver"
)

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	host := flag.String("host", "localhost", "host to bind to")
	flag.Parse()

	server := testserver.NewServer()
	addr := fmt.Sprintf("%s:%d", *host, *port)

	// Print available endpoints
	fmt.Println("Maestro Test Server")
	fmt.Println("======================")
	fmt.Printf("Listening on http://%s\n\n", addr)
	fmt.Println("Endpoints:")
	fmt.Println("  GET  /health              - Health check")
	fmt.Println("  GET  /status/{code}       - Return specific status code")
	fmt.Println("  GET  /delay/{ms}          - Delay response by milliseconds")
	fmt.Println("  POST /echo                - Echo request body")
	fmt.Println("  GET  /random-delay        - Random delay (?min=50&max=200)")
	fmt.Println("  GET  /fail-rate           - Fail percentage of requests (?rate=10)")
	fmt.Println("  GET  /json                - JSON response with metadata")
	fmt.Println("  GET  /headers             - Echo request headers as JSON")
	fmt.Println()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		os.Exit(0)
	}()

	log.Fatal(http.ListenAndServe(addr, server.Handler()))
}
