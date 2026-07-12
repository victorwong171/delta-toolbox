package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

type DiagnosticReport struct {
	GatewayIP           string
	GatewayLatency      time.Duration
	GatewayLoss         float64
	AdapterName         string
	AdapterSpeed        string
	AdapterLinkOK       bool
	Adapters            []Adapter
	SelectedAdapterIP   string
	SelectedAdapterName string

	MaxPayload     int
	OptimumMTU     int
	MTUProbeStatus string

	ActiveConns    int
	TCPConns       int
	UDPConns       int
	CloseWaitLeaks []CloseWaitLeak

	ProxyEnabled    bool
	ProxyServer     string
	PACUrl          string
	ProxyIsOrphaned bool

	TotalHosts     int
	HostsAnomalies []string

	LocalDNSSpeed time.Duration
	AliDNSSpeed   time.Duration
	DNSPodSpeed   time.Duration
	DNSHijacked   bool
}

type CloseWaitLeak struct {
	PID         int
	ProcessName string
	Count       int
}

type Adapter struct {
	Index                int    `json:"Index"`
	Name                 string `json:"Name"`
	InterfaceDescription string `json:"InterfaceDescription"`
	LinkSpeed            string `json:"LinkSpeed"`
	Status               string `json:"Status"`
	IPv4Address          string `json:"IPv4Address"`
	Gateway              string `json:"Gateway"`
	GatewayLatency       time.Duration
	GatewayLoss          float64
	InternetOK           bool
	IsOK                 bool
}

type AdapterList []Adapter

func (al *AdapterList) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil
	}
	if trimmed[0] == '[' {
		type Alias AdapterList
		return json.Unmarshal(trimmed, (*Alias)(al))
	}
	var single Adapter
	if err := json.Unmarshal(trimmed, &single); err != nil {
		return err
	}
	*al = []Adapter{single}
	return nil
}

func isVPNOrProxyAdapter(ad Adapter) bool {
	if ad.Status != "Up" {
		return false
	}
	name := strings.ToLower(ad.Name)
	desc := strings.ToLower(ad.InterfaceDescription)

	keywords := []string{"tun", "tap", "clash", "mihomo", "wireguard", "tailscale", "zerotier", "openvpn", "vpn", "anyconnect", "fortissl", "forticlient", "globalprotect"}
	for _, kw := range keywords {
		if strings.Contains(name, kw) || strings.Contains(desc, kw) {
			if strings.Contains(desc, "vmware") || strings.Contains(desc, "virtualbox") || strings.Contains(desc, "vbox") {
				continue
			}
			return true
		}
	}
	return false
}

func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	if ip4[0] == 10 {
		return true
	}
	if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
		return true
	}
	if ip4[0] == 192 && ip4[1] == 168 {
		return true
	}
	if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true
	}
	return false
}

func parsePingOutput(output string) (time.Duration, float64) {
	var avgLatency time.Duration = 0
	var loss float64 = 100.0

	// Parse loss percentage (universal check for % character)
	lossIndex := strings.Index(output, "%")
	if lossIndex != -1 {
		start := lossIndex
		for start > 0 && ((output[start-1] >= '0' && output[start-1] <= '9') || output[start-1] == '.') {
			start--
		}
		if start < lossIndex {
			_, _ = fmt.Sscanf(output[start:lossIndex], "%f", &loss)
		}
	}

	// Parse latency average
	rtIndex := strings.Index(output, "round-trip")
	if rtIndex != -1 {
		// macOS / Linux format: min/avg/max/stddev = 1.011/1.234/1.567/0.200 ms
		lines := strings.Split(output[rtIndex:], "\n")
		if len(lines) > 0 {
			line := lines[0]
			eqIndex := strings.Index(line, "=")
			if eqIndex != -1 {
				statsStr := strings.TrimSpace(line[eqIndex+1:])
				statsStr = strings.TrimSuffix(statsStr, " ms")
				parts := strings.Split(statsStr, "/")
				if len(parts) >= 2 {
					var avgMs float64
					if _, err := fmt.Sscanf(parts[1], "%f", &avgMs); err == nil {
						avgLatency = time.Duration(avgMs * float64(time.Millisecond))
					}
				}
			}
		}
	} else {
		// Windows format: Average = 12ms / 平均 = 12ms
		avgIndex := strings.Index(output, "Average =")
		if avgIndex == -1 {
			avgIndex = strings.Index(output, "平均 =")
		}
		if avgIndex != -1 {
			str := output[avgIndex:]
			msIndex := strings.Index(str, "ms")
			if msIndex != -1 {
				var msVal int
				equalIndex := strings.Index(str, "=")
				if equalIndex != -1 && equalIndex < msIndex {
					numStr := strings.TrimSpace(str[equalIndex+1 : msIndex])
					if strings.Contains(numStr, "<") {
						numStr = "0"
					}
					_, _ = fmt.Sscanf(numStr, "%d", &msVal)
					avgLatency = time.Duration(msVal) * time.Millisecond
				}
			}
		}
	}
	return avgLatency, loss
}

