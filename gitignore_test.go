package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitignoreEditor(t *testing.T) {
	dir := t.TempDir()
	must := func(name string, isDir bool) {
		t.Helper()
		p := filepath.Join(dir, name)
		if isDir {
			if err := os.MkdirAll(p, 0755); err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	must("src", true)
	must("node_modules", true)
	must("README.md", false)
	must(".env", false)
	must(".git", true)

	app := &App{}
	ov, err := app.GetGitignoreOverview(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ov.Exists {
		t.Fatal("新目录不应存在 .gitignore")
	}
	if len(ov.Entries) != 4 {
		t.Fatalf("应列出 4 个候选条目（排除 .git/.gitignore），实际 %d: %+v", len(ov.Entries), ov.Entries)
	}

	created, err := app.SaveGitignore(dir, []string{"node_modules", ".env", "../evil", "not-exist"})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Created || !created.OK {
		t.Fatalf("应自动创建 .gitignore：%+v", created)
	}
	for _, rule := range []string{"/node_modules/", "/.env"} {
		if !strings.Contains(created.Content, rule) {
			t.Fatalf("缺少规则 %q，内容：%q", rule, created.Content)
		}
	}
	if strings.Contains(created.Content, "evil") || strings.Contains(created.Content, "not-exist") {
		t.Fatalf("不应写入非法路径：%q", created.Content)
	}

	// 混入用户手写规则，保存时不得丢失；取消勾选要能删掉精确规则
	custom := "# 自定义注释\n*.log\n/node_modules/\n/.env\n"
	if err := os.WriteFile(filepath.Join(dir, gitignoreName), []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}
	ov2, err := app.GetGitignoreOverview(dir)
	if err != nil {
		t.Fatal(err)
	}
	ignored := map[string]bool{}
	for _, e := range ov2.Entries {
		ignored[e.Name] = e.Ignored
	}
	if !ignored["node_modules"] || !ignored[".env"] || ignored["src"] {
		t.Fatalf("忽略状态识别错误：%+v", ignored)
	}

	res, err := app.SaveGitignore(dir, []string{".env"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created {
		t.Fatal("已存在的文件不应标记为新建")
	}
	for _, keep := range []string{"# 自定义注释", "*.log", "/.env"} {
		if !strings.Contains(res.Content, keep) {
			t.Fatalf("应保留 %q，内容：%q", keep, res.Content)
		}
	}
	if strings.Contains(res.Content, "node_modules") {
		t.Fatalf("取消勾选后应移除规则，内容：%q", res.Content)
	}

	ov3, err := app.GetGitignoreOverview(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ov3.Entries {
		if e.Name == "node_modules" && e.Ignored {
			t.Fatal("node_modules 应恢复为未忽略")
		}
	}
}
