package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
)

func main() {
	os.Exit(run())
}

func run() int {
	// Setup context with signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Load config from environment (need it for log level)
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed to load configuration: %v\n", err)
		return 1
	}

	// Parse log level
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	// Create logger with text handler
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Create and run app
	app := NewApp(cfg, logger)
	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "\n✗ Error: %v\n", err)
		return 1
	}

	return 0
}
