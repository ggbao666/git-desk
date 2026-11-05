package main

// .gitignore 编辑器：以 Git 仓库的真实文件状态为数据源。
//
// 列表来自 git ls-files（已跟踪）、git status（未跟踪 / 已修改）、
// git ls-files --ignored（已忽略）以及仓库根目录一层内容；
// 忽略状态用 git check-ignore --no-index 判定，因此 *.log 之类的
// 自定义通配符规则也能被正确识别，取消勾选时会写入 ! 否定规则。

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	gitignoreName    = ".gitignore"
	maxIgnoreEntries = 600
)

// IgnoreEntry 是仓库中的一个文件或目录（路径相对仓库根）。
type IgnoreEntry struct {
	Name    string `json:"name"` // 相对仓库根，目录不带尾斜杠
	IsDir   bool   `json:"isDir"`
	Ignored bool   `json:"ignored"` // 当前是否真的被 Git 忽略
	State   string `json:"state"`   // untracked | ignored | modified | tracked | rule
	Tracked bool   `json:"tracked"` // 是否含已跟踪文件（加入忽略时需要 untrack）
}

// IgnoreOverview 是编辑器打开时的完整状态。
type IgnoreOverview struct {
	Dir       string        `json:"dir"`
	Root      string        `json:"root"`
	Exists    bool          `json:"exists"`
	Entries   []IgnoreEntry `json:"entries"`
	Truncated bool          `json:"truncated"`
}

// IgnoreSaveResult 描述一次保存的结果。
type IgnoreSaveResult struct {
	OK        bool     `json:"ok"`
	Created   bool     `json:"created"`
	Content   string   `json:"content"`
	Added     int      `json:"added"`
	Removed   int      `json:"removed"`
	Untracked []string `json:"untracked"` // 已从 Git 索引移除（磁盘文件保留）
	Warnings  []string `json:"warnings"`
}

type ignoreCandidate struct {
	name      string
	isDir     bool
	tracked   bool
	modified  bool
	untracked bool
}

func cleanDirPath(dir string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(dir), `"`))
}

// normalizeIgnoreRule 把规则行与路径归一化为可比较的小写 key。
func normalizeIgnoreRule(s string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(s), "/"))
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

func nulItems(raw string) []string {
	var out []string
	for _, token := range strings.Split(raw, "\x00") {
		if s := strings.TrimSpace(token); s != "" {
			out = append(out, filepath.ToSlash(s))
		}
	}
	return out
}

// topOf 把 a/b/c.txt 归约为顶层目录 a/（isDir=true）。
func topOf(p string) (string, bool) {
	p = strings.TrimSuffix(filepath.ToSlash(p), "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i], true
	}
	return p, false
}

// safeIgnoreName 拒绝 .git、.gitignore 自身和任何越界路径。
func safeIgnoreName(name string) (string, bool) {
	n := strings.Trim(filepath.ToSlash(name), "/")
	if n == "" || n == ".git" || n == gitignoreName || strings.HasPrefix(n, ".git/") {
		return "", false
	}
	for _, seg := range strings.Split(n, "/") {
		if seg == "." || seg == ".." {
			return "", false
		}
	}
	return n, true
}

