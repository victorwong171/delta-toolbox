//go:build darwin

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

	defaultContent := `##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
`
	return os.WriteFile(hostsPath, []byte(defaultContent), 0644)
}

func disableMacProxy() error {
	cmd := exec.Command("networksetup", "-listallnetworkservices")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return err
	}
	lines := strings.Split(out.String(), "\n")
	for _, service := range lines {
		service = strings.TrimSpace(service)
		if service == "" || strings.Contains(service, "*") || strings.Contains(service, "An asterisk") {
			continue
		}
		_ = exec.Command("networksetup", "-setwebproxystate", service, "off").Run()
		_ = exec.Command("networksetup", "-setsecurewebproxystate", service, "off").Run()
		_ = exec.Command("networksetup", "-setautoproxyurlstate", service, "off").Run()
	}
	return nil
}

func runFixes(report *DiagnosticReport, scanner *bufio.Scanner) {
	anyIssues := false

	// Fix 1: Orphaned Proxy
	if report.ProxyIsOrphaned {
		anyIssues = true
		prompt := fmt.Sprintf(getT(FixPrompt), "Disable System Proxy / 禁用系统代理")
		if confirmFix(prompt, scanner) {
			if !isAdminOrRoot() {
				printColored("warning", "Disabling system proxy may require root/admin privileges on macOS. / 禁用系统代理在 macOS 上可能需要 root 或管理员权限。")
			}
			if err := disableMacProxy(); err == nil {
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
				printColored("danger", "This action requires root privileges! Please run with: sudo ./net_inspect / 此操作需要 root 权限！请使用 sudo ./net_inspect 重新运行。")
			} else {
				if err := restoreDefaultHosts(); err == nil {
					printColored("success", getT(FixSuccess), fmt.Sprintf("Hosts restored. Original backed up to /etc/hosts.bak"))
				} else {
					printColored("danger", getT(FixFailed), err.Error())
				}
			}
		} else {
			fmt.Println(getT(FixSkipped))
		}
	}

	// Fix 3: Flush DNS
	if report.DNSHijacked || report.GatewayLoss > 0 || report.LocalDNSSpeed > 100*time.Millisecond {
		anyIssues = true
		prompt := fmt.Sprintf(getT(FixPrompt), "Flush DNS Cache / 清理 DNS 缓存")
		if confirmFix(prompt, scanner) {
			if !isAdminOrRoot() {
				printColored("warning", "mDNSResponder reload requires root privileges. Please run with sudo if it fails. / 重启 mDNSResponder 需要 root 权限，若失败请使用 sudo 运行。")
			}
			// Flush DNS cache command on macOS
			_ = exec.Command("dscacheutil", "-flushcache").Run()
			cmd := exec.Command("killall", "-HUP", "mDNSResponder")
			if err := cmd.Run(); err == nil {
				printColored("success", getT(FixSuccess), "DNS cache flushed and mDNSResponder restarted.")
			} else {
				printColored("danger", getT(FixFailed), err.Error())
			}
		} else {
			fmt.Println(getT(FixSkipped))
		}
	}

	// Fix 4: Renew DHCP configuration
	if report.GatewayLoss == 100 && report.AdapterName != "" {
		anyIssues = true
		prompt := fmt.Sprintf(getT(FixPrompt), fmt.Sprintf("Renew DHCP configuration on %s / 重新获取 IP 地址", report.AdapterName))
		if confirmFix(prompt, scanner) {
			if !isAdminOrRoot() {
				printColored("danger", "Renewing DHCP requires root privileges! Please run with: sudo ./net_inspect / 此操作需要 root 权限！请使用 sudo ./net_inspect 重新运行。")
			} else {
				printColored("info", fmt.Sprintf("Renewing DHCP on interface %s...", report.AdapterName))
				cmd := exec.Command("ipconfig", "set", report.AdapterName, "DHCP")
				if err := cmd.Run(); err == nil {
					printColored("success", getT(FixSuccess), "DHCP lease renewed.")
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
