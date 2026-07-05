package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Leaf proxy types whitelist
var leafProxyTypes = map[string]bool{
	"shadowsocks": true,
	"ss":          true,
	"vmess":       true,
	"vless":       true,
	"trojan":      true,
	"hysteria":    true,
	"hysteria2":   true,
	"tuic":        true,
	"snell":       true,
	"socks5":      true,
	"socks":       true,
	"http":        true,
	"wireguard":   true,
	"ssr":         true,
}

type ProxyDetail struct {
	Type string   `json:"type"`
	All  []string `json:"all"`
}

type ProxiesResponse struct {
	Proxies map[string]ProxyDetail `json:"proxies"`
}

type DelayResponse struct {
	Delay int `json:"delay"`
}

type NodeResult struct {
	Name   string
	Type   string
	Delays map[string]int // target -> delay (ms), -1 if error/timeout
	Avg    float64        // average of successful delays
}

// detectClashConfig attempts to find running Clash/Mihomo GUI core config files
func detectClashConfig() (string, string) {
	var paths []string

	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}

	appData := os.Getenv("APPDATA")

	if appData != "" {
		// Windows paths
		paths = append(paths,
			filepath.Join(appData, "io.github.clash-verge-rev.clash-verge-rev", "config.yaml"),
			filepath.Join(appData, "io.github.clash-verge-rev.clash-verge-rev", "run", "config.yaml"),
			filepath.Join(appData, "clash-verge", "config.yaml"),
			filepath.Join(appData, "clash-verge", "run", "config.yaml"),
			filepath.Join(appData, "mihomo-party", "config.yaml"),
			filepath.Join(appData, "mihomo-party", "run", "config.yaml"),
			filepath.Join(appData, "clash-nyanpasu", "config.yaml"),
			filepath.Join(appData, "clash-nyanpasu", "run", "config.yaml"),
		)
	}

	if home != "" {
		// macOS/Unix paths
		paths = append(paths,
			filepath.Join(home, "Library", "Application Support", "io.github.clash-verge-rev.clash-verge-rev", "config.yaml"),
			filepath.Join(home, "Library", "Application Support", "io.github.clash-verge-rev.clash-verge-rev", "run", "config.yaml"),
			filepath.Join(home, "Library", "Application Support", "clash-verge", "config.yaml"),
			filepath.Join(home, "Library", "Application Support", "clash-verge", "run", "config.yaml"),
			filepath.Join(home, "Library", "Application Support", "mihomo-party", "config.yaml"),
			filepath.Join(home, "Library", "Application Support", "mihomo-party", "run", "config.yaml"),
			filepath.Join(home, "Library", "Application Support", "clash-nyanpasu", "config.yaml"),
			filepath.Join(home, "Library", "Application Support", "clash-nyanpasu", "run", "config.yaml"),
			filepath.Join(home, ".config", "clash", "config.yaml"),
			filepath.Join(home, ".config", "mihomo", "config.yaml"),
		)
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			if controller, secret, err := parseClashConfigYaml(path); err == nil && controller != "" {
				return controller, secret
			}
		}
	}
	return "", ""
}