func checkProxyAlive(server string) bool {
	if server == "" {
		return false
	}
	endpoints := []string{}
	if strings.Contains(server, ";") {
		parts := strings.Split(server, ";")
		for _, part := range parts {
			if strings.Contains(part, "=") {
				subparts := strings.Split(part, "=")
				if len(subparts) == 2 {
					endpoints = append(endpoints, subparts[1])
				}
			} else {
				endpoints = append(endpoints, part)
			}
		}
	} else {
		endpoints = append(endpoints, server)
	}

	for _, ep := range endpoints {
		ep = strings.TrimSpace(ep)
		if ep == "" {
			continue
		}
		conn, err := net.DialTimeout("tcp", ep, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
	}
	return false
}

func testDNSServer(dnsServer string, domain string, localIP string) ([]string, time.Duration, error) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := &net.Dialer{Timeout: 1 * time.Second}
			if localIP != "" {
				if strings.HasPrefix(network, "tcp") {
					if addr, err := net.ResolveTCPAddr("tcp", localIP+":0"); err == nil {
						dialer.LocalAddr = addr
					}
				} else {
					if addr, err := net.ResolveUDPAddr("udp", localIP+":0"); err == nil {
						dialer.LocalAddr = addr
					}
				}
			}
			return dialer.DialContext(ctx, network, dnsServer)
		},
	}
	start := time.Now()
	ips, err := r.LookupHost(context.Background(), domain)
	duration := time.Since(start)
	if err != nil {
		return nil, 0, err
	}
	return ips, duration, nil
}

func testLocalDNS(domain string, localIP string) ([]string, time.Duration, error) {
	if localIP == "" {
		start := time.Now()
		ips, err := net.LookupHost(domain)
		return ips, time.Since(start), err
	}
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := &net.Dialer{Timeout: 1 * time.Second}
			if strings.HasPrefix(network, "tcp") {
				if addr, err := net.ResolveTCPAddr("tcp", localIP+":0"); err == nil {
					dialer.LocalAddr = addr
				}
			} else {
				if addr, err := net.ResolveUDPAddr("udp", localIP+":0"); err == nil {
					dialer.LocalAddr = addr
				}
			}
			return dialer.DialContext(ctx, network, address)
		},
	}
	start := time.Now()
	ips, err := r.LookupHost(context.Background(), domain)
	return ips, time.Since(start), err
}

func diagnoseMTU(report *DiagnosticReport) {
	target := "223.5.5.5"
	sizes := []int{1472, 1464}
	successSize := 0

	for _, size := range sizes {
		ok, err := testPingDF(target, size, report.SelectedAdapterIP)
		if err == nil && ok {
			successSize = size
			break
		}
	}

	if successSize == 0 {
		for size := 1450; size >= 1000; size -= 10 {
			ok, err := testPingDF(target, size, report.SelectedAdapterIP)
			if err == nil && ok {
				successSize = size
				break
			}
		}
	}

	if successSize > 0 {
		report.MaxPayload = successSize
		report.OptimumMTU = successSize + 28

		if report.OptimumMTU == 1500 {
			printColored("success", getT(ResultSuccess), fmt.Sprintf(getT(PMTUOpt), report.OptimumMTU, report.MaxPayload))
		} else if report.OptimumMTU == 1492 {
			printColored("warning", getT(ResultWarning), fmt.Sprintf(getT(PMTUOpt), report.OptimumMTU, report.MaxPayload)+" (Standard PPPoE MTU detected, ensure router TCP MSS clamping is configured)")
		} else {
			printColored("warning", getT(ResultWarning), fmt.Sprintf(getT(PMTUOpt), report.OptimumMTU, report.MaxPayload)+" (Sub-optimal Path MTU detected, potential VPN or customized tunneling)")
		}
	} else {
		report.MTUProbeStatus = "Offline/Unresponsive"
		printColored("danger", getT(ResultDanger), "Path MTU discovery failed. Target 223.5.5.5 is unreachable or blocks ICMP.")
	}
}

