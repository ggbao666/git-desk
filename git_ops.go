package main

// Git 业务操作：状态、拉取、推送、提交、冲突、分支、历史。
// 逐函数移植自 md0/main/ipc/git.js。

import (
	"regexp"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// 仓库引用（分支 / 远端 / 上游）
// ---------------------------------------------------------------------------

type repoRefs struct {
	branch   string
	remote   string
	upstream string
}

func readRepoRefs(gitPath, root string) repoRefs {
	results := runParallel(
		func() cmdResult { return capture(gitPath, []string{"rev-parse", "--abbrev-ref", "HEAD"}, root, statusTimeout) },
		func() cmdResult { return capture(gitPath, []string{"config", "--get", "remote.origin.url"}, root, statusTimeout) },
		func() cmdResult {
			return capture(gitPath, []string{"rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"}, root, statusTimeout)
		},
	)
	branchResult, originResult, upstreamResult := results[0], results[1], results[2]

	branch := "（尚无提交）"
	if branchResult.Code == 0 {
		branch = strings.TrimSpace(branchResult.Stdout)
	}
	upstream := ""
	if upstreamResult.Code == 0 {
		upstream = strings.TrimSpace(upstreamResult.Stdout)
	}
	remote := ""
	if originResult.Code == 0 {
		remote = strings.TrimSpace(originResult.Stdout)
	}

	// 没有 origin 时，尝试从上游分支或第一个远端取地址
	if remote == "" {
		remoteName := ""
		if upstream != "" {
			remoteName = strings.SplitN(upstream, "/", 2)[0]
		}
		if remoteName == "" {
			list := capture(gitPath, []string{"remote"}, root, statusTimeout)
			if list.Code == 0 {
				if first := firstLine(list.Stdout); first != "" {
					remoteName = first
				}
			}
		}
		if remoteName != "" {
			url := capture(gitPath, []string{"remote", "get-url", remoteName}, root, statusTimeout)
			if url.Code == 0 {
				remote = strings.TrimSpace(url.Stdout)
			}
		}
	}
	return repoRefs{branch: branch, remote: remote, upstream: upstream}
}

// ---------------------------------------------------------------------------
// 仓库状态
// ---------------------------------------------------------------------------

func computeStatus(root string, fetch bool, configured string) *GitStatus {
	repo, isRepo, errText := openRepo(root, configured)
	if repo == nil {
		return &GitStatus{OK: false, IsRepo: isRepo, Files: []GitFile{}, Error: errText}
	}

	if fetch {
		fetched := capture(repo.git.Path, []string{"fetch", "--quiet", "--prune"}, repo.root, fetchTimeout)
		if fetched.Code != 0 {
			errText = firstLine(fetched.Stderr)
			if errText == "" {
				errText = "无法从远端刷新仓库信息"
			}
		}
	}

	refs := readRepoRefs(repo.git.Path, repo.root)
	parallel := runParallel(
		func() cmdResult {
			return capture(repo.git.Path, []string{"status", "--porcelain", "-z", "--untracked-files=all"}, repo.root, statusTimeout)
		},
		func() cmdResult {
			return capture(repo.git.Path, []string{"rev-parse", "-q", "--verify", "MERGE_HEAD"}, repo.root, statusTimeout)
		},
	)
	porcelain, mergeHead := parallel[0], parallel[1]

	files := parseStatusEntries(porcelain.Stdout)
	conflicted := 0
	for _, f := range files {
		if f.Conflicted {
			conflicted++
		}
	}

	ahead, behind := 0, 0
	if refs.upstream != "" {
		counts := capture(repo.git.Path, []string{"rev-list", "--left-right", "--count", "@{u}...HEAD"}, repo.root, statusTimeout)
		if counts.Code == 0 {
			parts := strings.Fields(strings.TrimSpace(counts.Stdout))
			if len(parts) >= 2 {
				behind, _ = strconv.Atoi(parts[0])
				ahead, _ = strconv.Atoi(parts[1])
			}
		}
	}

	return &GitStatus{
		OK:              true,
		IsRepo:          true,
		Root:            repo.root,
		Workspace:       repo.dir,
		Branch:          refs.branch,
		Remote:          refs.remote,
		Upstream:        refs.upstream,
		Ahead:           ahead,
		Behind:          behind,
		Changes:         len(files),
		Conflicted:      conflicted,
		MergeInProgress: mergeHead.Code == 0,
		Files:           files,
		GitPath:         repo.git.Path,
		GitVersion:      repo.git.Version,
		GitSource:       repo.git.Source,
		FetchError:      errText,
	}
}

// ---------------------------------------------------------------------------
// 拉取
// ---------------------------------------------------------------------------

func doPull(root, configured string) *GitActionResult {
	repo, isRepo, errText := openRepo(root, configured)
	if repo == nil {
		_ = isRepo
		return failedAction(errText)
	}
	// 允许分支分叉时自动合并：冲突保留在仓库里，由用户选择处理方式
	args := []string{"pull", "--no-rebase", "--no-edit"}
	r := capture(repo.git.Path, args, repo.root, pullTimeout)
	out := r.output()
	conflicted := regexp.MustCompile(`(?i)CONFLICT|Automatic merge failed`).MatchString(out)
	res := &GitActionResult{
		OK:         r.Code == 0,
		Root:       repo.root,
		Code:       r.Code,
		Conflicted: conflicted,
		Output:     out,
	}
	if r.Code != 0 {
		res.Error = out
		if res.Error == "" {
			res.Error = "拉取失败"
		}
	}
	return res
}

// ---------------------------------------------------------------------------
// 推送
// ---------------------------------------------------------------------------

type pushTarget struct {
	args        []string
	setUpstream bool
	upstream    string
	err         string
}

func describePushArgs(gitPath, root string) pushTarget {
	up := capture(gitPath, []string{"rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"}, root, statusTimeout)
	if up.Code == 0 && strings.TrimSpace(up.Stdout) != "" {
		return pushTarget{args: []string{"push"}, upstream: strings.TrimSpace(up.Stdout)}
	}
	branchResult := capture(gitPath, []string{"rev-parse", "--abbrev-ref", "HEAD"}, root, statusTimeout)
	branch := ""
	if branchResult.Code == 0 {
		branch = strings.TrimSpace(branchResult.Stdout)
	}
	if branch == "" || branch == "HEAD" {
		return pushTarget{err: "当前分支无法推送（尚无提交或处于游离 HEAD）"}
	}

	remote := ""
	origin := capture(gitPath, []string{"config", "--get", "remote.origin.url"}, root, statusTimeout)
	if origin.Code == 0 {
		remote = "origin"
	}
	if remote == "" {
		list := capture(gitPath, []string{"remote"}, root, statusTimeout)
		if list.Code == 0 {
			remote = firstLine(list.Stdout)
		}
	}
	if remote == "" {
		return pushTarget{err: "未配置远端仓库，无法推送"}
	}
	return pushTarget{args: []string{"push", "-u", remote, branch}, setUpstream: true}
}

func pushRepo(gitPath, root string) *GitActionResult {
	target := describePushArgs(gitPath, root)
	if target.err != "" {
		return failedAction(target.err)
	}
	r := capture(gitPath, target.args, root, pushTimeout)
	out := r.output()
	res := &GitActionResult{OK: r.Code == 0, Root: root, Code: r.Code, Output: out}
	if r.Code != 0 {
		res.Error = out
		if res.Error == "" {
			res.Error = "推送失败"
		}
	}
	return res
}

func doPush(root, configured string) *GitActionResult {
	repo, _, errText := openRepo(root, configured)
	if repo == nil {
		return failedAction(errText)
	}
	return pushRepo(repo.git.Path, repo.root)
}

// ---------------------------------------------------------------------------
// 提交并推送
// ---------------------------------------------------------------------------

func validRepoPath(value string) bool {
	if strings.TrimSpace(value) == "" || strings.HasPrefix(value, "/") {
		return false
	}
	if isWindows() && filepathIsAbs(value) {
		return false
	}
	for _, segment := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
		if segment == ".." {
			return false
		}
	}
	return true
}

