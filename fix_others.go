//go:build !windows && !darwin

package main

import (
	"bufio"
)

func isHostsWritable() bool {
	return false
}

func restoreDefaultHosts() error {
	return nil
}

func runFixes(report *DiagnosticReport, scanner *bufio.Scanner) {
	printColored("info", "Remediations are not supported on this platform. / 当前系统平台暂不支持自动修复。")
}