func diagnoseHosts(report *DiagnosticReport) {
	hostsPath := getHostsPath()
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		printColored("danger", getT(ResultDanger), "Failed to read hosts file: "+err.Error())
		return
	}

	lines := strings.Split(string(data), "\n")
	activeMappings := 0
	anomaliesCount := 0

	watchedDomains := map[string]bool{
		"baidu.com":         true,
		"qq.com":            true,
		"taobao.com":        true,
		"github.com":        true,
		"google.com":        true,
		"microsoft.com":     true,
		"windowsupdate.com": true,
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			activeMappings++
			ip := fields[0]
			for _, domain := range fields[1:] {
				domainLower := strings.ToLower(domain)
				isWatched := false
				for w := range watchedDomains {
					if domainLower == w || strings.HasSuffix(domainLower, "."+w) {
						isWatched = true
						break
					}
				}
				if isWatched {
					if ip == "127.0.0.1" || ip == "::1" || isPrivateIP(ip) {
						anomaliesCount++
						report.HostsAnomalies = append(report.HostsAnomalies, fmt.Sprintf("%s -> %s", domain, ip))
						printColored("danger", getT(ResultDanger), fmt.Sprintf("Suspicious Static Mapping: %s is mapped to %s", domain, ip))
					}
				}
			}
		}
	}

	report.TotalHosts = activeMappings
	if anomaliesCount > 0 {
		printColored("danger", getT(ResultDanger), fmt.Sprintf(getT(HostsStatus), activeMappings, anomaliesCount))
	} else {
		printColored("success", getT(ResultSuccess), fmt.Sprintf(getT(HostsStatus), activeMappings, 0))
	}
}

func diagnoseDNS(report *DiagnosticReport) {
	domain := "baidu.com"

	localIPs, localDur, localErr := testLocalDNS(domain, report.SelectedAdapterIP)
	_, aliDur, aliErr := testDNSServer("223.5.5.5:53", domain, report.SelectedAdapterIP)
	_, podDur, podErr := testDNSServer("119.29.29.29:53", domain, report.SelectedAdapterIP)

	localSpeedStr := "N/A"
	if localErr == nil {
		localSpeedStr = fmt.Sprintf("%dms", localDur/time.Millisecond)
		report.LocalDNSSpeed = localDur
	}
	aliSpeedStr := "N/A"
	if aliErr == nil {
		aliSpeedStr = fmt.Sprintf("%dms", aliDur/time.Millisecond)
		report.AliDNSSpeed = aliDur
	}
	podSpeedStr := "N/A"
	if podErr == nil {
		podSpeedStr = fmt.Sprintf("%dms", podDur/time.Millisecond)
		report.DNSPodSpeed = podDur
	}

	printColored("info", getT(ResultInfo), fmt.Sprintf(getT(DNSStatus), localSpeedStr, aliSpeedStr, podSpeedStr))

	if localErr != nil {
		report.DNSHijacked = false
		printColored("danger", getT(ResultDanger), "Local DNS resolution failed! Network might be disconnected or DNS server config is invalid.")
	} else {
		hijacked := false
		for _, ip := range localIPs {
			if ip == "127.0.0.1" || ip == "::1" || isPrivateIP(ip) {
				hijacked = true
				break
			}
		}

		if hijacked {
			report.DNSHijacked = true
			printColored("danger", getT(ResultDanger), getT(ResultDanger)+" (DNS hijacking detected! Domain baidu.com resolved to loopback or private IP)")
		} else {
			if localDur > 100*time.Millisecond {
				printColored("warning", getT(ResultWarning), fmt.Sprintf("Local DNS resolution is slow / 本地 DNS 解析延迟过高 (%dms > 100ms)", localDur/time.Millisecond))
			} else {
				printColored("success", getT(ResultSuccess), "Local DNS domain resolution is healthy (non-poisoned IP addresses returned).")
			}
		}
	}
}

