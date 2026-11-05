package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// .gitignore 编辑器：基于真实 Git 仓库的集成测试
// ---------------------------------------------------------------------------

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// newIgnoreTestRepo 建一个包含已跟踪 / 未跟踪 / 通配符忽略 / 目录忽略的仓库。
func newIgnoreTestRepo(t *testing.T) (*App, *gitRunner, string) {
	t.Helper()
	git := testGitRunner(t)
	dir := t.TempDir()
	mustGit(t, git, dir, "init", "-q")
	mustGit(t, git, dir, "config", "user.name", "GitDesk Test")
	mustGit(t, git, dir, "config", "user.email", "test@gitdesk.local")

	writeTestFile(t, dir, ".gitignore", "# 自定义注释\n*.log\n/build/\n")
	writeTestFile(t, dir, "tracked.txt", "tracked")
	writeTestFile(t, dir, "src/main.go", "package main")
	mustGit(t, git, dir, "add", ".")
	mustGit(t, git, dir, "commit", "-q", "-m", "init")

	writeTestFile(t, dir, "new.txt", "new")         // 未跟踪
	writeTestFile(t, dir, "cache/x.tmp", "tmp")     // 未跟踪目录
	writeTestFile(t, dir, "debug.log", "log")       // 被 *.log 忽略
	writeTestFile(t, dir, "build/out.js", "bundle") // 被 /build/ 忽略
	return &App{}, git, dir
}

func isIgnoredByGit(t *testing.T, git *gitRunner, dir, path string) bool {
	t.Helper()
	r := capture(git.Path, []string{"check-ignore", "--no-index", path}, dir, statusTimeout)
	return r.Code == 0
}

func entryOf(ov IgnoreOverview, name string) (IgnoreEntry, bool) {
	for _, e := range ov.Entries {
		if strings.EqualFold(e.Name, name) {
			return e, true
		}
	}
	return IgnoreEntry{}, false
}