func doCommitPush(root, configured, message string, files []string) *GitCommitResult {
	repo, _, errText := openRepo(root, configured)
	if repo == nil {
		return failedCommit(errText)
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return failedCommit("请填写提交说明")
	}
	var clean []string
	for _, f := range files {
		if validRepoPath(f) {
			clean = append(clean, f)
		}
	}
	if len(clean) == 0 {
		return failedCommit("请选择要提交的文件")
	}

	added := capture(repo.git.Path, append([]string{"add", "-A", "--"}, clean...), repo.root, statusTimeout)
	if added.Code != 0 {
		return &GitCommitResult{OK: false, Root: repo.root, Output: added.output(), Error: firstLine(added.Stderr)}
	}
	staged := capture(repo.git.Path, []string{"diff", "--cached", "--name-only"}, repo.root, statusTimeout)
	if staged.Code != 0 {
		return &GitCommitResult{OK: false, Root: repo.root, Error: "无法读取暂存区状态"}
	}
	if strings.TrimSpace(staged.Stdout) == "" {
		return &GitCommitResult{OK: false, Root: repo.root, Error: "选中的文件没有可提交的改动"}
	}

	commit := capture(repo.git.Path, []string{"commit", "-m", message}, repo.root, commitTimeout)
	commitOutput := commit.output()
	if commit.Code != 0 {
		return &GitCommitResult{OK: false, Root: repo.root, Output: commitOutput, Error: firstLine(commit.Stderr)}
	}
	pushed := pushRepo(repo.git.Path, repo.root)
	res := &GitCommitResult{
		OK:        pushed.OK,
		Committed: true,
		Pushed:    pushed.OK,
		Root:      repo.root,
		Output:    joinOutput(commitOutput, pushed.Output),
	}
	if !pushed.OK {
		reason := pushed.Error
		if reason == "" {
			reason = "推送失败"
		}
		res.Error = "提交成功，但推送失败：" + reason
	}
	return res
}