func parseClashConfigYaml(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var cfg struct {
		ExternalController string `yaml:"external-controller"`
		Secret             string `yaml:"secret"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", "", err
	}
	return cfg.ExternalController, cfg.Secret, nil
}

func fetchProxies(ctx context.Context, apiURL, secret string) (map[string]ProxyDetail, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL+"/proxies", nil)
	if err != nil {
		return nil, err
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	var res ProxiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Proxies, nil
}

func testProxyDelay(ctx context.Context, apiURL, secret, name, targetURL string, timeoutMs int) (int, error) {
	escapedName := url.PathEscape(name)
	query := url.Values{}
	query.Set("timeout", fmt.Sprintf("%d", timeoutMs))
	query.Set("url", targetURL)

	testURL := fmt.Sprintf("%s/proxies/%s/delay?%s", apiURL, escapedName, query.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return 0, err
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	client := &http.Client{
		Timeout: time.Duration(timeoutMs+1000) * time.Millisecond,
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	var res DelayResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, err
	}
	return res.Delay, nil
}

// CJK/Double-width characters visual alignment helpers
func stringWidth(s string) int {
	width := 0
	for _, r := range s {
		if r > 127 {
			width += 2 // Chinese characters / full-width
		} else {
			width += 1
		}
	}
	return width
}

func padRight(s string, length int) string {
	w := stringWidth(s)
	if w >= length {
		return s
	}
	return s + strings.Repeat(" ", length-w)
}

func runClashTest(hostsList, manualAPI, manualSecret string, concurrency, timeoutMs int) {
	fmt.Println("\n\033[36m==================== Clash Verge Latency Test ====================\033[0m")

	apiURL := manualAPI
	secret := manualSecret

	// If not provided manually, auto-detect
	if apiURL == "" {
		fmt.Printf("Searching for Clash configuration... ")
		detectedAPI, detectedSecret := detectClashConfig()
		if detectedAPI != "" {
			apiURL = detectedAPI
			secret = detectedSecret
			fmt.Println("\033[32mFound!\033[0m")
		} else {
			fmt.Println("\033[31mNot found. Falling back to default localhost:9097.\033[0m")
			apiURL = "127.0.0.1:9097"
		}
	}

	// Normalize apiURL scheme
	if !strings.HasPrefix(apiURL, "http://") && !strings.HasPrefix(apiURL, "https://") {
		apiURL = "http://" + apiURL
	}

	maskedSecret := "None"
	if secret != "" {
		if len(secret) > 4 {
			maskedSecret = secret[:2] + "****" + secret[len(secret)-2:]
		} else {
			maskedSecret = "****"
		}
	}

	fmt.Printf("Controller API: \033[33m%s\033[0m | Secret Token: \033[33m%s\033[0m\n", apiURL, maskedSecret)

	// Fetch proxies list
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	proxies, err := fetchProxies(ctx, apiURL, secret)
	cancel()
	if err != nil {
		printColored("danger", "Failed to retrieve proxy list from Clash core API: %v", err)
		printColored("info", "Please make sure Clash / Clash Verge is running and external-controller port is correct.")
		return
	}

	// Filter leaf nodes using whitelist
	var leafProxies []string
	proxyTypes := make(map[string]string)
	for name, detail := range proxies {
		lowerType := strings.ToLower(detail.Type)
		// White-list checking
		if leafProxyTypes[lowerType] {
			// Exclude built-in virtual nodes
			if name == "DIRECT" || name == "REJECT" || name == "GLOBAL" {
				continue
			}
			leafProxies = append(leafProxies, name)
			proxyTypes[name] = detail.Type
		}
	}

	totalNodes := len(leafProxies)
	if totalNodes == 0 {
		printColored("warning", "No backend proxy nodes found in Clash configuration.")
		return
	}

	fmt.Printf("Detected \033[32m%d\033[0m proxy nodes. Parsing target hosts...\n", totalNodes)

	targets := strings.Split(hostsList, ",")
	for i, t := range targets {
		targets[i] = strings.TrimSpace(t)
	}

	fmt.Printf("Target Hosts: \033[35m%s\033[0m\n", strings.Join(targets, " | "))
	fmt.Printf("Testing with concurrency \033[32m%d\033[0m, timeout \033[32m%dms\033[0m...\n\n", concurrency, timeoutMs)

	// Worker queue setup
	type Job struct {
		ProxyName string
		TargetURL string
	}
	type JobResult struct {
		ProxyName string
		TargetURL string
		Delay     int
	}

	jobsCount := len(leafProxies) * len(targets)
	jobs := make(chan Job, jobsCount)
	resultsChan := make(chan JobResult, jobsCount)

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				ctxTest, cancelTest := context.WithTimeout(context.Background(), time.Duration(timeoutMs+500)*time.Millisecond)
				delay, err := testProxyDelay(ctxTest, apiURL, secret, job.ProxyName, job.TargetURL, timeoutMs)
				cancelTest()
				if err != nil {
					resultsChan <- JobResult{ProxyName: job.ProxyName, TargetURL: job.TargetURL, Delay: -1}
				} else {
					resultsChan <- JobResult{ProxyName: job.ProxyName, TargetURL: job.TargetURL, Delay: delay}
				}
			}
		}()
	}

	// Queue all jobs
	for _, name := range leafProxies {
		for _, target := range targets {
			jobs <- Job{ProxyName: name, TargetURL: target}
		}
	}
	close(jobs)

	// Close results channel when workers complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Read and structure results
	nodeResults := make(map[string]*NodeResult)
	for _, name := range leafProxies {
		nodeResults[name] = &NodeResult{
			Name:   name,
			Type:   proxyTypes[name],
			Delays: make(map[string]int),
		}
	}

	for res := range resultsChan {
		nodeResults[res.ProxyName].Delays[res.TargetURL] = res.Delay
	}

	// Calculate average and compile list for sorting
	var list []*NodeResult
	for _, res := range nodeResults {
		sum := 0
		successCount := 0
		for _, delay := range res.Delays {
			if delay >= 0 {
				sum += delay
				successCount++
			}
		}
		if successCount > 0 {
			res.Avg = float64(sum) / float64(successCount)
		} else {
			res.Avg = 999999
		}
		list = append(list, res)
	}

	// Sort results: Successful nodes first (descending), then by average delay (ascending)
	sort.Slice(list, func(i, j int) bool {
		succI := 0
		for _, d := range list[i].Delays {
			if d >= 0 {
				succI++
			}
		}
		succJ := 0
		for _, d := range list[j].Delays {
			if d >= 0 {
				succJ++
			}
		}

		if succI != succJ {
			return succI > succJ
		}
		if list[i].Avg != list[j].Avg {
			return list[i].Avg < list[j].Avg
		}
		return list[i].Name < list[j].Name
	})

	// Print beautiful CJK-aligned table
	nameColWidth := 25
	// Find max proxy name width to dynamically size the name column
	for _, res := range list {
		w := stringWidth(res.Name)
		if w > nameColWidth {
			nameColWidth = w
		}
	}
	if nameColWidth > 45 {
		nameColWidth = 45 // Caps name column size to avoid wrapping issues
	}

	typeColWidth := 12
	targetColWidth := 15

	// Header line
	header := padRight("Proxy Node Name", nameColWidth) + " | " + padRight("Type", typeColWidth)
	for i, t := range targets {
		// Just print hostname / short name
		parsedURL, err := url.Parse(t)
		label := t
		if err == nil && parsedURL.Host != "" {
			label = parsedURL.Host
		}
		if len(label) > targetColWidth {
			label = label[:targetColWidth-3] + "..."
		}
		header += " | " + padRight(label, targetColWidth)
		if i == 0 {
			// Show first target label
		}
	}
	header += " | " + padRight("Average", targetColWidth)

	fmt.Println(header)
	fmt.Println(strings.Repeat("-", len(header)+6))

	for _, res := range list {
		displayName := res.Name
		if stringWidth(displayName) > nameColWidth {
			// Truncate safely
			runes := []rune(displayName)
			displayName = ""
			w := 0
			for _, r := range runes {
				rw := 1
				if r > 127 {
					rw = 2
				}
				if w+rw > nameColWidth-3 {
					displayName += "..."
					break
				}
				displayName += string(r)
				w += rw
			}
		}

		line := padRight(displayName, nameColWidth) + " | " + padRight(res.Type, typeColWidth)
		for _, target := range targets {
			delay := res.Delays[target]
			var delayStr string
			if delay < 0 {
				delayStr = "\033[31mTimeout\033[0m"
			} else if delay < 150 {
				delayStr = fmt.Sprintf("\033[32m%d ms\033[0m", delay)
			} else if delay < 400 {
				delayStr = fmt.Sprintf("\033[33m%d ms\033[0m", delay)
			} else {
				delayStr = fmt.Sprintf("\033[31m%d ms\033[0m", delay)
			}
			// Color codes add 9 characters to output but take 0 terminal columns.
			// Let's pad it carefully by using raw string without color code for sizing.
			rawDelayStr := "Timeout"
			if delay >= 0 {
				rawDelayStr = fmt.Sprintf("%d ms", delay)
			}
			line += " | " + delayStr + strings.Repeat(" ", targetColWidth-stringWidth(rawDelayStr))
		}

		// Average column
		var avgStr string
		if res.Avg >= 999999 {
			avgStr = "\033[31mFailed\033[0m"
		} else if res.Avg < 150 {
			avgStr = fmt.Sprintf("\033[32m%.1f ms\033[0m", res.Avg)
		} else if res.Avg < 400 {
			avgStr = fmt.Sprintf("\033[33m%.1f ms\033[0m", res.Avg)
		} else {
			avgStr = fmt.Sprintf("\033[31m%.1f ms\033[0m", res.Avg)
		}
		rawAvgStr := "Failed"
		if res.Avg < 999999 {
			rawAvgStr = fmt.Sprintf("%.1f ms", res.Avg)
		}
		line += " | " + avgStr + strings.Repeat(" ", targetColWidth-stringWidth(rawAvgStr))

		fmt.Println(line)
	}
	fmt.Println(strings.Repeat("=", len(header)+6))
	fmt.Println("\033[32mLatency test completed!\033[0m")
}
