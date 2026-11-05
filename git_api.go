package main

// Wails 绑定层：把 Git 引擎暴露给前端。
// 所有方法都带 root 参数（当前仓库根目录），避免 Go 端状态与前端竞态。

func (a *App) GitStatus(root string, fetch bool) *GitStatus {
	return computeStatus(root, fetch, a.settings.GitPath)
}

func (a *App) GitPull(root string) *GitActionResult {
	return doPull(root, a.settings.GitPath)
}

func (a *App) GitPush(root string) *GitActionResult {
	return doPush(root, a.settings.GitPath)
}

func (a *App) GitCommitPush(root string, message string, files []string) *GitCommitResult {
	return doCommitPush(root, a.settings.GitPath, message, files)
}

func (a *App) GitResolveConflict(root string, mode string) *GitActionResult {
	return doResolveConflict(root, a.settings.GitPath, mode)
}

func (a *App) GitBranches(root string) *GitBranchesResult {
	return doBranches(root, a.settings.GitPath)
}

func (a *App) GitBranchCreate(root string, name string) *GitBranchResult {
	return doBranchCreate(root, a.settings.GitPath, name)
}

func (a *App) GitBranchSwitch(root string, name string) *GitBranchResult {
	return doBranchSwitch(root, a.settings.GitPath, name)
}

func (a *App) GitBranchDelete(root string, name string, force bool) *GitBranchDeleteResult {
	return doBranchDelete(root, a.settings.GitPath, name, force)
}

func (a *App) GitHistory(root string, offset int, limit int) *GitHistoryResult {
	return doHistory(root, a.settings.GitPath, limit, offset)
}
