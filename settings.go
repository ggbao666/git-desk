package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings 持久化到用户配置目录。
type Settings struct {
	GitPath string `json:"gitPath"` // 空 = 自动探测
	Theme   string `json:"theme"`   // dark | light
}

func DefaultSettings() Settings {
	return Settings{GitPath: "", Theme: "dark"}
}

func configDir() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		if home, e := os.UserHomeDir(); e == nil {
			return filepath.Join(home, ".GitDesk")
		}
		return ".GitDesk"
	}
	return filepath.Join(cfg, "GitDesk")
}

func settingsFilePath() string { return filepath.Join(configDir(), "settings.json") }

func loadSettings() Settings {
	s := DefaultSettings()
	data, err := os.ReadFile(settingsFilePath())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	if s.Theme != "light" {
		s.Theme = "dark"
	}
	return s
}

func saveSettingsToDisk(s Settings) error {
	if err := os.MkdirAll(configDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsFilePath(), data, 0644)
}
