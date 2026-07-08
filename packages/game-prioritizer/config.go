package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	PollingIntervalMs   int      `json:"polling_interval_ms"`
	BoostPriority       string   `json:"boost_priority"` // "High" or "AboveNormal"
	FuzzyTolerance      int      `json:"fuzzy_tolerance"`
	BoostAnyForeground  bool     `json:"boost_any_foreground"`
	RestoreOnFocusLoss  bool     `json:"restore_on_focus_loss"`
	Blacklist           []string `json:"blacklist"`
	Whitelist           []string `json:"whitelist"`
	SwitchAudioDevice   string   `json:"switch_audio_device"`
}

func DefaultConfig() *Config {
	return &Config{
		PollingIntervalMs:  1000,
		BoostPriority:      "High",
		FuzzyTolerance:     8,
		BoostAnyForeground: false,
		RestoreOnFocusLoss: true,
		SwitchAudioDevice:  "Realtek High Definition Audio",
		Blacklist: []string{
			// Browsers
			"chrome.exe", "msedge.exe", "firefox.exe", "opera.exe", "brave.exe",
			"vivaldi.exe", "safari.exe", "iexplore.exe", "librewolf.exe", "waterfox.exe",
			
			// System components
			"explorer.exe", "taskmgr.exe", "cmd.exe", "powershell.exe", "conhost.exe",
			"searchhost.exe", "startmenuexperiencehost.exe", "shellexperiencehost.exe",
			"dwm.exe", "svchost.exe", "spoolsv.exe", "lsass.exe", "services.exe",
			"wininit.exe", "csrss.exe", "winlogon.exe", "rundll32.exe", "ctfmon.exe",
			"fontdrvhost.exe", "textinputhost.exe", "lockapp.exe", "applicationframehost.exe",

			// IDEs & Development
			"code.exe", "idea64.exe", "clion64.exe", "rider64.exe", "webstorm64.exe",
			"pycharm64.exe", "devenv.exe", "sublime_text.exe", "notepad.exe",
			"notepad++.exe", "goland64.exe", "cursor.exe", "git-credential-manager.exe",

			// Communication
			"discord.exe", "slack.exe", "teams.exe", "wechat.exe", "qq.exe",
			"telegram.exe", "whatsapp.exe", "feishu.exe", "lark.exe", "dingtalk.exe",

			// Game Storefronts & Launchers
			"steam.exe", "steamservice.exe", "steamwebhelper.exe", "epicgameslauncher.exe",
			"galaxyclient.exe", "origin.exe", "eadesktop.exe", "battle.net.exe",
			"uplay.exe", "upc.exe", "wegame.exe", "riotclientux.exe", "riotclientuxrender.exe",

			// Overlays & Utilities
			"obs64.exe", "obs32.exe", "nvidia share.exe", "nvcontainer.exe",
			"amddvr.exe", "discordhookhelper64.exe", "gamebar.exe", "gamebarft.exe",
		},
		Whitelist: []string{
			"dota2.exe", "cs2.exe", "valorant.exe", "league of legends.exe",
			"lolclient.exe", "wow.exe", "minecraft.exe", "javaw.exe",
			"genshinimpact.exe", "hs.exe", "overwatch.exe", "pubg.exe",
			"tarkov.exe", "escapefromtarkov.exe", "starrail.exe", "wuthering waves.exe",
			"client-win64-shipping.exe", "cyberpunk2077.exe", "witcher3.exe",
			"eldenring.exe", "gta5.exe", "rdr2.exe",
		},
	}
}

func LoadConfig() (*Config, error) {
	config := DefaultConfig()

	exePath, err := os.Executable()
	if err != nil {
		return config, err
	}
	configDir := filepath.Dir(exePath)
	configPath := filepath.Join(configDir, "config.json")

	// Fallback to current working directory if executable directory doesn't have config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = "config.json"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Save default config if not found
			saveConfig(configPath, config)
			return config, nil
		}
		return config, err
	}

	err = json.Unmarshal(data, config)
	if err != nil {
		return config, err
	}

	// Normalize whitelist/blacklist to lowercase for case-insensitive matching
	for i, v := range config.Blacklist {
		config.Blacklist[i] = strings.ToLower(v)
	}
	for i, v := range config.Whitelist {
		config.Whitelist[i] = strings.ToLower(v)
	}

	return config, nil
}

func saveConfig(path string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
