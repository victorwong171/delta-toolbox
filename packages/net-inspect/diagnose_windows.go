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
	"sync"
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
	cmd := exec.Command("powershell", "-NoProfile", "-Command", `[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Get-NetAdapter | ForEach-Object {
		$config = Get-NetIPConfiguration -InterfaceIndex $_.InterfaceIndex -ErrorAction SilentlyContinue
		$ipv4 = $config.IPv4Address.IPAddress
		if ($ipv4 -is [array]) { $ipv4 = $ipv4[0] }
		$gw = $config.IPv4DefaultGateway.NextHop
		if ($gw -is [array]) { $gw = $gw[0] }
		[PSCustomObject]@{
			Index                = $_.InterfaceIndex
			Name                 = $_.Name
			InterfaceDescription = $_.InterfaceDescription
			LinkSpeed            = $_.LinkSpeed
			Status               = $_.Status
			IPv4Address          = $ipv4
			Gateway              = $gw
		}
	} | ConvertTo-Json`)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var adapters AdapterList
	if err := json.Unmarshal(out.Bytes(), &adapters); err != nil {
		return nil, err
	}
	return []Adapter(adapters), nil
}

func probePing(sourceIP, target string) (time.Duration, float64) {
	if sourceIP == "" || target == "" {
		return 0, 100.0
	}
	isIPv6 := strings.Contains(sourceIP, ":")
	var cmdStr string
	if isIPv6 {
		cmdStr = fmt.Sprintf("[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; ping -6 -n 2 -w 500 -S %s %s", sourceIP, target)
	} else {
		cmdStr = fmt.Sprintf("[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; ping -n 2 -w 500 -S %s %s", sourceIP, target)
	}
	cmd := exec.Command("powershell", "-NoProfile", "-Command", cmdStr)
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run()
	return parsePingOutput(out.String())
}

func runFinalGatewayPing(report *DiagnosticReport) {
	if report.GatewayIP == "" || report.SelectedAdapterIP == "" {
		return
	}
	isIPv6 := strings.Contains(report.SelectedAdapterIP, ":")
	var cmdStr string
	if isIPv6 {
		cmdStr = fmt.Sprintf("[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; ping -6 -n 4 -S %s %s", report.SelectedAdapterIP, report.GatewayIP)
	} else {
		cmdStr = fmt.Sprintf("[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; ping -n 4 -S %s %s", report.SelectedAdapterIP, report.GatewayIP)
	}
	cmd := exec.Command("powershell", "-NoProfile", "-Command", cmdStr)
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run()
	report.GatewayLatency, report.GatewayLoss = parsePingOutput(out.String())
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

func testPingDF(target string, size int, sourceIP string) (bool, error) {
	var cmdStr string
	if sourceIP != "" {
		isIPv6 := strings.Contains(sourceIP, ":")
		if isIPv6 {
			cmdStr = fmt.Sprintf("ping -6 %s -f -n 1 -l %d -S %s", target, size, sourceIP)
		} else {
			cmdStr = fmt.Sprintf("ping %s -f -n 1 -l %d -S %s", target, size, sourceIP)
		}
	} else {
		cmdStr = fmt.Sprintf("ping %s -f -n 1 -l %d", target, size)
	}
	cmd := exec.Command("powershell", "-NoProfile", "-Command", fmt.Sprintf("[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; %s", cmdStr))
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

	adapters, err := getNetAdapters()
	if err != nil {
		printColored("danger", "Failed to retrieve network adapters: "+err.Error())
		return
	}

	var wg sync.WaitGroup
	probedAdapters := make([]Adapter, len(adapters))
	copy(probedAdapters, adapters)

	for i := range probedAdapters {
		ad := &probedAdapters[i]
		if ad.Status != "Up" || ad.IPv4Address == "" {
			continue
		}

		wg.Add(1)
		go func(a *Adapter) {
			defer wg.Done()
			gwLatency := time.Duration(0)
			gwLoss := 100.0
			internetOK := false

			// Probe gateway
			if a.Gateway != "" {
				gwLatency, gwLoss = probePing(a.IPv4Address, a.Gateway)
			}

			// Probe internet
			_, pubLoss := probePing(a.IPv4Address, "223.5.5.5")
			if pubLoss < 100.0 {
				internetOK = true
			}

			a.GatewayLatency = gwLatency
			a.GatewayLoss = gwLoss
			a.InternetOK = internetOK
			a.IsOK = (gwLoss < 100.0 || internetOK)
		}(ad)
	}
	wg.Wait()

	report.Adapters = probedAdapters
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

	var traceCmd *exec.Cmd
	if report.SelectedAdapterIP != "" {
		isIPv6 := strings.Contains(report.SelectedAdapterIP, ":")
		if isIPv6 {
			traceCmd = exec.Command("tracert", "-6", "-S", report.SelectedAdapterIP, "-d", "-h", "3", "2001:4860:4860::8888")
		} else {
			traceCmd = exec.Command("tracert", "-d", "-h", "3", "8.8.8.8")
			printColored("info", " [i] Windows环境下IPv4路由跟踪由系统默认路由托管，无法强制绑定源网卡。")
		}
	} else {
		traceCmd = exec.Command("tracert", "-d", "-h", "3", "8.8.8.8")
	}
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
