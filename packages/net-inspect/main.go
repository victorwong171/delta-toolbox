package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

type Lang string

const (
	ZH Lang = "zh-CN"
	EN Lang = "en-US"
)

var currentLang Lang = EN

type TransKey int

const (
	Title TransKey = iota
	SystemInfo
	LocDetected
	PressAnyKey
	CategoryLink
	CategoryMTU
	CategoryNAT
	CategoryProxy
	CategoryHosts
	CategoryDNS
	Scanning
	ScanComplete
	ResultSuccess
	ResultWarning
	ResultDanger
	ResultInfo
	GatewayPing
	LinkSpeed
	PMTUOpt
	NATConnections
	CLOSEWAITLeak
	DoubleNAT
	ProxyStatus
	L3ProxyStatus
	HostsStatus
	DNSStatus
	FixPrompt
	FixSuccess
	FixFailed
	AdminRequired
	FixConfirmHint
	FixSkipped
)

var translations = map[Lang]map[TransKey]string{
	ZH: {
		Title:          "==================== 网络卫士 (NetInspect) ====================",
		SystemInfo:     "系统语言: %s | 诊断开始时间: %s",
		LocDetected:    "系统语言设为中文 (zh-CN)。",
		PressAnyKey:    "\n诊断结束，请按回车键退出...",
		CategoryLink:   "[1] 物理链路与网关测试",
		CategoryMTU:    "[2] 路径 MTU (PMTU) 探测",
		CategoryNAT:    "[3] NAT 会话与套接字分析",
		CategoryProxy:  "[4] 代理与拦截器检测",
		CategoryHosts:  "[5] Hosts 文件审计",
		CategoryDNS:    "[6] DNS 健康度与劫持检测",
		Scanning:       "正在扫描 [%s]...",
		ScanComplete:   "所有诊断已完成！",
		ResultSuccess:  " [✔] 正常: %s",
		ResultWarning:  " [⚠] 警告: %s",
		ResultDanger:   " [✘] 严重: %s",
		ResultInfo:     " [i] 提示: %s",
		GatewayPing:    "网关 IP: %s, 延迟: %s, 丢包率: %s",
		LinkSpeed:      "网口名称: %s, 协商速率: %s, 状态: %s",
		PMTUOpt:        "最佳路径 MTU: %d 字节 (最大载荷: %d 字节)",
		NATConnections: "活动连接数: %d (TCP: %d, UDP: %d)",
		CLOSEWAITLeak:  "进程 %s (PID: %d) 存在异常 CLOSE_WAIT 堆积 (%d 个)",
		DoubleNAT:      "检测到多重 NAT / CGNAT (第一跳: %s, 第二跳: %s, 第三跳: %s)",
		ProxyStatus:    "系统代理: %s, 代理地址: %s, PAC配置: %s",
		L3ProxyStatus:  "检测到活跃的 L3 虚拟网卡代理/VPN: %s (%s)",
		HostsStatus:    "Hosts 文件共 %d 条有效映射，异常条目: %d",
		DNSStatus:      "解析延迟: 本地DNS %s | AliDNS %s | DNSPod %s",
		FixPrompt:      "--> 是否尝试修复此问题？[%s] (yes/no): ",
		FixSuccess:     "修复成功: %s",
		FixFailed:      "修复失败: %s",
		AdminRequired:  "此操作需要管理员权限！请右键以管理员身份运行此程序。",
		FixConfirmHint: "请输入 yes 或 no 确认！",
		FixSkipped:     "已跳过此项修复。",
	},
	EN: {
		Title:          "==================== NetInspect Diagnostics ====================",
		SystemInfo:     "System Language: %s | Diagnostic Time: %s",
		LocDetected:    "System language set to English (en-US).",
		PressAnyKey:    "\nDiagnostics finished, press enter to exit...",
		CategoryLink:   "[1] Physical Link & Gateway Diagnostic",
		CategoryMTU:    "[2] Path MTU (PMTU) Discovery",
		CategoryNAT:    "[3] NAT Session & Socket Analysis",
		CategoryProxy:  "[4] Proxy & Interceptor Diagnostic",
		CategoryHosts:  "[5] Hosts File Auditor",
		CategoryDNS:    "[6] DNS Health & Hijacking Audit",
		Scanning:       "Scanning [%s]...",
		ScanComplete:   "All diagnostics completed!",
		ResultSuccess:  " [✔] Normal: %s",
		ResultWarning:  " [⚠] Warning: %s",
		ResultDanger:   " [✘] Danger: %s",
		ResultInfo:     " [i] Info: %s",
		GatewayPing:    "Gateway IP: %s, Latency: %s, Packet Loss: %s",
		LinkSpeed:      "Adapter: %s, Negotiation Speed: %s, Status: %s",
		PMTUOpt:        "Optimal Path MTU: %d bytes (Max Payload: %d bytes)",
		NATConnections: "Active connections: %d (TCP: %d, UDP: %d)",
		CLOSEWAITLeak:  "Process %s (PID: %d) has CLOSE_WAIT socket accumulation (%d connections)",
		DoubleNAT:      "Double NAT or CGNAT detected (Hop 1: %s, Hop 2: %s, Hop 3: %s)",
		ProxyStatus:    "System Proxy: %s, Server: %s, PAC Url: %s",
		L3ProxyStatus:  "Active L3 TUN/TAP Proxy/VPN detected: %s (%s)",
		HostsStatus:    "Hosts File: %d active mappings, %d anomalies",
		DNSStatus:      "Resolution Latency: Local DNS %s | AliDNS %s | DNSPod %s",
		FixPrompt:      "--> Do you want to fix this? [%s] (yes/no): ",
		FixSuccess:     "Fix succeeded: %s",
		FixFailed:      "Fix failed: %s",
		AdminRequired:  "This action requires Administrator privileges! Please re-run as Administrator.",
		FixConfirmHint: "Please enter yes or no to confirm!",
		FixSkipped:     "Skipped this fix.",
	},
}

