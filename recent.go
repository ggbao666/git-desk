package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 最近打开的仓库目录，持久化到用户配置目录。
const maxRecentDirs = 12

type RecentItem struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	OpenedAt int64  `json:"openedAt"` // Unix 秒
}

type recentStore struct {
	Items []RecentItem `json:"items"`
}

func recentFilePath() string { return filepath.Join(configDir(), "recent.json") }

func readRecentStore() recentStore {
	data, err := os.ReadFile(recentFilePath())
	if err != nil {
		return recentStore{}
	}
	var store recentStore
	if err := json.Unmarshal(data, &store); err != nil {
		return recentStore{}
	}
	// 兼容早期可能存在的 { dirs: [...] } 格式
	return store
}

// loadRecentDirs 读取列表并过滤掉已经不存在 / 不再是仓库根目录的项。
func loadRecentDirs() []RecentItem {
	store := readRecentStore()
	list := make([]RecentItem, 0, len(store.Items))
	for _, item := range store.Items {
		if item.Path == "" {
			continue
		}
		stat, err := os.Stat(item.Path)
		if err != nil || !stat.IsDir() {
			continue
		}
		if item.Name == "" {
			item.Name = filepath.Base(item.Path)
		}
		list = append(list, item)
	}
	return list
}

func writeRecentDirs(list []RecentItem) {
	if len(list) > maxRecentDirs {
		list = list[:maxRecentDirs]
	}
	if err := os.MkdirAll(configDir(), 0755); err != nil {
		return
	}
	data, err := json.MarshalIndent(recentStore{Items: list}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(recentFilePath(), data, 0644)
}

// addRecentDir 记录一次打开：已存在则提到最前并更新时间。
func addRecentDir(path string) []RecentItem {
	if strings.TrimSpace(path) == "" {
		return loadRecentDirs()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	now := time.Now().Unix()
	cur := loadRecentDirs()
	list := []RecentItem{{Path: abs, Name: filepath.Base(abs), OpenedAt: now}}
	for _, item := range cur {
		if strings.EqualFold(item.Path, abs) {
			continue
		}
		if len(list) >= maxRecentDirs {
			break
		}
		list = append(list, item)
	}
	writeRecentDirs(list)
	return list
}

func removeRecentDir(path string) []RecentItem {
	cur := loadRecentDirs()
	list := make([]RecentItem, 0, len(cur))
	for _, item := range cur {
		if strings.EqualFold(item.Path, path) {
			continue
		}
		list = append(list, item)
	}
	writeRecentDirs(list)
	return list
}
