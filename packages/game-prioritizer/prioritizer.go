package main

import (
	"fmt"
	"log/slog"
	"strings"
)

type Prioritizer struct {
	config             *Config
	osPrioritizer      OSPrioritizer
}

// OSPrioritizer defines the platform-specific operations.
type OSPrioritizer interface {
	GetForegroundWindowInfo() (hwnd uintptr, pid uint32, err error)
	GetProcessName(pid uint32) (string, error)
	IsFullscreen(hwnd uintptr, tolerance int) (bool, error)
	GetProcessPriority(pid uint32) (uint32, error)
	SetProcessPriority(pid uint32, priority uint32) error
	GetPriorityClassValue(name string) uint32
	SwitchAudioDevice(name string) error
}

func NewPrioritizer(config *Config, osPrioritizer OSPrioritizer) *Prioritizer {
	return &Prioritizer{
		config:        config,
		osPrioritizer: osPrioritizer,
	}
}

func (p *Prioritizer) RunOnce() error {
	hwnd, pid, err := p.osPrioritizer.GetForegroundWindowInfo()
	if err != nil {
		return fmt.Errorf("failed to get foreground window info: %w", err)
	}

	if pid == 0 || hwnd == 0 {
		return fmt.Errorf("no active foreground window detected")
	}

	exeName, err := p.osPrioritizer.GetProcessName(pid)
	if err != nil {
		return fmt.Errorf("failed to query foreground process name: %w", err)
	}
	exeNameLower := strings.ToLower(exeName)

	// Blacklist check
	for _, b := range p.config.Blacklist {
		if exeNameLower == b {
			return fmt.Errorf("foreground process '%s' is in blacklist", exeName)
		}
	}

	// Whitelist check
	isWhitelisted := false
	for _, w := range p.config.Whitelist {
		if exeNameLower == w {
			isWhitelisted = true
			break
		}
	}

	shouldBoost := false
	reason := ""

	if isWhitelisted {
		shouldBoost = true
		reason = "whitelist"
	} else if p.config.BoostAnyForeground {
		shouldBoost = true
		reason = "boost_any_foreground"
	} else {
		// Fullscreen check
		isFullscreen, err := p.osPrioritizer.IsFullscreen(hwnd, p.config.FuzzyTolerance)
		if err != nil {
			slog.Debug("Failed to check if window is fullscreen", "hwnd", hwnd, "error", err)
		} else if isFullscreen {
			shouldBoost = true
			reason = "fullscreen/borderless"
		}
	}

	if !shouldBoost {
		return fmt.Errorf("foreground process '%s' does not meet prioritizing criteria (not whitelisted/fullscreen)", exeName)
	}

	slog.Info("Detected target game process in foreground", "name", exeName, "pid", pid, "reason", reason)

	// Switch audio device first
	if p.config.SwitchAudioDevice != "" {
		slog.Info("Switching default audio playback device", "target", p.config.SwitchAudioDevice)
		err = p.osPrioritizer.SwitchAudioDevice(p.config.SwitchAudioDevice)
		if err != nil {
			slog.Warn("Failed to switch default audio playback device", "error", err)
			// Continue boosting even if audio switch fails
		}
	}

	// Fetch current priority to check
	origPriority, err := p.osPrioritizer.GetProcessPriority(pid)
	if err != nil {
		return fmt.Errorf("failed to get target process priority: %w", err)
	}

	targetPriority := p.osPrioritizer.GetPriorityClassValue(p.config.BoostPriority)
	if origPriority >= targetPriority {
		slog.Info("Process already has high or equal CPU priority", "name", exeName, "pid", pid)
		return nil
	}

	slog.Info("Boosting process CPU priority", "name", exeName, "pid", pid, "priority", p.config.BoostPriority)
	err = p.osPrioritizer.SetProcessPriority(pid, targetPriority)
	if err != nil {
		return fmt.Errorf("failed to set CPU priority (try running as Administrator): %w", err)
	}

	slog.Info("Prioritization completed successfully")
	return nil
}
