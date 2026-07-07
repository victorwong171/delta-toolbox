//go:build windows

package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func enableAnsiColor() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode := kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode := kernel32.NewProc("SetConsoleMode")

	handle, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return
	}

	var mode uint32
	ret, _, _ := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return
	}

	mode |= 0x0004 // ENABLE_VIRTUAL_TERMINAL_PROCESSING
	_, _, _ = procSetConsoleMode.Call(uintptr(handle), uintptr(mode))
}

func detectLanguagePlatform() bool {
	cmd := exec.Command("reg", "query", "HKCU\\Control Panel\\International", "/v", "LocaleName")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		output := out.String()
		if strings.Contains(output, "zh-") {
			return true
		}
	}
	return false
}

func getHostsPath() string {
	return filepath.Join(os.Getenv("SystemRoot"), "System32", "drivers", "etc", "hosts")
}

func isAdminOrRoot() bool {
	// A simple and reliable way to check for admin privileges on Windows
	// is to try to run a command that requires it, such as "fsutil dirty query <drive>:"
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		drive = "C:"
	}
	cmd := exec.Command("fsutil", "dirty", "query", drive)
	if err := cmd.Run(); err == nil {
		return true
	}
	return false
}

func getNetAdapters() ([]Adapter, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Get-NetAdapter | Select-Object Name, InterfaceDescription, LinkSpeed, Status | ConvertTo-Json")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	jsonBytes := out.Bytes()
	trimmed := strings.TrimSpace(string(jsonBytes))
	if len(trimmed) > 0 && trimmed[0] == '{' {
		trimmed = "[" + trimmed + "]"
		jsonBytes = []byte(trimmed)
	}

	var adapters []Adapter
	if err := json.Unmarshal(jsonBytes, &adapters); err != nil {
		return nil, err
	}
	return adapters, nil
}

func getProcessMap() (map[int]string, error) {
	cmd := exec.Command("tasklist", "/FO", "CSV", "/NH")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	r := csv.NewReader(&out)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	processMap := make(map[int]string)
	for _, record := range records {
		if len(record) >= 2 {
			name := record[0]
			pidStr := record[1]
			var pid int
			if _, err := fmt.Sscanf(pidStr, "%d", &pid); err == nil {
				processMap[pid] = name
			}
		}
	}
	return processMap, nil
}

func parseProxySettings(output string) (bool, string, string) {
	enabled := false
	server := ""
	pacUrl := ""

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ProxyEnable") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				val := parts[len(parts)-1]
				if val == "0x1" {
					enabled = true
				}
			}
		} else if strings.HasPrefix(line, "ProxyServer") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				server = strings.Join(parts[2:], " ")
			}
		} else if strings.HasPrefix(line, "AutoConfigURL") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				pacUrl = strings.Join(parts[2:], " ")
			}
		}
	}
	return enabled, server, pacUrl
}