// ---------------------------------------------------------------------------
// 冲突处理
// ---------------------------------------------------------------------------

func doResolveConflict(root, configured, mode string) *GitActionResult {
	repo, _, errText := openRepo(root, configured)
	if repo == nil {
		return failedAction(errText)
	}
	if mode != "ours" && mode != "theirs" && mode != "abort" {
		return failedAction("未知的冲突处理方式")
	}
	gitPath := repo.git.Path

	if mode == "abort" {
		mergeHead := capture(gitPath, []string{"rev-parse", "-q", "--verify", "MERGE_HEAD"}, repo.root, statusTimeout)
		var r cmdResult
		if mergeHead.Code == 0 {
			r = capture(gitPath, []string{"merge", "--abort"}, repo.root, statusTimeout)
		} else {
			r = capture(gitPath, []string{"rebase", "--abort"}, repo.root, statusTimeout)
		}
		out := r.output()
		res := &GitActionResult{OK: r.Code == 0, Root: repo.root, Code: r.Code, Output: out}
		if r.Code != 0 {
			res.Error = out
			if res.Error == "" {
				res.Error = "当前没有可放弃的合并或变基"
			}
		}
		return res
	}

	paths := listUnmergedPaths(gitPath, repo.root)
	if len(paths) == 0 {
		return failedAction("当前没有冲突文件")
	}
	flag := "--ours"
	if mode == "theirs" {
		flag = "--theirs"
	}
	var logs []string
	for _, p := range paths {
		checkedOut := capture(gitPath, []string{"checkout", flag, "--", p}, repo.root, statusTimeout)
		if checkedOut.Code == 0 {
			staged := capture(gitPath, []string{"add", "--", p}, repo.root, statusTimeout)
			if staged.Code != 0 {
				logs = append(logs, firstLine(staged.Stderr))
			}
			continue
		}
		// 所选一侧没有该文件（一侧删除）：按删除处理，完成冲突解决
		removed := capture(gitPath, []string{"rm", "-f", "--", p}, repo.root, statusTimeout)
		if removed.Code != 0 {
			logs = append(logs, firstLine(removed.Stderr))
		}
	}

	// 合并状态下补一次合并提交，让工作区回到正常状态
	mergeHead := capture(gitPath, []string{"rev-parse", "-q", "--verify", "MERGE_HEAD"}, repo.root, statusTimeout)
	if mergeHead.Code == 0 {
		committed := capture(gitPath, []string{"commit", "--no-edit"}, repo.root, commitTimeout)
		logs = append(logs, committed.output())
		if committed.Code != 0 {
			return &GitActionResult{OK: false, Root: repo.root, Output: joinOutput(logs...), Error: firstLine(committed.Stderr)}
		}
	}
	return &GitActionResult{OK: true, Root: repo.root, Output: joinOutput(logs...)}
}

// ---------------------------------------------------------------------------
// 分支管理
// ---------------------------------------------------------------------------

var branchBadChars = regexp.MustCompile(`[~\^:?*\[\]\\\x00-\x1f]`)

func validateBranchName(raw string) (string, string) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", "请填写分支名称"
	}
	if len(name) > 200 {
		return "", "分支名称过长"
	}
	if strings.ContainsAny(name, " \t\n\r") {
		return "", "分支名称不能包含空格"
	}
	if branchBadChars.MatchString(name) {
		return "", "分支名称包含非法字符"
	}
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "/") ||
		strings.HasSuffix(name, ".") || strings.HasSuffix(name, "/") {
		return "", "分支名称不能以 - . / 开头或结尾"
	}
	if strings.Contains(name, "..") || strings.Contains(name, "//") || strings.Contains(name, "@{") {
		return "", "分支名称格式无效"
	}
	if strings.HasSuffix(name, ".lock") {
		return "", "分支名称格式无效"
	}
	return name, ""
}

