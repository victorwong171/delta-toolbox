//go:build darwin

package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func enableAnsiColor() {
	// macOS terminals support ANSI colors natively, no special initialization needed
}

func detectLanguagePlatform() bool {
	// Rely on LANG environment variable on macOS
	return false
}

func getHostsPath() string {
	return "/etc/hosts"
}

func isAdminOrRoot() bool {
	return os.Geteuid() == 0
}

func getProcessMap() (map[int]string, error) {
	cmd := exec.Command("ps", "-ax", "-o", "pid,comm")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	processMap := make(map[int]string)
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "PID") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			var pid int
			if _, err := fmt.Sscanf(fields[0], "%d", &pid); err == nil {
				// command path can be full path, let's get the base name
				comm := fields[1]
				for i := 2; i < len(fields); i++ {
					comm += " " + fields[i]
				}
				processMap[pid] = filepath.Base(comm)
			}
		}
	}
	return processMap, nil
}

func testPingDF(target string, size int) (bool, error) {
	// On macOS, -D disables fragmentation, -s specifies data payload size (excluding 8-byte ICMP header), -c 1 sends 1 packet.
	cmd := exec.Command("ping", "-D", "-s", fmt.Sprintf("%d", size), "-c", "1", target)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	output := out.String()
	if err != nil {
		if strings.Contains(output, "too long") || strings.Contains(output, "Message too long") || strings.Contains(output, "frag") {
			return false, nil // fragmented
		}
		return false, fmt.Errorf("ping failed: %v", err)
	}
	if strings.Contains(output, "bytes from") {
		return true, nil
	}
	return false, fmt.Errorf("timeout or unreachable")
}

func getMacGateway() string {
	cmd := exec.Command("route", "-n", "get", "default")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gateway:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1]
			}
		}
	}
	return ""
}

func getMacInterfaceDetails(iface string) (string, string, bool) {
	cmd := exec.Command("ifconfig", iface)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return iface, "Unknown", false
	}
	output := out.String()
	status := false
	speed := "Unknown"

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "status:") {
			if strings.Contains(line, "active") {
				status = true
			}
		}
		if strings.Contains(line, "media:") {
			parts := strings.Split(line, "media:")
			if len(parts) >= 2 {
				speed = strings.TrimSpace(parts[1])
			}
		}
	}
	return iface, speed, status
}

func diagnoseLink(report *DiagnosticReport) {
	report.GatewayIP = getMacGateway()
	if report.GatewayIP == "" {
		report.GatewayIP = "192.168.1.1"
	}

	pingCmd := exec.Command("ping", "-c", "4", report.GatewayIP)
	var pingOut bytes.Buffer
	pingCmd.Stdout = &pingOut
	_ = pingCmd.Run()
	avgLat, loss := parsePingOutput(pingOut.String())
	report.GatewayLatency = avgLat
	report.GatewayLoss = loss

	// Get primary interface details
	cmd := exec.Command("route", "-n", "get", "default")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		lines := strings.Split(out.String(), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "interface:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					name, speed, ok := getMacInterfaceDetails(fields[1])
					report.AdapterName = name
					report.AdapterSpeed = speed
					report.AdapterLinkOK = ok
					break
				}
			}
		}
	}

	if report.GatewayLoss == 0 {
		printColored("success", getT(ResultSuccess), fmt.Sprintf(getT(GatewayPing), report.GatewayIP, fmt.Sprintf("%dms", report.GatewayLatency/time.Millisecond), fmt.Sprintf("%.0f%%", report.GatewayLoss)))
	} else if report.GatewayLoss < 100 {
		printColored("warning", getT(ResultWarning), fmt.Sprintf(getT(GatewayPing), report.GatewayIP, fmt.Sprintf("%dms", report.GatewayLatency/time.Millisecond), fmt.Sprintf("%.0f%%", report.GatewayLoss)))
	} else {
		printColored("danger", getT(ResultDanger), fmt.Sprintf(getT(GatewayPing), report.GatewayIP, "N/A", "100%"))
	}

	if report.AdapterName != "" {
		statusStr := "Down"
		if report.AdapterLinkOK {
			statusStr = "OK"
		}
		printColored("success", getT(ResultSuccess), fmt.Sprintf(getT(LinkSpeed), report.AdapterName, report.AdapterSpeed, statusStr))
	}
}