func testPingDF(target string, size int) (bool, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", fmt.Sprintf("[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; ping %s -f -n 1 -l %d", target, size))
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run()

	output := out.String()
	if strings.Contains(output, "Reply from") || strings.Contains(output, "来自") || strings.Contains(output, "bytes=") || strings.Contains(output, "字节=") {
		return true, nil
	}
	if strings.Contains(output, "fragmented") || strings.Contains(output, "分片") {
		return false, nil
	}
	return false, fmt.Errorf("timeout or unreachable")
}

func diagnoseLink(report *DiagnosticReport) {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1 -ExpandProperty NextHop")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		report.GatewayIP = strings.TrimSpace(out.String())
	}
	if report.GatewayIP == "" {
		report.GatewayIP = "192.168.1.1"
	}

	pingCmd := exec.Command("powershell", "-NoProfile", "-Command", fmt.Sprintf("[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; ping -n 4 %s", report.GatewayIP))
	var pingOut bytes.Buffer
	pingCmd.Stdout = &pingOut
	_ = pingCmd.Run()
	avgLat, loss := parsePingOutput(pingOut.String())
	report.GatewayLatency = avgLat
	report.GatewayLoss = loss

	if adapters, err := getNetAdapters(); err == nil {
		for _, ad := range adapters {
			if ad.Status == "Up" && !strings.Contains(ad.InterfaceDescription, "Virtual") && !strings.Contains(ad.InterfaceDescription, "Tunnel") && !strings.Contains(ad.InterfaceDescription, "VMware") && !strings.Contains(ad.InterfaceDescription, "VirtualBox") {
				report.AdapterName = ad.Name
				report.AdapterSpeed = ad.LinkSpeed
				report.AdapterLinkOK = true
				break
			}
		}
		if report.AdapterName == "" {
			for _, ad := range adapters {
				if ad.Status == "Up" {
					report.AdapterName = ad.Name
					report.AdapterSpeed = ad.LinkSpeed
					report.AdapterLinkOK = true
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
		isDegraded := false
		if strings.Contains(report.AdapterSpeed, "Mbps") {
			var speedVal int
			_, _ = fmt.Sscanf(report.AdapterSpeed, "%d", &speedVal)
			if speedVal <= 100 && (strings.Contains(strings.ToLower(report.AdapterName), "以太网") || strings.Contains(strings.ToLower(report.AdapterName), "ethernet")) {
				isDegraded = true
			}
		}
		if isDegraded {
			printColored("warning", getT(ResultWarning), fmt.Sprintf(getT(LinkSpeed), report.AdapterName, report.AdapterSpeed, "Degraded (Negotiated at <=100Mbps Ethernet)"))
		} else {
			printColored("success", getT(ResultSuccess), fmt.Sprintf(getT(LinkSpeed), report.AdapterName, report.AdapterSpeed, "OK"))
		}
	}
}

func diagnoseNAT(report *DiagnosticReport) {
	cmd := exec.Command("netstat", "-ano")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		printColored("danger", getT(ResultDanger), "Failed to execute netstat -ano: "+err.Error())
		return
	}

	lines := strings.Split(out.String(), "\n")
	tcpCount := 0
	udpCount := 0
	closeWaitMap := make(map[int]int)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		proto := strings.ToUpper(fields[0])
		if proto == "TCP" {
			tcpCount++
			state := ""
			pid := 0
			if len(fields) >= 5 {
				state = strings.ToUpper(fields[3])
				_, _ = fmt.Sscanf(fields[4], "%d", &pid)
			}
			if state == "CLOSE_WAIT" && pid > 0 {
				closeWaitMap[pid]++
			}
		} else if proto == "UDP" {
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

	processMap, _ := getProcessMap()
	leakDetected := false
	for pid, count := range closeWaitMap {
		if count >= 30 {
			procName := "Unknown"
			if name, ok := processMap[pid]; ok {
				procName = name
			}
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

	traceCmd := exec.Command("tracert", "-d", "-h", "3", "8.8.8.8")
	var traceOut bytes.Buffer
	traceCmd.Stdout = &traceOut
	_ = traceCmd.Run()

	traceLines := strings.Split(traceOut.String(), "\n")
	hops := []string{}
	for _, line := range traceLines {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			ipCandidate := fields[len(fields)-1]
			if net.ParseIP(ipCandidate) != nil {
				hops = append(hops, ipCandidate)
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

func diagnoseProxy(report *DiagnosticReport) {
	cmd := exec.Command("reg", "query", "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings")
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run()

	enabled, server, pacUrl := parseProxySettings(out.String())
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

	// Scan active L3 virtual network proxy/VPN adapters
	if adapters, err := getNetAdapters(); err == nil {
		for _, ad := range adapters {
			if isVPNOrProxyAdapter(ad) {
				printColored("info", getT(ResultInfo), fmt.Sprintf(getT(L3ProxyStatus), ad.Name, ad.InterfaceDescription))
			}
		}
	}
}
