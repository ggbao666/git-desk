package main

// .gitignore 编辑器：只管理仓库根目录一层的精确规则，
// 用户手写的注释、通配符、否定规则一律原样保留。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const gitignoreName = ".gitignore"

// IgnoreEntry 是目标目录下一层的一个文件或目录。
type IgnoreEntry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Ignored bool   `json:"ignored"`
}

// IgnoreOverview 是编辑器打开时的完整状态。
type IgnoreOverview struct {
	Dir     string        `json:"dir"`
	Exists  bool          `json:"exists"`
	Entries []IgnoreEntry `json:"entries"`
}

// IgnoreSaveResult 描述一次保存的结果。
type IgnoreSaveResult struct {
	OK      bool   `json:"ok"`
	Created bool   `json:"created"`
	Content string `json:"content"`
}

func cleanDirPath(dir string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(dir), `"`))
}

// listIgnoreCandidates 列出目录一层，排除 .git 与 .gitignore 自身。
func listIgnoreCandidates(dir string) ([]IgnoreEntry, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]IgnoreEntry, 0, len(items))
	for _, it := range items {
		name := it.Name()
		if name == ".git" || name == gitignoreName {
			continue
		}
		out = append(out, IgnoreEntry{Name: name, IsDir: it.IsDir()})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// normalizeIgnoreRule 把规则行与文件名归一化为可比较形式。
func normalizeIgnoreRule(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSuffix(s, "/")
	return strings.ToLower(s)
}

func ignoreRuleLine(name string, isDir bool) string {
	if isDir {
		return "/" + name + "/"
	}
	return "/" + name
}

func readGitignore(dir string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(dir, gitignoreName))
	if err != nil {
		return "", false
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n"), true
}

// GetGitignoreOverview 返回目标目录的文件清单和当前忽略状态。
func (a *App) GetGitignoreOverview(dir string) (IgnoreOverview, error) {
	dir = cleanDirPath(dir)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return IgnoreOverview{}, fmt.Errorf("目录不存在：%s", dir)
	}
	entries, err := listIgnoreCandidates(dir)
	if err != nil {
		return IgnoreOverview{}, err
	}
	content, exists := readGitignore(dir)

	rules := make(map[string]bool)
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "!") {
			continue
		}
		if n := normalizeIgnoreRule(line); n != "" {
			rules[n] = true
		}
	}
	for i := range entries {
		entries[i].Ignored = rules[strings.ToLower(entries[i].Name)]
	}
	return IgnoreOverview{Dir: dir, Exists: exists, Entries: entries}, nil
}

// SaveGitignore 按勾选结果增删精确规则；checked 中的名字必须真实存在于目录里，
// 其余一律忽略，避免前端注入任意规则或路径穿越。
func (a *App) SaveGitignore(dir string, checked []string) (IgnoreSaveResult, error) {
	dir = cleanDirPath(dir)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return IgnoreSaveResult{}, fmt.Errorf("目录不存在：%s", dir)
	}
	candidates, err := listIgnoreCandidates(dir)
	if err != nil {
		return IgnoreSaveResult{}, err
	}
	known := make(map[string]IgnoreEntry, len(candidates))
	for _, e := range candidates {
		known[strings.ToLower(e.Name)] = e
	}

	want := make(map[string]bool)
	for _, name := range checked {
		key := strings.ToLower(strings.Trim(strings.TrimSpace(name), "/"))
		if _, ok := known[key]; ok {
			want[key] = true
		}
	}

	content, exists := readGitignore(dir)
	kept := make([]string, 0, len(strings.Split(content, "\n"))+len(want))
	covered := make(map[string]bool)
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "!") {
			if n := normalizeIgnoreRule(line); n != "" {
				if _, ok := known[n]; ok {
					// 指向候选条目的精确规则：勾选则保留，取消勾选即移除
					if want[n] && !covered[n] {
						kept = append(kept, line)
						covered[n] = true
					}
					continue
				}
			}
		}
		kept = append(kept, line)
	}

	// 新勾选但原本没有规则的条目，追加一行
	for _, e := range candidates {
		key := strings.ToLower(e.Name)
		if want[key] && !covered[key] {
			kept = append(kept, ignoreRuleLine(e.Name, e.IsDir))
		}
	}
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	final := ""
	if len(kept) > 0 {
		final = strings.Join(kept, "\n") + "\n"
	}

	if err := os.WriteFile(filepath.Join(dir, gitignoreName), []byte(final), 0644); err != nil {
		return IgnoreSaveResult{}, err
	}
	return IgnoreSaveResult{OK: true, Created: !exists, Content: final}, nil
}