// readBranches 读取本地分支列表与当前分支（尚未提交的仓库也能拿到当前分支名）。
func readBranches(gitPath, root string) (string, []GitBranch) {
	results := runParallel(
		func() cmdResult { return capture(gitPath, []string{"symbolic-ref", "--short", "-q", "HEAD"}, root, statusTimeout) },
		func() cmdResult {
			return capture(gitPath, []string{"for-each-ref", "--format=%(refname:short)%09%(HEAD)%09%(upstream:short)", "refs/heads"}, root, statusTimeout)
		},
	)
	current := ""
	if results[0].Code == 0 {
		current = strings.TrimSpace(results[0].Stdout)
	}
	branches := []GitBranch{}
	if results[1].Code == 0 {
		for _, line := range strings.Split(results[1].Stdout, "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.TrimSpace(line) == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) == 0 || parts[0] == "" {
				continue
			}
			upstream := ""
			if len(parts) > 2 {
				upstream = strings.TrimSpace(parts[2])
			}
			branches = append(branches, GitBranch{
				Name:     parts[0],
				Current:  len(parts) > 1 && parts[1] == "*",
				Upstream: upstream,
			})
		}
	}
	// 仓库还没有提交时，当前分支尚未出现在 refs/heads 中
	if current != "" {
		found := false
		for _, b := range branches {
			if b.Name == current {
				found = true
				break
			}
		}
		if !found {
			branches = append([]GitBranch{{Name: current, Current: true}}, branches...)
		}
	}
	// 当前分支排在最前，其余按名称排序
	sorted := make([]GitBranch, 0, len(branches))
	for _, b := range branches {
		if current != "" && b.Name == current {
			continue
		}
		sorted = append(sorted, b)
	}
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Name < sorted[j-1].Name; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	if current != "" {
		sorted = append([]GitBranch{{Name: current, Current: true, Upstream: upstreamOf(branches, current)}}, sorted...)
	}
	return current, sorted
}

func upstreamOf(list []GitBranch, name string) string {
	for _, b := range list {
		if b.Name == name {
			return b.Upstream
		}
	}
	return ""
}

func doBranches(root, configured string) *GitBranchesResult {
	repo, _, errText := openRepo(root, configured)
	if repo == nil {
		return &GitBranchesResult{OK: false, Branches: []GitBranch{}, Error: errText}
	}
	current, list := readBranches(repo.git.Path, repo.root)
	return &GitBranchesResult{OK: true, Root: repo.root, Branch: current, Branches: list}
}

func doBranchCreate(root, configured, rawName string) *GitBranchResult {
	repo, _, errText := openRepo(root, configured)
	if repo == nil {
		return &GitBranchResult{OK: false, Error: errText}
	}
	name, errText := validateBranchName(rawName)
	if errText != "" {
		return &GitBranchResult{OK: false, Root: repo.root, Error: errText}
	}
	_, list := readBranches(repo.git.Path, repo.root)
	for _, b := range list {
		if b.Name == name {
			return &GitBranchResult{OK: false, Root: repo.root, Error: "分支「" + name + "」已存在"}
		}
	}
	r := capture(repo.git.Path, []string{"checkout", "-b", name}, repo.root, branchTimeout)
	res := &GitBranchResult{OK: r.Code == 0, Root: repo.root, Branch: name, Output: r.output()}
	if r.Code != 0 {
		res.Error = firstLine(r.Stderr)
		if res.Error == "" {
			res.Error = r.output()
		}
		if res.Error == "" {
			res.Error = "创建分支失败"
		}
	}
	return res
}

func doBranchSwitch(root, configured, rawName string) *GitBranchResult {
	repo, _, errText := openRepo(root, configured)
	if repo == nil {
		return &GitBranchResult{OK: false, Error: errText}
	}
	name := strings.TrimSpace(rawName)
	if name == "" {
		return &GitBranchResult{OK: false, Root: repo.root, Error: "请选择要切换的分支"}
	}
	current, list := readBranches(repo.git.Path, repo.root)
	exists := false
	for _, b := range list {
		if b.Name == name {
			exists = true
			break
		}
	}
	if !exists {
		return &GitBranchResult{OK: false, Root: repo.root, Error: "本地不存在分支「" + name + "」"}
	}
	if current == name {
		return &GitBranchResult{OK: true, Root: repo.root, Branch: name, Unchanged: true}
	}
	r := capture(repo.git.Path, []string{"checkout", name}, repo.root, branchTimeout)
	res := &GitBranchResult{OK: r.Code == 0, Root: repo.root, Branch: name, Output: r.output()}
	if r.Code != 0 {
		res.Error = firstLine(r.Stderr)
		if res.Error == "" {
			res.Error = r.output()
		}
		if res.Error == "" {
			res.Error = "切换分支失败"
		}
	}
	return res
}