func diagnoseNAT(report *DiagnosticReport) {
	cmd := exec.Command("netstat", "-an")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		printColored("danger", getT(ResultDanger), "Failed to execute netstat -an: "+err.Error())
		return
	}

	lines := strings.Split(out.String(), "\n")
	tcpCount := 0
	udpCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		proto := strings.ToLower(fields[0])
		if strings.HasPrefix(proto, "tcp") {
			tcpCount++
		} else if strings.HasPrefix(proto, "udp") {
			udpCount++
		}
	}

	report.TCPConns = tcpCount
	report.UDPConns = udpCount
	report.ActiveConns = tcpCount + udpCount

	if report.ActiveConns < 500 {
		printColored("success", getT(ResultSuccess), fmt.Sprintf(getT(NATConnections), report.ActiveConns, report.TCPConns, report.UDPConns))
	} else if report.ActiveConns < 1000 {
		printColored("warning", getT(ResultWarning), fmt.Sprintf(getT(NATConnections), report.ActiveConns, report.TCPConns, report.UDPConns)+" (Moderately high, router load may increase)")
	} else {
		printColored("danger", getT(ResultDanger), fmt.Sprintf(getT(NATConnections), report.ActiveConns, report.TCPConns, report.UDPConns)+" (Critical: extremely high session count, router NAT tables might be exhausted!)")
	}

	// CLOSE_WAIT leak check using lsof
	lsofCmd := exec.Command("lsof", "-i", "-P", "-n")
	var lsofOut bytes.Buffer
	lsofCmd.Stdout = &lsofOut
	_ = lsofCmd.Run()

	closeWaitMap := make(map[int]int)
	procNames := make(map[int]string)
	lsofLines := strings.Split(lsofOut.String(), "\n")
	for _, line := range lsofLines {
		if strings.Contains(line, "CLOSE_WAIT") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				procName := fields[0]
				var pid int
				if _, err := fmt.Sscanf(fields[1], "%d", &pid); err == nil {
					closeWaitMap[pid]++
					procNames[pid] = procName
				}
			}
		}
	}

	leakDetected := false
	for pid, count := range closeWaitMap {
		if count >= 30 {
			procName := procNames[pid]
			report.CloseWaitLeaks = append(report.CloseWaitLeaks, CloseWaitLeak{
				PID:         pid,
				ProcessName: procName,
				Count:       count,
			})
			leakDetected = true
			printColored("danger", getT(ResultDanger), fmt.Sprintf(getT(CLOSEWAITLeak), procName, pid, count))
		}
	}
	if !leakDetected {
		printColored("success", getT(ResultSuccess), "No CLOSE_WAIT socket leaks detected.")
	}

	// Traceroute check
	traceCmd := exec.Command("traceroute", "-q", "1", "-m", "3", "-n", "8.8.8.8")
	var traceOut bytes.Buffer
	traceCmd.Stdout = &traceOut
	_ = traceCmd.Run()

	traceLines := strings.Split(traceOut.String(), "\n")
	hops := []string{}
	for _, line := range traceLines {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		for _, field := range fields {
			cleaned := strings.Trim(field, "()[]")
			if net.ParseIP(cleaned) != nil {
				hops = append(hops, cleaned)
				break
			}
		}
	}

	privateHops := 0
	privateIPs := []string{"N/A", "N/A", "N/A"}
	for i, hopIP := range hops {
		if i < 3 {
			privateIPs[i] = hopIP
			if isPrivateIP(hopIP) {
				privateHops++
			}
		}
	}

	if privateHops >= 2 {
		printColored("warning", getT(ResultWarning), fmt.Sprintf(getT(DoubleNAT), privateIPs[0], privateIPs[1], privateIPs[2]))
	} else {
		printColored("success", getT(ResultSuccess), "Direct NAT to WAN interface (no double NAT or CGNAT detected in path hops).")
	}
}

func parseMacProxySettings(output string) (bool, string, string) {
	enabled := false
	host := ""
	port := ""
	pacUrl := ""

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "HTTPEnable :") {
			if strings.Contains(line, "1") {
				enabled = true
			}
		} else if strings.HasPrefix(line, "HTTPProxy :") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				host = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(line, "HTTPPort :") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				port = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(line, "ProxyAutoConfigEnable :") {
			if strings.Contains(line, "1") {
				enabled = true
			}
		} else if strings.HasPrefix(line, "ProxyAutoConfigURLString :") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				pacUrl = strings.TrimSpace(strings.Join(parts[1:], ":"))
			}
		}
	}
	server := ""
	if host != "" && port != "" {
		server = host + ":" + port
	}
	return enabled, server, pacUrl
}

func diagnoseProxy(report *DiagnosticReport) {
	cmd := exec.Command("scutil", "--proxy")
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run()

	enabled, server, pacUrl := parseMacProxySettings(out.String())
	report.ProxyEnabled = enabled
	report.ProxyServer = server
	report.PACUrl = pacUrl

	envProxy := os.Getenv("HTTP_PROXY")
	if envProxy == "" {
		envProxy = os.Getenv("http_proxy")
	}

	proxyStr := "Disabled"
	if enabled {
		proxyStr = "Enabled"
	}
	if envProxy != "" {
		proxyStr = fmt.Sprintf("Enabled (Env: %s)", envProxy)
	}

	if enabled {
		alive := checkProxyAlive(server)
		if !alive {
			report.ProxyIsOrphaned = true
			printColored("danger", getT(ResultDanger), fmt.Sprintf(getT(ProxyStatus), proxyStr, server, pacUrl)+" (Orphaned Proxy Connection! Server is offline and will block traffic)")
		} else {
			printColored("info", getT(ResultInfo), fmt.Sprintf(getT(ProxyStatus), proxyStr, server, pacUrl))
		}
	} else {
		printColored("success", getT(ResultSuccess), fmt.Sprintf(getT(ProxyStatus), "Disabled", "N/A", "N/A"))
	}

	// Scan active virtual proxy network interfaces (e.g. utun on macOS)
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			name := strings.ToLower(iface.Name)
			if strings.HasPrefix(name, "utun") || strings.HasPrefix(name, "tun") || strings.HasPrefix(name, "tap") {
				printColored("info", getT(ResultInfo), fmt.Sprintf(getT(L3ProxyStatus), iface.Name, "macOS Virtual TUN/TAP Interface"))
			}
		}
	}
}