// checkIgnoredKeys 批量询问 git：这些路径当前是否被忽略。
// --no-index 让判断只看规则，不受「文件已被跟踪」影响。
func checkIgnoredKeys(repo *repoCtx, candidates map[string]*ignoreCandidate) map[string]bool {
	result := make(map[string]bool, len(candidates))
	if len(candidates) == 0 {
		return result
	}
	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var stdin bytes.Buffer
	for _, key := range keys {
		c := candidates[key]
		p := c.name
		if c.isDir {
			p += "/"
		}
		stdin.WriteString(p)
		stdin.WriteByte(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, repo.git.Path, "check-ignore", "--no-index", "-z", "--stdin")
	cmd.Dir = repo.root
	cmd.Stdin = &stdin
	cmd.Env = append(os.Environ(),
		"GIT_PAGER=cat", "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C",
	)
	hideConsoleWindow(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_ = cmd.Run() // 退出码 1 = 没有任何路径被忽略，属于正常情况

	for _, token := range strings.Split(stdout.String(), "\x00") {
		if key := normalizeIgnoreRule(token); key != "" {
			result[key] = true
		}
	}
	return result
}

// collectIgnoreCandidates 汇总仓库中可管理的文件 / 目录。
func collectIgnoreCandidates(repo *repoCtx) (map[string]*ignoreCandidate, bool) {
	gitPath, root := repo.git.Path, repo.root
	jobs := runParallel(
		func() cmdResult { return capture(gitPath, []string{"ls-files", "-z"}, root, statusTimeout) },
		func() cmdResult {
			return capture(gitPath, []string{"status", "--porcelain", "-z", "--untracked-files=all"}, root, statusTimeout)
		},
		func() cmdResult {
			return capture(gitPath, []string{"status", "--porcelain", "-z", "--untracked-files=normal"}, root, statusTimeout)
		},
		func() cmdResult {
			return capture(gitPath, []string{"ls-files", "--others", "--ignored", "--exclude-standard", "-z", "--directory"}, root, statusTimeout)
		},
	)

	candidates := make(map[string]*ignoreCandidate, 128)
	add := func(raw string, isDir bool) *ignoreCandidate {
		name, ok := safeIgnoreName(raw)
		if !ok {
			return nil
		}
		key := strings.ToLower(name)
		c := candidates[key]
		if c == nil {
			c = &ignoreCandidate{name: name, isDir: isDir}
			candidates[key] = c
		}
		if isDir {
			c.isDir = true
		}
		return c
	}
	markTop := func(p string, untracked bool) {
		if top, isDir := topOf(p); isDir {
			if c := add(top, true); c != nil {
				if untracked {
					c.untracked = true
				} else {
					c.modified = true
					c.tracked = true
				}
			}
		}
	}

	// 1. 已跟踪文件：既有具体路径，也有可整目录管理的顶层条目
	for _, p := range nulItems(jobs[0].Stdout) {
		if c := add(p, false); c != nil {
			c.tracked = true
		}
		if top, isDir := topOf(p); isDir {
			if c := add(top, true); c != nil {
				c.tracked = true
			}
		}
	}

	// 2. 有改动 / 未跟踪的具体文件
	for _, f := range parseStatusEntries(jobs[1].Stdout) {
		p := filepath.ToSlash(f.Path)
		untracked := f.Index == "?" && f.Worktree == "?"
		if c := add(p, strings.HasSuffix(p, "/")); c != nil {
			if untracked {
				c.untracked = true
			} else {
				c.modified = true
				c.tracked = true
			}
		}
		markTop(p, untracked)
	}

	// 3. 未跟踪的顶层目录（--untracked-files=normal 会输出 dir/）
	for _, f := range parseStatusEntries(jobs[2].Stdout) {
		if f.Index == "?" && f.Worktree == "?" {
			p := filepath.ToSlash(f.Path)
			if c := add(p, strings.HasSuffix(p, "/")); c != nil {
				c.untracked = true
			}
		}
	}

	// 4. 当前已被忽略的路径（--directory 只给顶层目录，不展开内部文件）
	for _, p := range nulItems(jobs[3].Stdout) {
		_ = add(p, strings.HasSuffix(p, "/"))
	}

	// 5. 磁盘上仓库根目录的一层内容，保证新建 / 空仓库也有可选项
	if items, err := os.ReadDir(root); err == nil {
		for _, it := range items {
			_ = add(it.Name(), it.IsDir())
		}
	}

	// 6. .gitignore 里已有的精确规则：即使目标已不在磁盘上，也要能取消
	content, exists := readGitignore(root)
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		body := strings.TrimPrefix(t, "!")
		key := normalizeIgnoreRule(body)
		if key == "" || strings.ContainsAny(key, "*?[") {
			continue
		}
		trimmed := strings.TrimSpace(body)
		if c := add(strings.Trim(trimmed, "/"), strings.HasSuffix(trimmed, "/")); c != nil {
			_ = c
		}
	}
	return candidates, exists
}

func buildIgnoreEntries(repo *repoCtx, candidates map[string]*ignoreCandidate) []IgnoreEntry {
	ignored := checkIgnoredKeys(repo, candidates)
	weight := map[string]int{"untracked": 0, "ignored": 1, "modified": 2, "tracked": 3, "rule": 4}
	out := make([]IgnoreEntry, 0, len(candidates))
	for key, c := range candidates {
		isIgnored := ignored[key]
		state := "rule"
		switch {
		case isIgnored:
			state = "ignored"
		case c.untracked:
			state = "untracked"
		case c.modified:
			state = "modified"
		case c.tracked:
			state = "tracked"
		}
		out = append(out, IgnoreEntry{
			Name: c.name, IsDir: c.isDir, Ignored: isIgnored,
			State: state, Tracked: c.tracked || c.modified,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if weight[out[i].State] != weight[out[j].State] {
			return weight[out[i].State] < weight[out[j].State]
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// ignoreRepo 把用户选的目录定位到 Git 仓库根；非仓库直接报错。
func (a *App) ignoreRepo(dir string) (*repoCtx, error) {
	dir = cleanDirPath(dir)
	if dir == "" {
		return nil, fmt.Errorf("请先选择目录")
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("目录不存在：%s", dir)
	}
	if root := findRepoRoot(dir); root != "" {
		dir = root
	}
	git, errText := resolveGitExe(a.settings.GitPath)
	if git == nil {
		return nil, fmt.Errorf("%s", errText)
	}
	top := capture(git.Path, []string{"rev-parse", "--show-toplevel"}, dir, statusTimeout)
	if top.Code != 0 {
		return nil, fmt.Errorf("「%s」不是 Git 仓库，请选择仓库目录（或仓库内任意子目录）", dir)
	}
	root := strings.TrimSpace(top.Stdout)
	if root == "" {
		root = dir
	} else {
		root = filepath.Clean(filepath.FromSlash(root))
	}
	return &repoCtx{git: git, root: root, dir: root}, nil
}

// GetGitignoreOverview 返回仓库文件清单和当前忽略状态。
func (a *App) GetGitignoreOverview(dir string) (IgnoreOverview, error) {
	repo, err := a.ignoreRepo(dir)
	if err != nil {
		return IgnoreOverview{}, err
	}
	candidates, exists := collectIgnoreCandidates(repo)
	entries := buildIgnoreEntries(repo, candidates)
	truncated := false
	if len(entries) > maxIgnoreEntries {
		entries = entries[:maxIgnoreEntries]
		truncated = true
	}
	return IgnoreOverview{
		Dir: repo.root, Root: repo.root, Exists: exists,
		Entries: entries, Truncated: truncated,
	}, nil
}

// managedIgnoreRule 判断一行是否是「针对某个候选条目的精确规则」。
// 注释、通配符、否定规则之外的内容一律视为用户自定义规则原样保留。
func managedIgnoreRule(line string, candidates map[string]*ignoreCandidate) (string, bool, bool) {
	t := strings.TrimSpace(line)
	if t == "" {
		return "", false, false
	}
	negated := strings.HasPrefix(t, "!")
	key := normalizeIgnoreRule(strings.TrimPrefix(t, "!"))
	if key == "" || strings.ContainsAny(key, "*?[") {
		return "", false, false
	}
	if _, ok := candidates[key]; !ok {
		return "", false, false
	}
	return key, negated, true
}

func writeIgnoreFile(root string, lines []string) (string, error) {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	final := ""
	if len(lines) > 0 {
		final = strings.Join(lines, "\n") + "\n"
	}
	if err := os.WriteFile(filepath.Join(root, gitignoreName), []byte(final), 0644); err != nil {
		return "", err
	}
	return final, nil
}

// SaveGitignore 按勾选结果增删忽略规则。
//   - 勾选：加入精确规则（目录带尾斜杠），并移除同名的 ! 否定规则
//   - 取消：删除精确规则；若仍被 *.log 等自定义规则忽略，追加 ! 否定规则
//   - untrackTracked：对本次新加入忽略且已被 Git 跟踪的路径执行
//     git rm --cached（磁盘文件保留，索引标记为删除，随下次提交生效）
func (a *App) SaveGitignore(dir string, checked []string, untrackTracked bool) (IgnoreSaveResult, error) {
	repo, err := a.ignoreRepo(dir)
	if err != nil {
		return IgnoreSaveResult{}, err
	}
	root := repo.root
	candidates, exists := collectIgnoreCandidates(repo)
	beforeIgnored := checkIgnoredKeys(repo, candidates)

	want := make(map[string]bool, len(checked))
	for _, name := range checked {
		key := normalizeIgnoreRule(name)
		if _, ok := candidates[key]; ok {
			want[key] = true
		}
	}
	wantKeys := make([]string, 0, len(want))
	for key := range want {
		wantKeys = append(wantKeys, key)
	}
	sort.Strings(wantKeys)

	content, _ := readGitignore(root)
	// 去掉末尾换行再切分，避免 split 产生的空 token 被夹在新规则中间
	kept := make([]string, 0, len(strings.Split(content, "\n"))+len(want))
	covered := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		if key, negated, ok := managedIgnoreRule(line, candidates); ok {
			// 受管理的精确规则：勾选则最多保留一条正向规则
			if want[key] && !negated && !covered[key] {
				kept = append(kept, line)
				covered[key] = true
			}
			continue
		}
		kept = append(kept, line)
	}

	added, removed := 0, 0
	for _, key := range wantKeys {
		if !beforeIgnored[key] {
			added++
		}
		if beforeIgnored[key] || covered[key] {
			continue // 已被别的规则忽略，或原文件里已有精确规则
		}
		c := candidates[key]
		kept = append(kept, ignoreRuleLine(c.name, c.isDir))
		covered[key] = true
	}
	for key := range beforeIgnored {
		if !want[key] {
			removed++
		}
	}

	final, err := writeIgnoreFile(root, kept)
	if err != nil {
		return IgnoreSaveResult{}, err
	}

	// 写完后重新问一次 git：取消勾选但仍被通配符忽略的路径需要 ! 例外规则
	afterIgnored := checkIgnoredKeys(repo, candidates)
	var negations []string
	for key := range beforeIgnored {
		if want[key] || !afterIgnored[key] {
			continue
		}
		c := candidates[key]
		line := "!/" + c.name
		if c.isDir {
			line += "/"
		}
		negations = append(negations, line)
	}
	if len(negations) > 0 {
		sort.Strings(negations)
		final, err = writeIgnoreFile(root, append(append([]string{}, kept...), negations...))
		if err != nil {
			return IgnoreSaveResult{}, err
		}
	}

	untracked := []string{}
	warnings := []string{}
	if untrackTracked {
		for _, key := range wantKeys {
			if beforeIgnored[key] {
				continue
			}
			c := candidates[key]
			if !c.tracked {
				continue
			}
			r := capture(repo.git.Path,
				[]string{"rm", "--cached", "-r", "--ignore-unmatch", "--quiet", "--", c.name},
				root, statusTimeout)
			if r.Code == 0 {
				untracked = append(untracked, c.name)
			} else {
				warnings = append(warnings, fmt.Sprintf("%s：%s", c.name, firstLine(joinOutput(r.Stderr, r.Stdout))))
			}
		}
	}

	return IgnoreSaveResult{
		OK: true, Created: !exists, Content: final,
		Added: added, Removed: removed,
		Untracked: untracked, Warnings: warnings,
	}, nil
}
