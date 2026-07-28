package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/f0rkz/iptrack/internal/api"
	"github.com/f0rkz/iptrack/internal/ipam"
)

func main() {
	listen := flag.String("listen", env("IPTRACK_LISTEN", ":8080"), "HTTP listen address")
	databaseURL := flag.String("database-url", env("DATABASE_URL", "postgres://iptrack:iptrack@localhost:5432/iptrack?sslmode=disable"), "PostgreSQL connection URL")
	flag.Parse()

	startupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := ipam.OpenPostgres(startupCtx, *databaseURL)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	defer store.Close()
	handler := api.New(store, api.Options{DiscoveryWorkers: 64, MaxDiscoveryHosts: 4096})

	server := &http.Server{
		Addr:              *listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	fmt.Printf("iptrack listening on %s (postgres persistence enabled)\n", *listen)
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	case <-shutdown.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("HTTP shutdown: %v", err)
		}
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
