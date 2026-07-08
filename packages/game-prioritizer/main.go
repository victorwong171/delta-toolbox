package main

import (
	"flag"
	"log/slog"
	"os"
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
	intervalFlag := flag.Int("interval", config.PollingIntervalMs, "Polling interval in milliseconds (ignored in one-shot mode)")
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

	// Create prioritizer
	osPrioritizer := NewOSPrioritizer()
	prioritizer := NewPrioritizer(config, osPrioritizer)

	slog.Info("Running game prioritizer in one-shot mode...")
	err = prioritizer.RunOnce()
	if err != nil {
		slog.Error("Prioritization failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Prioritization completed successfully")
}
