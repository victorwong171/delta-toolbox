package main

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

type Prioritizer struct {
	config             *Config
	currentBoostedPID  uint32
	lastForegroundPID  uint32
	originalPriority   uint32
	ignoredPIDs        map[uint32]bool
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
}

func NewPrioritizer(config *Config, osPrioritizer OSPrioritizer) *Prioritizer {
	return &Prioritizer{
		config:        config,
		ignoredPIDs:   make(map[uint32]bool),
		osPrioritizer: osPrioritizer,
	}
}

func (p *Prioritizer) Start(ctx context.Context) {
	slog.Info("Starting game prioritizer monitoring loop", 
		slog.Int("interval_ms", p.config.PollingIntervalMs),
		slog.String("target_priority", p.config.BoostPriority),
	)

	ticker := time.NewTicker(time.Duration(p.config.PollingIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Shutting down prioritizer...")
			p.restoreCurrent()
			return
		case <-ticker.C:
			p.Tick()
		}
	}
}

func (p *Prioritizer) Tick() {
	hwnd, pid, err := p.osPrioritizer.GetForegroundWindowInfo()
	if err != nil {
		slog.Debug("Failed to get foreground window info", "error", err)
		return
	}

	// Desktop, Taskbar or no window focused
	if pid == 0 || hwnd == 0 {
		p.restoreCurrent()
		return
	}

	slog.Debug("Active foreground window detected", "hwnd", hwnd, "pid", pid)

	// If the focused process changed, restore priority of the previous one first
	// and clear the ignored PIDs map so we can try fresh checks for the new window.
	if pid != p.lastForegroundPID {
		p.restoreCurrent()
		p.ignoredPIDs = make(map[uint32]bool)
		p.lastForegroundPID = pid
	}

	// Already boosting this process, noop
	if pid == p.currentBoostedPID {
		return
	}

	// If this PID previously failed (e.g. anti-cheat access denied) during this focus session, skip it
	if p.ignoredPIDs[pid] {
		return
	}

	// Get process executable name
	exeName, err := p.osPrioritizer.GetProcessName(pid)
	if err != nil {
		slog.Warn("Failed to query process name", "pid", pid, "error", err)
		p.ignoredPIDs[pid] = true
		return
	}
	exeNameLower := strings.ToLower(exeName)
	slog.Debug("Resolved foreground process name", "pid", pid, "exeName", exeName)

	// Decision Chain:
	// 1. Blacklist check
	for _, b := range p.config.Blacklist {
		if exeNameLower == b {
			slog.Debug("Skipping blacklisted process", "name", exeName, "pid", pid)
			return
		}
	}

	// 2. Whitelist check (short-circuit fullscreen check)
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
		slog.Debug("Process is in whitelist", "name", exeName)
	} else if p.config.BoostAnyForeground {
		shouldBoost = true
		reason = "boost_any_foreground"
		slog.Debug("Boost any foreground option is enabled")
	} else {
		// 3. Fullscreen check
		slog.Debug("Checking if window is fullscreen/borderless...", "hwnd", hwnd, "name", exeName)
		isFullscreen, err := p.osPrioritizer.IsFullscreen(hwnd, p.config.FuzzyTolerance)
		if err != nil {
			slog.Debug("Failed to check if window is fullscreen", "hwnd", hwnd, "error", err)
		} else if isFullscreen {
			shouldBoost = true
			reason = "fullscreen/borderless"
			slog.Debug("Process window is fullscreen/borderless", "name", exeName)
		} else {
			slog.Debug("Process window is NOT fullscreen/borderless", "name", exeName)
		}
	}

	if shouldBoost {
		// Fetch current priority to restore it later
		origPriority, err := p.osPrioritizer.GetProcessPriority(pid)
		if err != nil {
			slog.Warn("Failed to get original process priority", "name", exeName, "pid", pid, "error", err)
			p.ignoredPIDs[pid] = true
			return
		}

		targetPriority := p.osPrioritizer.GetPriorityClassValue(p.config.BoostPriority)
		
		// If it's already at/above target priority, we don't need to boost it
		if origPriority >= targetPriority {
			slog.Debug("Process already has high or equal priority", "name", exeName, "pid", pid, "priority", origPriority)
			p.currentBoostedPID = pid
			p.originalPriority = origPriority
			return
		}

		slog.Info("Boosting process CPU priority", "name", exeName, "pid", pid, "reason", reason, "priority", p.config.BoostPriority)
		err = p.osPrioritizer.SetProcessPriority(pid, targetPriority)
		if err != nil {
			slog.Error("Failed to set process priority (anti-cheat or missing admin privileges?)", "name", exeName, "pid", pid, "error", err)
			p.ignoredPIDs[pid] = true
			return
		}

		p.currentBoostedPID = pid
		p.originalPriority = origPriority
	}
}

func (p *Prioritizer) restoreCurrent() {
	if p.currentBoostedPID == 0 {
		return
	}

	if p.config.RestoreOnFocusLoss {
		slog.Info("Restoring process CPU priority", "pid", p.currentBoostedPID, "priority_value", p.originalPriority)
		err := p.osPrioritizer.SetProcessPriority(p.currentBoostedPID, p.originalPriority)
		if err != nil {
			// Could fail if the process was closed in the meantime, which is expected
			slog.Debug("Failed to restore process priority (probably process terminated)", "pid", p.currentBoostedPID, "error", err)
		}
	}

	p.currentBoostedPID = 0
	p.originalPriority = 0
}