func TestGitignoreOverviewUsesRepoState(t *testing.T) {
	app, git, dir := newIgnoreTestRepo(t)
	ov, err := app.GetGitignoreOverview(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ov.Exists || ov.Root != dir {
		t.Fatalf("仓库定位或 .gitignore 识别错误：%+v", ov)
	}

	cases := []struct {
		name, state string
		ignored     bool
		tracked     bool
	}{
		{"new.txt", "untracked", false, false},
		{"cache", "untracked", false, false},
		{"debug.log", "ignored", true, false},
		{"build", "ignored", true, false},
		{"tracked.txt", "tracked", false, true},
		{"src/main.go", "tracked", false, true},
	}
	for _, c := range cases {
		e, ok := entryOf(ov, c.name)
		if !ok {
			t.Errorf("缺少条目 %s，实际条目：%+v", c.name, ov.Entries)
			continue
		}
		if e.State != c.state || e.Ignored != c.ignored || e.Tracked != c.tracked {
			t.Errorf("%s 状态错误：%+v，期望 state=%s ignored=%v tracked=%v",
				c.name, e, c.state, c.ignored, c.tracked)
		}
	}
	if _, ok := entryOf(ov, ".gitignore"); ok {
		t.Error(".gitignore 自身不应出现在列表里")
	}
	if !isIgnoredByGit(t, git, dir, "debug.log") {
		t.Fatal("测试前置条件失败：debug.log 应被 *.log 忽略")
	}
}

func TestGitignoreAddUntrackedAndTracked(t *testing.T) {
	app, git, dir := newIgnoreTestRepo(t)

	// checked 是「所有要忽略的条目」全量快照；非法路径应被后端丢弃
	res, err := app.SaveGitignore(dir,
		[]string{"build", "debug.log", "new.txt", "tracked.txt", "../evil", "not-exist"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Created {
		t.Fatalf("应更新已存在的 .gitignore：%+v", res)
	}
	for _, keep := range []string{"# 自定义注释", "*.log", "/build/", "/new.txt", "/tracked.txt"} {
		if !strings.Contains(res.Content, keep) {
			t.Errorf("应保留/写入 %q，内容：%q", keep, res.Content)
		}
	}
	if strings.Contains(res.Content, "evil") || strings.Contains(res.Content, "not-exist") {
		t.Fatalf("不应写入非法路径：%q", res.Content)
	}
	if !isIgnoredByGit(t, git, dir, "new.txt") || !isIgnoredByGit(t, git, dir, "tracked.txt") {
		t.Fatal("新规则未被 Git 识别")
	}
	if len(res.Untracked) != 1 || res.Untracked[0] != "tracked.txt" {
		t.Fatalf("已跟踪文件应从索引移除：%+v", res.Untracked)
	}
	if _, err := os.Stat(filepath.Join(dir, "tracked.txt")); err != nil {
		t.Fatal("git rm --cached 不应删除磁盘文件")
	}
	indexed := capture(git.Path, []string{"ls-files"}, dir, statusTimeout)
	if strings.Contains(indexed.Stdout, "tracked.txt") {
		t.Fatalf("tracked.txt 仍在索引中：%s", indexed.Stdout)
	}

	ov, err := app.GetGitignoreOverview(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"new.txt", "tracked.txt"} {
		e, ok := entryOf(ov, name)
		if !ok || !e.Ignored || e.State != "ignored" {
			t.Errorf("%s 应显示为已忽略：%+v", name, e)
		}
	}
}

func TestGitignoreRemoveFromWildcardRule(t *testing.T) {
	app, git, dir := newIgnoreTestRepo(t)

	// 保持 build/ 忽略，把被 *.log 通配符忽略的 debug.log 移出
	res, err := app.SaveGitignore(dir, []string{"build"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "*.log") {
		t.Fatalf("用户自定义通配符规则不得丢失：%q", res.Content)
	}
	if !strings.Contains(res.Content, "!/debug.log") {
		t.Fatalf("移出被通配符忽略的文件应写入否定规则：%q", res.Content)
	}
	if isIgnoredByGit(t, git, dir, "debug.log") {
		t.Fatal("debug.log 应已移出忽略")
	}
	if !isIgnoredByGit(t, git, dir, "build/") {
		t.Fatal("build/ 应保持忽略")
	}

	// 再次勾选 debug.log：否定规则要被移除，且不重复写精确规则
	res2, err := app.SaveGitignore(dir, []string{"build", "debug.log"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res2.Content, "!") {
		t.Fatalf("重新加入后不应残留否定规则：%q", res2.Content)
	}
	if !isIgnoredByGit(t, git, dir, "debug.log") {
		t.Fatal("debug.log 应重新被 *.log 忽略")
	}
}

func TestGitignoreRemoveExactRule(t *testing.T) {
	app, git, dir := newIgnoreTestRepo(t)

	// 取消 build/ 的精确规则，保留 *.log 对 debug.log 的忽略
	res, err := app.SaveGitignore(dir, []string{"debug.log"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Content, "/build/") {
		t.Fatalf("取消勾选应删除精确规则：%q", res.Content)
	}
	if isIgnoredByGit(t, git, dir, "build/") {
		t.Fatal("build/ 应恢复为未忽略（未跟踪）")
	}
	if !isIgnoredByGit(t, git, dir, "debug.log") {
		t.Fatal("debug.log 应仍被 *.log 忽略")
	}
}

func TestGitignoreAutoCreateAndRepoCheck(t *testing.T) {
	git := testGitRunner(t)
	app := &App{}
	dir := t.TempDir()
	mustGit(t, git, dir, "init", "-q")
	writeTestFile(t, dir, "a.txt", "a")

	ov, err := app.GetGitignoreOverview(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ov.Exists {
		t.Fatal("新仓库不应存在 .gitignore")
	}
	res, err := app.SaveGitignore(dir, []string{"a.txt"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || !strings.Contains(res.Content, "/a.txt") {
		t.Fatalf("应自动创建 .gitignore：%+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, gitignoreName)); err != nil {
		t.Fatal("保存后 .gitignore 应存在")
	}

	// 普通目录不是仓库，必须明确报错
	plain := t.TempDir()
	writeTestFile(t, plain, "b.txt", "b")
	if _, err := app.GetGitignoreOverview(plain); err == nil || !strings.Contains(err.Error(), "不是 Git 仓库") {
		t.Fatalf("非仓库目录应报错，实际：%v", err)
	}
	if _, err := app.SaveGitignore(plain, []string{"b.txt"}, false); err == nil {
		t.Fatal("非仓库目录不应允许写入 .gitignore")
	}
}

func childOf(list []IgnoreEntry, name string) (IgnoreEntry, bool) {
	for _, e := range list {
		if strings.EqualFold(e.Name, name) {
			return e, true
		}
	}
	return IgnoreEntry{}, false
}

func TestGitignoreChildrenAreLazyAndNestedRulesWork(t *testing.T) {
	app, git, dir := newIgnoreTestRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "logs", "archive"), 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "src/util/helper.go", "helper") // 未跟踪的深层文件
	writeTestFile(t, dir, "build/bin/app.exe", "bin")     // 被 /build/ 忽略

	// 1) 展开只返回一层子项，且状态来自 Git
	kids, err := app.GetGitignoreChildren(dir, "src")
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := childOf(kids, "src/main.go"); !ok || !e.Tracked || e.State != "tracked" {
		t.Errorf("src/main.go 应为已跟踪文件：%+v / %v", e, ok)
	}
	if e, ok := childOf(kids, "src/util"); !ok || !e.IsDir || e.State != "untracked" {
		t.Errorf("src/util 应为未跟踪目录：%+v / %v", e, ok)
	}
	if _, ok := childOf(kids, "src/util/helper.go"); ok {
		t.Error("展开目录只应返回直接子项，不应递归展开")
	}

	// 2) 被忽略目录里的子项仍标记为「已忽略」
	buildKids, err := app.GetGitignoreChildren(dir, "build")
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := childOf(buildKids, "build/bin"); !ok || !e.IsDir || !e.Ignored {
		t.Errorf("build/bin 应显示为已忽略目录：%+v / %v", e, ok)
	}

	// 3) 非法路径 / 非目录必须拒绝
	for _, bad := range []string{"", "../evil", ".git", "not-exist", "src/main.go"} {
		if _, err := app.GetGitignoreChildren(dir, bad); err == nil {
			t.Errorf("应拒绝无效目录 %q", bad)
		}
	}

	// 4) 已知深层路径要补齐中间目录，前端才能把文件挂到树节点上
	//    （被忽略目录的内部文件不在此列，它们由展开时的按需加载提供）
	ov, err := app.GetGitignoreOverview(dir)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := entryOf(ov, "src/util"); !ok || !e.IsDir || e.State != "untracked" {
		t.Errorf("overview 应补齐 src/util 且继承未跟踪状态：%+v / %v", e, ok)
	}

	// 5) 勾选展开后才出现的子路径要能写出精确规则；臆造路径仍要丢弃
	res, err := app.SaveGitignore(dir,
		[]string{"build", "debug.log", "src/util/helper.go", "logs/archive", "not-exist"}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{"/build/", "*.log", "/src/util/helper.go", "/logs/archive/"} {
		if !strings.Contains(res.Content, rule) {
			t.Errorf("应写入/保留 %q，内容：%q", rule, res.Content)
		}
	}
	if strings.Contains(res.Content, "not-exist") {
		t.Errorf("不应写入不存在的路径：%q", res.Content)
	}
	if !isIgnoredByGit(t, git, dir, "src/util/helper.go") {
		t.Error("嵌套文件规则未被 Git 识别")
	}
	if !isIgnoredByGit(t, git, dir, "logs/archive/") {
		t.Error("空目录规则未被 Git 识别")
	}
}
