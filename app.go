package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是 Wails 暴露给前端的绑定对象。
type App struct {
	ctx         context.Context
	currentDir  string
	startupDir  string
	settings    Settings
}

func NewApp(startupDir string) *App {
	return &App{
		startupDir: startupDir,
		settings:   loadSettings(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 由前端调用 window.runtime.OnFileDrop(..., false) 安装 DOM 拖放监听，
	// Wails 会把拖入的 File 对象解析成真实路径并发 wails:file-drop 事件。
	// 这里在 Go 端统一接收，再转成一个更简单的事件交给前端处理。
	// 用宽松解析兼容 Go 直发的 []string 与前端转发过来的 []interface{}。
	runtime.EventsOn(ctx, "wails:file-drop", func(data ...interface{}) {
		paths := extractDroppedPaths(data...)
		if len(paths) == 0 {
			return
		}
		runtime.EventsEmit(ctx, "app:dropped-paths", paths)
	})
}

// extractDroppedPaths 从 wails:file-drop 的 (x, y, paths) 参数里取出路径列表。
func extractDroppedPaths(data ...interface{}) []string {
	var out []string
	for _, item := range data {
		switch v := item.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				out = append(out, v)
			}
		case []string:
			out = append(out, v...)
		case []interface{}:
			for _, sub := range v {
				if s, ok := sub.(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, s)
				}
			}
		}
	}
	return dedupePaths(out)
}

// ---------------------------------------------------------------------------
// 设置
// ---------------------------------------------------------------------------

func (a *App) GetSettings() Settings { return a.settings }

func (a *App) SaveSettings(s Settings) (Settings, error) {
	if s.Theme != "light" {
		s.Theme = "dark"
	}
	s.GitPath = strings.TrimSpace(s.GitPath)
	a.settings = s
	if err := saveSettingsToDisk(s); err != nil {
		return a.settings, err
	}
	// Git 路径变化后让自动探测缓存失效
	detectAuto(true)
	if a.ctx != nil {
		if s.Theme == "light" {
			runtime.WindowSetLightTheme(a.ctx)
		} else {
			runtime.WindowSetDarkTheme(a.ctx)
		}
	}
	return a.settings, nil
}

// PickGitExecutable 让用户手动选择 git 可执行文件。
func (a *App) PickGitExecutable() (string, error) {
	filters := []runtime.FileFilter{}
	if isWindows() {
		filters = append(filters, runtime.FileFilter{DisplayName: "Git 可执行文件 (*.exe)", Pattern: "*.exe"})
	}
	filters = append(filters, runtime.FileFilter{DisplayName: "所有文件", Pattern: "*.*"})
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "选择 Git 程序",
		Filters: filters,
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

func (a *App) GitDetect(refresh bool) GitDetectResult {
	configured := strings.TrimSpace(a.settings.GitPath)
	if configured != "" {
		git, errText := resolveGitExe(configured)
		if git == nil {
			return GitDetectResult{OK: false, Source: "configured", Error: errText}
		}
		return GitDetectResult{OK: true, Path: git.Path, Version: git.Version, Source: "configured"}
	}
	auto := detectAuto(refresh)
	if auto == nil {
		return GitDetectResult{OK: false, Source: "auto", Error: "未检测到 Git，请手动指定 Git 程序路径"}
	}
	return GitDetectResult{OK: true, Path: auto.Path, Version: auto.Version, Source: "auto"}
}

// ---------------------------------------------------------------------------
// 目录选择 / 打开
// ---------------------------------------------------------------------------

// PickDirectory 弹出系统目录选择对话框，返回用户选择的路径（取消返回空串）。
func (a *App) PickDirectory() (string, error) {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 Git 仓库目录",
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// ResolveDroppedPath 把拖入的路径（文件、子目录、仓库根目录）归一成仓库根目录。
// 拖入仓库内任意文件时会自动向上查找 .git 定位仓库。
func (a *App) ResolveDroppedPath(path string) (string, error) {
	clean := strings.Trim(strings.TrimSpace(path), `"'`)
	if clean == "" {
		return "", errors.New("拖入的路径为空")
	}
	stat, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("无法访问：%s", clean)
	}
	dir := clean
	if !stat.IsDir() {
		dir = filepath.Dir(clean)
	}
	if root := findRepoRoot(dir); root != "" {
		return root, nil
	}
	// 没有 .git 也返回目录本身，交给 OpenDirectory 用 rev-parse 再判一次
	// （兼容 git worktree / submodule 的 .git 文件），并给出可初始化的提示。
	return dir, nil
}

// OpenDirectory 校验目录、定位仓库根、写入历史记录并设为当前工作区。
func (a *App) OpenDirectory(path string) (string, error) {
	dir := resolveDirectory(path)
	if dir == "" {
		return "", fmt.Errorf("目录不存在或不是文件夹：%s", path)
	}
	if root := findRepoRoot(dir); root != "" {
		dir = root
	}
	git, errText := resolveGitExe(a.settings.GitPath)
	if git == nil {
		return "", errors.New(errText)
	}
	top := capture(git.Path, []string{"rev-parse", "--show-toplevel"}, dir, statusTimeout)
	if top.Code != 0 {
		return "", fmt.Errorf("「%s」不是 Git 仓库，可先在设置中初始化或选择其他目录", dir)
	}
	root := strings.TrimSpace(top.Stdout)
	if root == "" {
		root = dir
	} else {
		root = filepath.Clean(filepath.FromSlash(root))
	}
	a.currentDir = root
	addRecentDir(root)
	return root, nil
}

// InitRepo 在选定目录执行 git init，作为「选了普通目录」时的补救入口。
func (a *App) InitRepo(path string) (string, error) {
	dir := resolveDirectory(path)
	if dir == "" {
		return "", fmt.Errorf("目录不存在或不是文件夹：%s", path)
	}
	git, errText := resolveGitExe(a.settings.GitPath)
	if git == nil {
		return "", errors.New(errText)
	}
	r := capture(git.Path, []string{"init"}, dir, statusTimeout)
	if r.Code != 0 {
		return "", fmt.Errorf("初始化失败：%s", firstLine(r.Stderr))
	}
	return a.OpenDirectory(dir)
}

func (a *App) GetCurrentDir() string { return a.currentDir }

func (a *App) GetStartupDir() string { return a.startupDir }

// ---------------------------------------------------------------------------
// 历史打开记录
// ---------------------------------------------------------------------------

func (a *App) GetRecentDirs() []RecentItem { return loadRecentDirs() }

func (a *App) RemoveRecentDir(path string) []RecentItem { return removeRecentDir(path) }

func (a *App) ClearRecentDirs() []RecentItem {
	writeRecentDirs([]RecentItem{})
	return loadRecentDirs()
}

// ---------------------------------------------------------------------------
// 系统交互
// ---------------------------------------------------------------------------

func (a *App) RevealInExplorer(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("路径为空")
	}
	p := filepath.FromSlash(path)
	var cmd *exec.Cmd
	switch {
	case isWindows():
		cmd = exec.Command("explorer.exe", "/select,", p)
	case goruntime.GOOS == "darwin":
		cmd = exec.Command("open", "-R", p)
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(p))
	}
	hideConsoleWindow(cmd)
	return cmd.Start()
}

func (a *App) CopyToClipboard(text string) error {
	if a.ctx == nil {
		return errors.New("窗口尚未就绪")
	}
	return runtime.ClipboardSetText(a.ctx, text)
}
