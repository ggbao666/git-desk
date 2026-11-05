package main

// 底层 Git 命令执行与可执行文件探测。
// 逻辑移植自 md0/main/ipc/git.js，行为与返回结构保持一致。

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	probeTimeout   = 10 * time.Second
	statusTimeout  = 20 * time.Second
	fetchTimeout   = 60 * time.Second
	commitTimeout  = 60 * time.Second
	pullTimeout    = 180 * time.Second
	pushTimeout    = 180 * time.Second
	branchTimeout  = 30 * time.Second
	historyTimeout = 30 * time.Second
	maxParentDepth = 8
)

// ---------------------------------------------------------------------------
// 命令执行
// ---------------------------------------------------------------------------

type cmdResult struct {
	Code     int
	Stdout   string
	Stderr   string
	TimedOut bool
}

// capture 运行一条 git 命令并捕获输出。超时会杀掉子进程。
func capture(gitPath string, args []string, cwd string, timeout time.Duration) cmdResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, gitPath, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = append(os.Environ(),
		"GIT_PAGER=cat",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0", // 绝不让 git 弹凭据输入框把 UI 卡死
		"GCM_INTERACTIVE=never",
		"LC_ALL=C", // 让错误信息稳定为英文，便于匹配
	)
	hideConsoleWindow(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := cmdResult{Code: 0, Stdout: stdout.String(), Stderr: stderr.String()}
	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.Code = -1
		result.Stderr += fmt.Sprintf("\n操作超时（%d 秒）", int(timeout.Seconds()))
		return result
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.Code = exitErr.ExitCode()
		} else {
			result.Code = -1
			result.Stderr += "\n" + err.Error()
		}
	}
	return result
}

func (r cmdResult) output() string { return joinOutput(r.Stdout, r.Stderr) }

func firstLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.TrimSuffix(lines[0], "\r"))
}

func joinOutput(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			kept = append(kept, s)
		}
	}
	return strings.Join(kept, "\n")
}

// runParallel 并发执行多条互不依赖的 git 查询（与 md0 的 Promise.all 对应）。
func runParallel(jobs ...func() cmdResult) []cmdResult {
	results := make([]cmdResult, len(jobs))
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func(idx int, fn func() cmdResult) {
			defer wg.Done()
			results[idx] = fn()
		}(i, job)
	}
	wg.Wait()
	return results
}

// ---------------------------------------------------------------------------
// Git 可执行文件探测
// ---------------------------------------------------------------------------

type gitRunner struct {
	Path    string
	Version string
	Source  string // auto | configured
}

var autoDetect struct {
	sync.Mutex
	scanned bool
	path    string
	version string
}

func isWindows() bool { return runtime.GOOS == "windows" }

