//go:build !windows && !darwin

package main

import (
	"errors"
)

func enableAnsiColor() {}

func detectLanguagePlatform() bool {
	return false
}

func getHostsPath() string {
	return "/etc/hosts"
}

func getProcessMap() (map[int]string, error) {
	return nil, errors.New("not implemented on this platform")
}

func testPingDF(target string, size int, sourceIP string) (bool, error) {
	return false, errors.New("not implemented on this platform")
}

func getNetAdapters() ([]Adapter, error) {
	return nil, errors.New("not implemented on this platform")
}

func diagnoseLink(report *DiagnosticReport) {
	printColored("warning", "Physical link diagnostics are not implemented on this platform.")
}

func diagnoseNAT(report *DiagnosticReport) {
	printColored("warning", "NAT diagnostics are not implemented on this platform.")
}

func diagnoseProxy(report *DiagnosticReport) {
	printColored("warning", "Proxy diagnostics are not implemented on this platform.")
}

func isAdminOrRoot() bool {
	return false
}

func runFinalGatewayPing(report *DiagnosticReport) {}