func doBranchDelete(root, configured, rawName string, force bool) *GitBranchDeleteResult {
	repo, _, errText := openRepo(root, configured)
	if repo == nil {
		return &GitBranchDeleteResult{OK: false, Error: errText}
	}
	name := strings.TrimSpace(rawName)
	if name == "" {
		return &GitBranchDeleteResult{OK: false, Root: repo.root, Error: "请选择要删除的分支"}
	}
	current, list := readBranches(repo.git.Path, repo.root)
	if current == name {
		return &GitBranchDeleteResult{OK: false, Root: repo.root, Error: "不能删除当前所在的分支"}
	}
	exists := false
	for _, b := range list {
		if b.Name == name {
			exists = true
			break
		}
	}
	if !exists {
		return &GitBranchDeleteResult{OK: false, Root: repo.root, Error: "本地不存在分支「" + name + "」"}
	}
	flag := "-d"
	if force {
		flag = "-D"
	}
	r := capture(repo.git.Path, []string{"branch", flag, name}, repo.root, branchTimeout)
	out := r.output()
	notMerged := !force && r.Code != 0 && regexp.MustCompile(`(?i)not fully merged|not merged|未完全合并`).MatchString(out)
	res := &GitBranchDeleteResult{OK: r.Code == 0, Root: repo.root, Output: out, NotMerged: notMerged}
	if r.Code != 0 {
		res.Error = firstLine(r.Stderr)
		if res.Error == "" {
			res.Error = out
		}
		if res.Error == "" {
			res.Error = "删除分支失败"
		}
	}
	return res
}

// ---------------------------------------------------------------------------
// 提交历史
// ---------------------------------------------------------------------------

func doHistory(root, configured string, limit, offset int) *GitHistoryResult {
	repo, _, errText := openRepo(root, configured)
	if repo == nil {
		return &GitHistoryResult{OK: false, Commits: []GitCommit{}, Error: errText}
	}
	if limit < 1 {
		limit = 30
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	// 用 %x1f（不可见分隔符）拼接字段，避免提交说明里的空格/制表符破坏解析
	format := "--format=%H%x1f%h%x1f%an%x1f%ad%x1f%P%x1f%D%x1f%s"
	r := capture(repo.git.Path, []string{
		"log",
		"--max-count=" + strconv.Itoa(limit),
		"--skip=" + strconv.Itoa(offset),
		"--date=format:%Y-%m-%d %H:%M",
		format,
	}, repo.root, historyTimeout)

	if r.Code != 0 {
		raw := r.output()
		if regexp.MustCompile(`(?i)does not have any commits|unknown revision|bad revision|ambiguous argument`).MatchString(raw) {
			return &GitHistoryResult{OK: true, Root: repo.root, Commits: []GitCommit{}, Empty: true}
		}
		return &GitHistoryResult{OK: false, Root: repo.root, Commits: []GitCommit{}, Error: firstLine(r.Stderr)}
	}

	commits := []GitCommit{}
	for _, line := range strings.Split(r.Stdout, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) < 7 {
			continue
		}
		var parents, refs []string
		if parts[4] != "" {
			parents = strings.Fields(parts[4])
		}
		if parts[5] != "" {
			for _, ref := range strings.Split(parts[5], ", ") {
				if s := strings.TrimSpace(ref); s != "" {
					refs = append(refs, s)
				}
			}
		}
		commits = append(commits, GitCommit{
			Hash:      parts[0],
			ShortHash: parts[1],
			Author:    parts[2],
			Date:      parts[3],
			Parents:   append([]string{}, parents...),
			Refs:      append([]string{}, refs...),
			Subject:   strings.Join(parts[6:], "\x1f"),
		})
	}
	return &GitHistoryResult{
		OK:      true,
		Root:    repo.root,
		Commits: commits,
		HasMore: len(commits) >= limit,
		Empty:   len(commits) == 0,
	}
}

// filepathIsAbs 是 filepath.IsAbs 的轻量封装，避免在非 Windows 分支里误判。
func filepathIsAbs(p string) bool {
	if len(p) >= 2 && p[1] == ':' {
		return true
	}
	return strings.HasPrefix(p, `\`)
}