func dedupePaths(list []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(list))
	for _, item := range list {
		if item == "" {
			continue
		}
		key := item
		if isWindows() {
			key = strings.ToLower(item)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

// pathCandidates 返回 PATH 中可能存在的 git 可执行文件。
func pathCandidates() []string {
	names := []string{"git"}
	if isWindows() {
		names = []string{"git.exe"}
	}
	var out []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		for _, name := range names {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out
}

// commonCandidates 是各平台常见安装位置。
func commonCandidates() []string {
	var out []string
	if isWindows() {
		programFiles := os.Getenv("ProgramFiles")
		programFilesX86 := os.Getenv("ProgramFiles(x86)")
		localAppData := os.Getenv("LOCALAPPDATA")
		userProfile := os.Getenv("USERPROFILE")
		if programFiles != "" {
			out = append(out,
				filepath.Join(programFiles, "Git", "cmd", "git.exe"),
				filepath.Join(programFiles, "Git", "bin", "git.exe"),
				filepath.Join(programFiles, "Git", "mingw64", "bin", "git.exe"),
			)
		}
		if programFilesX86 != "" {
			out = append(out, filepath.Join(programFilesX86, "Git", "cmd", "git.exe"))
		}
		if localAppData != "" {
			out = append(out, filepath.Join(localAppData, "Programs", "Git", "cmd", "git.exe"))
		}
		if userProfile != "" {
			out = append(out,
				filepath.Join(userProfile, "scoop", "apps", "git", "current", "cmd", "git.exe"),
				filepath.Join(userProfile, "Programs", "Git", "cmd", "git.exe"),
			)
		}
	} else {
		out = append(out, "/usr/bin/git", "/usr/local/bin/git", "/opt/homebrew/bin/git", "/opt/local/bin/git")
		if home := os.Getenv("HOME"); home != "" {
			out = append(out, filepath.Join(home, ".local", "bin", "git"))
		}
	}
	return out
}

// expandConfiguredPath 允许用户在设置里填命令名、目录或可执行文件完整路径。
func expandConfiguredPath(raw string) []string {
	value := strings.Trim(strings.TrimSpace(raw), `"'`)
	if value == "" {
		return nil
	}
	if !filepath.IsAbs(value) && !strings.ContainsAny(value, `/\`) {
		var out []string
		for _, candidate := range pathCandidates() {
			base := strings.ToLower(filepath.Base(candidate))
			if base == strings.ToLower(value) || base == strings.ToLower(value)+".exe" {
				out = append(out, candidate)
			}
		}
		return out
	}
	absolute, _ := filepath.Abs(value)
	if stat, err := os.Stat(absolute); err == nil && stat.IsDir() {
		if isWindows() {
			return dedupePaths([]string{
				filepath.Join(absolute, "git.exe"),
				filepath.Join(absolute, "cmd", "git.exe"),
				filepath.Join(absolute, "bin", "git.exe"),
				filepath.Join(absolute, "mingw64", "bin", "git.exe"),
			})
		}
		return dedupePaths([]string{filepath.Join(absolute, "git"), filepath.Join(absolute, "bin", "git")})
	}
	return []string{absolute}
}

func probeGitExe(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	r := capture(path, []string{"--version"}, "", probeTimeout)
	if r.Code == 0 && strings.Contains(strings.ToLower(r.Stdout), "git version") {
		return strings.TrimSpace(r.Stdout), true
	}
	return "", false
}

func detectAuto(refresh bool) *gitRunner {
	autoDetect.Lock()
	defer autoDetect.Unlock()
	if refresh {
		autoDetect.scanned = false
		autoDetect.path = ""
		autoDetect.version = ""
	}
	if autoDetect.scanned {
		if autoDetect.path == "" {
			return nil
		}
		return &gitRunner{Path: autoDetect.path, Version: autoDetect.version, Source: "auto"}
	}
	autoDetect.scanned = true
	all := dedupePaths(append(pathCandidates(), commonCandidates()...))
	for _, candidate := range all {
		if version, ok := probeGitExe(candidate); ok {
			autoDetect.path = candidate
			autoDetect.version = version
			return &gitRunner{Path: candidate, Version: version, Source: "auto"}
		}
	}
	return nil
}

// resolveGitExe 解析当前生效的 git 可执行文件。
func resolveGitExe(configured string) (*gitRunner, string) {
	if strings.TrimSpace(configured) != "" {
		for _, candidate := range expandConfiguredPath(configured) {
			if version, ok := probeGitExe(candidate); ok {
				return &gitRunner{Path: candidate, Version: version, Source: "configured"}, ""
			}
		}
		return nil, "指定的 Git 路径无效：" + strings.TrimSpace(configured)
	}
	if auto := detectAuto(false); auto != nil {
		return auto, ""
	}
	return nil, "未检测到 Git，请在设置中指定 Git 程序路径"
}

// ---------------------------------------------------------------------------
// 仓库定位
// ---------------------------------------------------------------------------

type repoCtx struct {
	git  *gitRunner
	root string
	dir  string
}

func resolveDirectory(dirPath string) string {
	if strings.TrimSpace(dirPath) == "" {
		return ""
	}
	absolute, err := filepath.Abs(dirPath)
	if err != nil {
		return ""
	}
	stat, err := os.Stat(absolute)
	if err != nil || !stat.IsDir() {
		return ""
	}
	return absolute
}

// looksLikeRepo 不依赖 git 命令，粗略判断目录是否在 Git 仓库内。
func looksLikeRepo(dirPath string) bool {
	current := dirPath
	for depth := 0; depth < maxParentDepth; depth++ {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return false
}

// findRepoRoot 从给定目录向上查找包含 .git 的目录（用于拖拽文件进来时定位）。
func findRepoRoot(dirPath string) string {
	current := dirPath
	for depth := 0; depth < 32; depth++ {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

// openRepo 校验目录、解析 git 可执行文件并定位仓库根。
// 返回值：repo, isRepo, errorText
func openRepo(dirPath string, configured string) (*repoCtx, bool, string) {
	dir := resolveDirectory(dirPath)
	if dir == "" {
		return nil, false, "工作空间目录无效"
	}
	if !looksLikeRepo(dir) {
		return nil, false, "当前目录不是 Git 仓库"
	}
	git, errText := resolveGitExe(configured)
	if git == nil {
		return nil, true, errText
	}
	top := capture(git.Path, []string{"rev-parse", "--show-toplevel"}, dir, statusTimeout)
	if top.Code != 0 {
		return nil, true, firstLine(top.Stderr)
	}
	root := strings.TrimSpace(top.Stdout)
	if root == "" {
		root = dir
	} else {
		// git 输出的是 POSIX 风格路径，统一成本机绝对路径
		root = filepath.Clean(filepath.FromSlash(root))
	}
	return &repoCtx{git: git, root: root, dir: dir}, true, ""
}

// ---------------------------------------------------------------------------
// status 解析
// ---------------------------------------------------------------------------

func classifyChange(index, worktree byte) string {
	conflicted := index == 'U' || worktree == 'U' ||
		(index == 'A' && worktree == 'A') ||
		(index == 'D' && worktree == 'D')
	switch {
	case conflicted:
		return "conflict"
	case index == '?' && worktree == '?':
		return "new"
	case index == 'R' || worktree == 'R':
		return "renamed"
	case index == 'C' || worktree == 'C':
		return "copied"
	case index == 'D' || worktree == 'D':
		return "deleted"
	case index == 'A':
		return "new"
	default:
		return "modified"
	}
}

// parseStatusEntries 解析 `git status --porcelain -z`（NUL 分隔，避免路径中的空格/中文被转义）。
func parseStatusEntries(raw string) []GitFile {
	tokens := strings.Split(raw, "\x00")
	files := make([]GitFile, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if len(token) < 4 {
			continue
		}
		index := token[0]
		worktree := token[1]
		filePath := token[3:]
		if filePath == "" {
			continue
		}
		original := ""
		if index == 'R' || index == 'C' || worktree == 'R' || worktree == 'C' {
			if i+1 < len(tokens) {
				original = tokens[i+1]
				i++
			}
		}
		kind := classifyChange(index, worktree)
		files = append(files, GitFile{
			Path:       filePath,
			Original:   original,
			Index:      string(index),
			Worktree:   string(worktree),
			Kind:       kind,
			Conflicted: kind == "conflict",
		})
	}
	return files
}

// listUnmergedPaths 返回所有未解决冲突的路径。
func listUnmergedPaths(gitPath, root string) []string {
	r := capture(gitPath, []string{"ls-files", "-u", "-z"}, root, statusTimeout)
	if r.Code != 0 {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, token := range strings.Split(r.Stdout, "\x00") {
		idx := strings.IndexByte(token, '\t')
		if idx < 0 {
			continue
		}
		p := token[idx+1:]
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}
