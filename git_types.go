package main

// 本文件中的结构体字段与 md0/renderer/js/git.js 使用的字段保持一致，
// 这样从 md0 独立出来的 Git 面板前端逻辑可以几乎原样复用。

// GitFile 是 `git status --porcelain -z` 解析出的一条改动。
type GitFile struct {
	Path       string `json:"path"`
	Original   string `json:"original"`
	Index      string `json:"index"`
	Worktree   string `json:"worktree"`
	Kind       string `json:"kind"` // new | modified | deleted | renamed | copied | conflict
	Conflicted bool   `json:"conflicted"`
}

// GitStatus 是仓库的完整状态快照。
type GitStatus struct {
	OK              bool      `json:"ok"`
	IsRepo          bool      `json:"isRepo"`
	Root            string    `json:"root"`
	Workspace       string    `json:"workspace"`
	Branch          string    `json:"branch"`
	Remote          string    `json:"remote"`
	Upstream        string    `json:"upstream"`
	Ahead           int       `json:"ahead"`
	Behind          int       `json:"behind"`
	Changes         int       `json:"changes"`
	Conflicted      int       `json:"conflicted"`
	MergeInProgress bool      `json:"mergeInProgress"`
	Files           []GitFile `json:"files"`
	GitPath         string    `json:"gitPath"`
	GitVersion      string    `json:"gitVersion"`
	GitSource       string    `json:"gitSource"`
	FetchError      string    `json:"fetchError"`
	Error           string    `json:"error"`
}

func failedStatus(err string) *GitStatus {
	return &GitStatus{OK: false, Files: []GitFile{}, Error: err}
}

// GitActionResult 用于 pull / push / resolveConflict。
type GitActionResult struct {
	OK         bool   `json:"ok"`
	Root       string `json:"root"`
	Code       int    `json:"code"`
	Conflicted bool   `json:"conflicted"`
	Output     string `json:"output"`
	Error      string `json:"error"`
}

func failedAction(err string) *GitActionResult {
	return &GitActionResult{OK: false, Code: -1, Error: err}
}

// GitCommitResult 是「提交并推送」的结果。
type GitCommitResult struct {
	OK        bool   `json:"ok"`
	Committed bool   `json:"committed"`
	Pushed    bool   `json:"pushed"`
	Root      string `json:"root"`
	Output    string `json:"output"`
	Error     string `json:"error"`
}

func failedCommit(err string) *GitCommitResult {
	return &GitCommitResult{OK: false, Error: err}
}

// GitBranch 是本地分支列表中的一项。
type GitBranch struct {
	Name     string `json:"name"`
	Current  bool   `json:"current"`
	Upstream string `json:"upstream"`
}

// GitBranchesResult 是分支列表。
type GitBranchesResult struct {
	OK       bool        `json:"ok"`
	Root     string      `json:"root"`
	Branch   string      `json:"branch"`
	Branches []GitBranch `json:"branches"`
	Error    string      `json:"error"`
}

// GitBranchResult 是创建 / 切换分支的结果。
type GitBranchResult struct {
	OK        bool   `json:"ok"`
	Root      string `json:"root"`
	Branch    string `json:"branch"`
	Output    string `json:"output"`
	Unchanged bool   `json:"unchanged"`
	Error     string `json:"error"`
}

// GitBranchDeleteResult 是删除分支的结果（含「未完全合并」提示）。
type GitBranchDeleteResult struct {
	OK        bool   `json:"ok"`
	Root      string `json:"root"`
	Output    string `json:"output"`
	NotMerged bool   `json:"notMerged"`
	Error     string `json:"error"`
}

// GitCommit 是提交历史中的一条记录。
type GitCommit struct {
	Hash      string   `json:"hash"`
	ShortHash string   `json:"shortHash"`
	Author    string   `json:"author"`
	Date      string   `json:"date"`
	Parents   []string `json:"parents"`
	Refs      []string `json:"refs"`
	Subject   string   `json:"subject"`
}

// GitHistoryResult 是分页提交历史。
type GitHistoryResult struct {
	OK      bool        `json:"ok"`
	Root    string      `json:"root"`
	Commits []GitCommit `json:"commits"`
	HasMore bool        `json:"hasMore"`
	Empty   bool        `json:"empty"`
	Error   string      `json:"error"`
}

// GitDetectResult 是 Git 可执行文件探测结果。
type GitDetectResult struct {
	OK      bool   `json:"ok"`
	Path    string `json:"path"`
	Version string `json:"version"`
	Source  string `json:"source"` // auto | configured
	Error   string `json:"error"`
}
