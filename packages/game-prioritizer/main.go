package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Initialize structured logging with default LevelInfo
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Initialize platform-specific capabilities (e.g. enable SeDebugPrivilege)
	initPlatform()

	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		slog.Warn("Failed to load config file, using default configuration", "error", err)
	}

	// CLI Flag overrides
	intervalFlag := flag.Int("interval", config.PollingIntervalMs, "Polling interval in milliseconds")
	priorityFlag := flag.String("priority", config.BoostPriority, "Target CPU priority class (High or AboveNormal)")
	boostAllFlag := flag.Bool("boost-all", config.BoostAnyForeground, "Boost any active foreground process that isn't blacklisted")
	verboseFlag := flag.Bool("verbose", false, "Enable verbose debug logging")
	flag.Parse()

	// Re-initialize structured logging if verbose is set
	if *verboseFlag {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		slog.SetDefault(logger)
		slog.Debug("Verbose debug logging enabled")
	}

	// Apply overrides
	config.PollingIntervalMs = *intervalFlag
	config.BoostPriority = *priorityFlag
	config.BoostAnyForeground = *boostAllFlag

	// Create and start prioritizer
	osPrioritizer := NewOSPrioritizer()
	prioritizer := NewPrioritizer(config, osPrioritizer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS interrupt signals (SIGINT/SIGTERM) to ensure priority is restored upon exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		slog.Info("Received termination signal, shutting down...", "signal", sig)
		cancel()
	}()

	slog.Info("CPU Game Prioritizer initialized successfully. Keep this window running in the background.")
	prioritizer.Start(ctx)
}
