package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testGitRunner(t *testing.T) *gitRunner {
	t.Helper()
	git, errText := resolveGitExe("")
	if git == nil {
		t.Skipf("本机未检测到 Git，跳过集成测试：%s", errText)
	}
	return git
}

func mustGit(t *testing.T, git *gitRunner, dir string, args ...string) {
	t.Helper()
	r := capture(git.Path, args, dir, statusTimeout)
	if r.Code != 0 {
		t.Fatalf("git %v 失败：%s", args, r.output())
	}
}

func TestDetectGit(t *testing.T) {
	git := testGitRunner(t)
	if git.Path == "" || !strings.Contains(strings.ToLower(git.Version), "git version") {
		t.Fatalf("非法的 Git 探测结果：%+v", git)
	}
	t.Logf("Git: %s (%s)", git.Version, git.Path)
}

func TestParseStatusEntries(t *testing.T) {
	raw := " M a.txt\x00?? b.txt\x00R  c.txt\x00old.txt\x00UU d.txt\x00A  e.txt\x00"
	files := parseStatusEntries(raw)
	if len(files) != 5 {
		t.Fatalf("期望 5 条改动，实际 %d：%+v", len(files), files)
	}
	want := []struct {
		path     string
		kind     string
		original string
		conflict bool
	}{
		{"a.txt", "modified", "", false},
		{"b.txt", "new", "", false},
		{"c.txt", "renamed", "old.txt", false},
		{"d.txt", "conflict", "", true},
		{"e.txt", "new", "", false},
	}
	for i, w := range want {
		got := files[i]
		if got.Path != w.path || got.Kind != w.kind || got.Original != w.original || got.Conflicted != w.conflict {
			t.Errorf("第 %d 条不符：%+v，期望 %+v", i, got, w)
		}
	}
}

func TestRepoWorkflow(t *testing.T) {
	// 隔离用户配置目录，避免测试污染真实的历史记录 / 设置
	t.Setenv("APPDATA", t.TempDir())

	git := testGitRunner(t)
	dir := t.TempDir()

	mustGit(t, git, dir, "init", "-q")
	mustGit(t, git, dir, "config", "user.name", "GitDesk Test")
	mustGit(t, git, dir, "config", "user.email", "test@gitdesk.local")

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.md"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	// 状态：两个未跟踪文件
	status := computeStatus(dir, false, "")
	if !status.OK || !status.IsRepo {
		t.Fatalf("状态读取失败：%+v", status)
	}
	if status.Changes != 2 {
		t.Fatalf("期望 2 个改动，实际 %d：%+v", status.Changes, status.Files)
	}
	if status.Root == "" || status.GitPath == "" {
		t.Fatalf("状态缺少仓库根或 Git 路径：%+v", status)
	}

	// 提交并推送（无远端，应「提交成功 + 推送失败」）
	commit := doCommitPush(dir, "", "first commit", []string{"a.txt", "sub/b.md"})
	if !commit.Committed {
		t.Fatalf("提交未成功：%+v", commit)
	}
	if commit.Pushed {
		t.Fatal("没有远端却报告推送成功")
	}

	// 提交历史
	history := doHistory(dir, "", 30, 0)
	if !history.OK || len(history.Commits) != 1 {
		t.Fatalf("提交历史异常：%+v", history)
	}
	if history.Commits[0].Subject != "first commit" {
		t.Fatalf("提交说明不符：%+v", history.Commits[0])
	}

	// 分支列表
	list := doBranches(dir, "")
	if !list.OK || len(list.Branches) == 0 || list.Branch == "" {
		t.Fatalf("分支列表异常：%+v", list)
	}
	initial := list.Branch

	// 创建 + 切换
	created := doBranchCreate(dir, "", "feature/gitdesk")
	if !created.OK {
		t.Fatalf("创建分支失败：%+v", created)
	}
	if s := computeStatus(dir, false, ""); s.Branch != "feature/gitdesk" {
		t.Fatalf("切换后分支不符：%s", s.Branch)
	}
	switched := doBranchSwitch(dir, "", initial)
	if !switched.OK {
		t.Fatalf("切回分支失败：%+v", switched)
	}
	if invalid := doBranchCreate(dir, "", "bad name"); invalid.OK {
		t.Fatal("含空格的非法分支名应被拒绝")
	}

	// 路径校验（防目录穿越）
	if validRepoPath("../outside.txt") || validRepoPath(`C:\Windows\system32`) {
		t.Fatal("非法仓库内路径未被拦截")
	}
	if !validRepoPath("sub/b.md") {
		t.Fatal("合法仓库内路径被误拦")
	}

	// 拖拽路径归一：从仓库内文件反查仓库根
	filePath := filepath.Join(dir, "sub", "b.md")
	app := &App{settings: DefaultSettings()}
	resolved, err := app.ResolveDroppedPath(filePath)
	if err != nil {
		t.Fatalf("拖拽路径解析失败：%v", err)
	}
	if resolved != filepath.Clean(dir) {
		t.Fatalf("拖拽路径未定位到仓库根：%s，期望 %s", resolved, dir)
	}

	// 打开目录 + 历史记录
	root, err := app.OpenDirectory(dir)
	if err != nil {
		t.Fatalf("打开仓库失败：%v", err)
	}
	if root == "" {
		t.Fatal("打开仓库返回空根目录")
	}
	recent := app.GetRecentDirs()
	if len(recent) == 0 || recent[0].Path != root {
		t.Fatalf("历史记录未写入：%+v", recent)
	}
}