func getT(key TransKey) string {
	return translations[currentLang][key]
}

func detectLanguage() {
	if detectLanguagePlatform() {
		currentLang = ZH
		return
	}
	langEnv := os.Getenv("LANG")
	if strings.Contains(strings.ToLower(langEnv), "zh") {
		currentLang = ZH
		return
	}
	currentLang = EN
}

func printColored(level string, format string, args ...interface{}) {
	var color string
	switch level {
	case "success":
		color = "\033[32m" // Green
	case "warning":
		color = "\033[33m" // Yellow
	case "danger":
		color = "\033[31m" // Red
	case "info":
		color = "\033[36m" // Cyan
	default:
		color = "\033[0m"
	}
	reset := "\033[0m"
	fmt.Printf("%s"+format+"%s\n", append([]interface{}{color}, append(args, reset)...)...)
}

func main() {
	// CLI Flag parsing
	clashMode := flag.Bool("clash", false, "Enable Clash Verge all-node latency testing mode / 开启 Clash Verge 全节点延迟测试模式")
	clashHosts := flag.String("clash-hosts", "http://www.google.com/generate_204,https://www.github.com", "Comma-separated target hosts/URLs for latency testing / 逗号分隔的延迟测试目标 URL")
	clashAPI := flag.String("clash-api", "", "Manual Clash controller URL (e.g. http://127.0.0.1:7897) / 手动指定 Clash API 控制器地址")
	clashSecret := flag.String("clash-secret", "", "Manual Clash controller secret / 手动指定 Clash 密钥")
	clashConcurrency := flag.Int("clash-concurrency", 15, "Max concurrent latency tests / 最大并发测试协程数")
	clashTimeout := flag.Int("clash-timeout", 3000, "Timeout for each latency test in ms / 每个测试的超时时间 (毫秒)")

	flag.Parse()

	enableAnsiColor()
	detectLanguage()

	// If Clash mode is requested, run it and exit
	if *clashMode {
		runClashTest(*clashHosts, *clashAPI, *clashSecret, *clashConcurrency, *clashTimeout)
		return
	}

	fmt.Println(getT(Title))
	fmt.Printf(getT(SystemInfo)+"\n", string(currentLang), time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(strings.Repeat("-", 60))

	report := &DiagnosticReport{}

	// Run Diagnostics
	runDiagnosticStep(CategoryLink, func() {
		diagnoseLink(report)
	})

	runDiagnosticStep(CategoryMTU, func() {
		diagnoseMTU(report)
	})

	runDiagnosticStep(CategoryNAT, func() {
		diagnoseNAT(report)
	})

	runDiagnosticStep(CategoryProxy, func() {
		diagnoseProxy(report)
	})

	runDiagnosticStep(CategoryHosts, func() {
		diagnoseHosts(report)
	})

	runDiagnosticStep(CategoryDNS, func() {
		diagnoseDNS(report)
	})

	fmt.Println("\n" + getT(ScanComplete))
	fmt.Println(strings.Repeat("=", 60))

	// Remediations Interactive Phase
	scanner := bufio.NewScanner(os.Stdin)
	runFixes(report, scanner)

	fmt.Println(getT(PressAnyKey))
	_, _ = os.Stdin.Read(make([]byte, 1))
}

func runDiagnosticStep(catKey TransKey, fn func()) {
	fmt.Printf(getT(Scanning)+"\r", getT(catKey))
	fn()
	// Clear the scanning line and print the category header
	fmt.Printf("\r\033[K%s\n", getT(catKey))
}
