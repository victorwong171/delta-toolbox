# 网络卫士 (NetInspect) - 跨平台网络诊断与 Clash 延迟测试工具

`NetInspect` 是一个用 Go 语言编写的轻量级、跨平台网络诊断与配置优化命令行工具。它旨在帮助用户快速定位本地网络故障，并支持对本地 Clash Verge（及其他兼容核心的 GUI 客户端）的所有代理节点进行多目标主机的并发延迟测速。

---

## 🌟 核心特性

1. **一键式网络健康审计**（支持 Windows 与 macOS）：
   * **物理链路与网关**：智能获取网关 IP，测试延迟与丢包，检测以太网卡协商速率。
   * **路径 MTU (PMTU) 探测**：通过带不分片标识（DF）的 ICMP 报文，测算本地最佳 MTU 大小。
   * **NAT 与 Socket 泄露分析**：统计活动网络连接数，检测异常 CLOSE_WAIT 堆积进程，审计多重 NAT/CGNAT。
   * **系统代理与 VPN 检测**：检查系统代理、PAC 脚本及活跃的 L3 TUN/TAP 虚拟网卡代理。
   * **Hosts 安全审计**：扫描静态域名解析，自动识别对主流网站的本地静默重定向。
   * **DNS 健康度与劫持检测**：对比测试本地 DNS、AliDNS、DNSPod 解析速度并验证解析 IP 安全性。
2. **Clash 节点并发测速**（`-clash` 模式）：
   * **配置自动识别**：智能扫描检测 Clash Verge、Clash Verge Rev、Mihomo Party、Clash Nyanpasu 以及 Mihomo 核心的默认配置路径，提取外部控制器端口与 API 密钥。
   * **严格白名单节点清洗**：仅提取具体的后台代理节点（如 Shadowsocks、Vmess、Trojan、Hysteria 等），百分之百清洗并过滤策略组和虚拟系统节点。
   * **全并发与超时防挂起**：支持自定义并发连接数，API 连接引入双重超时保护，防止 Clash 核心卡死导致程序僵死。
   * **中英文 CJK 对齐表格**：精确定算多国字符及 Emoji 在终端的显示宽度，渲染出完美对齐的性能测速报表，并根据延迟区间进行红黄绿彩色分级。
3. **交互式网络故障一键修复**：
   * 支持一键禁用残留/孤立代理配置、还原默认 Hosts 文件、清空本地 DNS 缓存、重新获取 DHCP 网卡 IP 地址等操作。
   * 自动在 Windows/macOS 级别验证 Root/管理员身份，在权限不足时主动引导。

---

## 🛠️ 编译与安装

确保本地已安装 Go (1.20+) 开发环境，克隆项目后在根目录下运行：

### 本地编译

```bash
# 编译本地平台可执行文件
go build -o net_inspect
```

### 跨平台交叉编译

如果您想在一台机器上为其他系统编译二进制包：

```bash
# 编译为 Windows 64位版本
$env:GOOS="windows"; $env:GOARCH="amd64"; go build -o net_inspect.exe

# 编译为 macOS Intel 架构版本
$env:GOOS="darwin"; $env:GOARCH="amd64"; go build -o net_inspect_darwin_amd64

# 编译为 macOS Apple Silicon (M1/M2/M3) 架构版本
$env:GOOS="darwin"; $env:GOARCH="arm64"; go build -o net_inspect_darwin_arm64
```

---

## 🚀 命令行使用指南

直接运行程序会默认进入**网络全面诊断模式**。若传入 `-clash` 参数，则进入 **Clash 全节点延迟测试模式**。

### 1. 网络诊断模式

在普通或管理员终端中直接运行编译后的文件。

```bash
# Windows
.\net_inspect.exe

# macOS (推荐使用 sudo 运行以执行 Hosts 修复和 DHCP 重置)
sudo ./net_inspect
```

*诊断完成后，若存在异常，程序会进入交互修复阶段，请输入 `yes` 确认修复或 `no` 跳过。*

### 2. Clash 节点延迟测试模式

#### A. 基础运行（全自动）
自动探测本地 Clash API，并测试节点对默认测试目标（Google/GitHub）的响应速度：
```bash
net_inspect -clash
```

#### B. 测试特定目标主机
使用 `-clash-hosts` 传递逗号分隔的 URL 列表。测试程序将评估所有节点连接这些目标网站的速度：
```bash
net_inspect -clash -clash-hosts https://github.com,https://www.youtube.com,http://www.gstatic.com/generate_204
```

#### C. 手动指定配置
如果工具未能自动识别到您的 Clash API 端口，或您使用的是远程 Clash/Mihomo 面板，可手动指定 API 地址及 Secret：
```bash
net_inspect -clash -clash-api 127.0.0.1:7897 -clash-secret "your_secret_token_here"
```

#### D. 并发与超时调优
```bash
# 设置 25 个并发测速线程，单个节点超时限制为 2000毫秒
net_inspect -clash -clash-concurrency 25 -clash-timeout 2000
```

---

## 📝 命令行参数一览

| 参数 | 默认值 | 作用说明 |
| :--- | :--- | :--- |
| `-clash` | `false` | 开启 Clash Verge 全节点延迟测试模式 |
| `-clash-hosts` | `http://www.google.com/generate_204,https://www.github.com` | 并发延迟测试的目标主机 URL，以英文逗号分隔 |
| `-clash-api` | `""` | 手动指定 Clash API 控制器地址 (例如 `127.0.0.1:9090`)，默认自动探测 |
| `-clash-secret` | `""` | 手动指定 Clash 控制器的 Authorization Secret，默认自动探测 |
| `-clash-concurrency`| `15` | 并发测速的最大工作协程（goroutine）数 |
| `-clash-timeout` | `3000` | 单个测试请求的超时限制（毫秒） |

---

## 🗂️ 目录结构说明

为了支持跨平台编译与模块化维护，项目采用了平台条件编译架构：

* `main.go`: 程序入口、命令行 Flags 定义、统一的多国语言翻译资源。
* `clash.go`: Clash 运行配置查找、YAML 安全反序列化、并发请求调度与终端对齐展示。
* `diagnose.go`: 平台无关的通用网络诊断定义、数据结构、Ping 输出统计解析。
* `diagnose_windows.go` / `fix_windows.go`: 仅在 Windows 环境下编译，使用 WMI、Registry 等 Windows 系统底层服务。
* `diagnose_darwin.go` / `fix_darwin.go`: 仅在 macOS 环境下编译，使用 `scutil`、`route`、`lsof` 等 Darwin 系统服务。
* `diagnose_others.go` / `fix_others.go`: 用于保证项目在 Linux 或其他未直接对接底层特性的系统下顺利通过编译。
