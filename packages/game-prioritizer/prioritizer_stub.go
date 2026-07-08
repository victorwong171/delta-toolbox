//go:build !windows

package main

import (
	"fmt"
)

type StubPrioritizer struct{}

func NewOSPrioritizer() OSPrioritizer {
	return &StubPrioritizer{}
}

func initPlatform() {}

func (s *StubPrioritizer) GetForegroundWindowInfo() (hwnd uintptr, pid uint32, err error) {
	return 0, 0, fmt.Errorf("unsupported platform")
}

func (s *StubPrioritizer) GetProcessName(pid uint32) (string, error) {
	return "", fmt.Errorf("unsupported platform")
}

func (s *StubPrioritizer) IsFullscreen(hwnd uintptr, tolerance int) (bool, error) {
	return false, fmt.Errorf("unsupported platform")
}

func (s *StubPrioritizer) GetProcessPriority(pid uint32) (uint32, error) {
	return 0, fmt.Errorf("unsupported platform")
}

func (s *StubPrioritizer) SetProcessPriority(pid uint32, priority uint32) error {
	return fmt.Errorf("unsupported platform")
}

func (s *StubPrioritizer) GetPriorityClassValue(name string) uint32 {
	return 0
}

func (s *StubPrioritizer) SwitchAudioDevice(name string) error {
	return fmt.Errorf("unsupported platform")
}
