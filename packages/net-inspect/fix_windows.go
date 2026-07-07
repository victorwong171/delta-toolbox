//go:build windows

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func isHostsWritable() bool {
	hostsPath := getHostsPath()
	file, err := os.OpenFile(hostsPath, os.O_WRONLY, 0666)
	if err != nil {
		return false
	}
	file.Close()
	return true
}

func restoreDefaultHosts() error {
	hostsPath := getHostsPath()
	bakPath := hostsPath + ".bak"

	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return err
	}

	err = os.WriteFile(bakPath, data, 0644)
	if err != nil {
		return err
	}

	defaultContent := `# Copyright (c) 1993-2009 Microsoft Corp.
#
# This is a sample HOSTS file used by Microsoft TCP/IP for Windows.
#
# This file contains the mappings of IP addresses to host names. Each
# entry should be kept on an individual line. The IP address should
# be placed in the first column followed by the corresponding host name.
# The IP address and the host name should be separated by at least one
# space.
#
# Additionally, comments (such as these) may be inserted on individual
# lines or following the machine name denoted by a '#' symbol.
#
# For example:
#
#      102.54.94.97     rhino.acme.com          # source server
#       38.25.63.10     x.acme.com              # x client host

# localhost name resolution is handled within DNS itself.
#	127.0.0.1       localhost
#	::1             localhost
`
	return os.WriteFile(hostsPath, []byte(defaultContent), 0644)
}

func runFixes(report *DiagnosticReport, scanner *bufio.Scanner) {
	anyIssues := false

	// Fix 1: Orphaned Proxy
	if report.ProxyIsOrphaned {
		anyIssues = true
		prompt := fmt.Sprintf(getT(FixPrompt), "Disable System Proxy / 禁用系统代理")
		if confirmFix(prompt, scanner) {
			cmd := exec.Command("reg", "add", "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings", "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f")
			if err := cmd.Run(); err == nil {
				printColored("success", getT(FixSuccess), "System Proxy disabled.")
			} else {
				printColored("danger", getT(FixFailed), err.Error())
			}
		} else {
			fmt.Println(getT(FixSkipped))
		}
	}

	// Fix 2: Hosts File anomalies
	if len(report.HostsAnomalies) > 0 {
		anyIssues = true
		prompt := fmt.Sprintf(getT(FixPrompt), "Restore Default Hosts File / 还原默认 Hosts 文件")
		if confirmFix(prompt, scanner) {
			if !isAdminOrRoot() || !isHostsWritable() {
				printColored("danger", "%s", getT(AdminRequired))
			} else {
				if err := restoreDefaultHosts(); err == nil {
					printColored("success", getT(FixSuccess), fmt.Sprintf("Hosts restored. Original backed up to hosts.bak"))
				} else {
					printColored("danger", getT(FixFailed), err.Error())
				}
			}
		} else {
			fmt.Println(getT(FixSkipped))
		}
	}

	// Fix 3: Flush DNS (Recommended for DNS slow-down, packet loss, or hijacking)
	if report.DNSHijacked || report.GatewayLoss > 0 || report.LocalDNSSpeed > 100*time.Millisecond {
		anyIssues = true
		prompt := fmt.Sprintf(getT(FixPrompt), "Flush DNS Cache / 清理 DNS 缓存")
		if confirmFix(prompt, scanner) {
			if !isAdminOrRoot() {
				printColored("warning", "Running without Administrator privileges. Flush DNS might fail. / 未检测到管理员权限，清理 DNS 缓存可能会失败。")
			}
			cmd := exec.Command("ipconfig", "/flushdns")
			if err := cmd.Run(); err == nil {
				printColored("success", getT(FixSuccess), "DNS cache flushed.")
			} else {
				printColored("danger", getT(FixFailed), err.Error())
			}
		} else {
			fmt.Println(getT(FixSkipped))
		}
	}

	// Fix 4: Renew DHCP configuration (Only suggested for severe packet loss or Gateway unreachability)
	if report.GatewayLoss == 100 {
		anyIssues = true
		prompt := fmt.Sprintf(getT(FixPrompt), "Release & Renew DHCP IP Lease (Network will drop briefly) / 释放并重新获取 IP 地址")
		if confirmFix(prompt, scanner) {
			if !isAdminOrRoot() {
				printColored("danger", "%s", getT(AdminRequired))
			} else {
				printColored("info", "Releasing IP...")
				_ = exec.Command("ipconfig", "/release").Run()
				printColored("info", "Renewing IP...")
				cmd := exec.Command("ipconfig", "/renew")
				if err := cmd.Run(); err == nil {
					printColored("success", getT(FixSuccess), "IP address renewed.")
				} else {
					printColored("danger", getT(FixFailed), err.Error())
				}
			}
		} else {
			fmt.Println(getT(FixSkipped))
		}
	}

	if !anyIssues {
		printColored("success", "No issues found that require automated remediation fixes. Your system configurations look good! / 未发现需要修复的配置问题。")
	}
}
