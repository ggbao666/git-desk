// Git 工作台：同步、选择性提交并推送、分支管理、提交历史、冲突处理
// 交互逻辑移植自 md0/renderer/js/git.js，后端由 Go 实现。
import { App } from './api.js'
import { $, el, basename, toast, confirmDialog } from './ui.js'

const KIND_LABELS = {
  new: '新增', modified: '修改', deleted: '删除',
  renamed: '重命名', copied: '复制', conflict: '冲突',
}
const HISTORY_PAGE = 30
const TABS = ['sync', 'changes', 'branches', 'history']

let root = ''
let status = null
let busy = null
let activeTab = 'sync'
let token = 0

let selected = new Set()
let known = new Set()

let branches = []
let branchSelection = ''
let currentBranch = ''
let history = []
let historyExhausted = false
let loadingHistory = false
let historyRequest = 0

const infoOf = () => (status && status.ok && status.isRepo ? status : null)
const filesOf = (info) => (info && Array.isArray(info.files) ? info.files : [])

export function getRoot() { return root }
export function hasBusy() { return !!busy }

// ---------------------------------------------------------------------------
// 通用渲染
// ---------------------------------------------------------------------------

function setLog(text) {
  const log = $('#log')
  if (!log) return
  log.textContent = text || ''
  log.hidden = !text
  if (text) log.scrollTop = log.scrollHeight
}

function setStatus(text) {
  const line = $('#status-line')
  if (line) line.textContent = text || '就绪'
}

function syncText(next) {
  if (!next) return '—'
  const behind = Math.max(0, Number(next.behind) || 0)
  const ahead = Math.max(0, Number(next.ahead) || 0)
  if (!next.upstream) return next.changes ? `本地改动 ${next.changes} 个文件` : '未设置上游分支'
  if (!behind && !ahead) return next.changes ? `已是最新 · 改动 ${next.changes} 个文件` : '已是最新'
  const parts = []
  if (behind) parts.push(`落后 ${behind} 个提交`)
  if (ahead) parts.push(`领先 ${ahead} 个提交`)
  if (next.changes) parts.push(`改动 ${next.changes} 个文件`)
  return parts.join(' · ')
}

function applyStatus(next) {
  const files = filesOf(next)
  const paths = new Set(files.map((f) => f.path))
  for (const p of [...selected]) if (!paths.has(p)) selected.delete(p)
  for (const p of [...known]) if (!paths.has(p)) known.delete(p)
  for (const f of files) {
    if (known.has(f.path)) continue
    known.add(f.path)
    if (!f.conflicted) selected.add(f.path) // 新出现的文件默认勾选（冲突除外）
  }
}

function resetSelection() {
  selected = new Set()
  known = new Set()
}

export function setActiveTab(tab) {
  if (!TABS.includes(tab)) return
  activeTab = tab
  for (const t of TABS) {
    const button = $(`.tab[data-tab="${t}"]`)
    const panel = $(`.panel[data-panel="${t}"]`)
    button?.classList.toggle('is-active', t === tab)
    button?.setAttribute('aria-selected', t === tab ? 'true' : 'false')
    if (panel) panel.hidden = t !== tab
  }
  if (tab !== 'branches') hideBranchCreate()
  const p = $(`.panel[data-panel="${tab}"]`)
  if (p) p.scrollTop = 0
}

function setTabBadge(name, count) {
  const badge = $(`#badge-${name}`)
  if (!badge) return
  const value = Math.max(0, Number(count) || 0)
  badge.hidden = value === 0
  badge.textContent = String(value)
}

// ---------------------------------------------------------------------------
// 概览 / 同步
// ---------------------------------------------------------------------------

function renderOverview() {
  const info = infoOf()
  $('#ov-branch').textContent = info ? info.branch || '—' : '—'
  const remote = $('#ov-remote')
  remote.textContent = info ? info.remote || '未配置' : '—'
  remote.title = info ? info.remote || '' : ''
  const sync = $('#ov-sync')
  sync.textContent = info ? syncText(info) : '—'
  sync.className = `stat-value${info && info.behind > 0 ? ' behind' : ''}`
  const changes = $('#ov-changes')
  changes.textContent = info ? (info.conflicted > 0 ? `${info.conflicted} 个冲突` : `${info.changes} 个文件`) : '—'
  changes.className = `stat-value${info && info.conflicted > 0 ? ' conflict' : (info && info.changes > 0 ? ' dirty' : '')}`

  const badge = $('#ws-git-badge')
  if (info) {
    badge.textContent = info.branch && info.branch !== '（尚无提交）' ? info.branch : 'Git'
    badge.title = `${info.gitVersion || ''}\n${info.gitPath || ''}`
    badge.classList.remove('bad')
  } else {
    badge.textContent = 'Git'
    badge.classList.toggle('bad', !!(status && !status.ok))
  }
}

function renderSync() {
  const info = infoOf()
  const hint = $('#sync-hint')
  if (!status) hint.textContent = '正在读取仓库状态…'
  else if (!status.ok) hint.textContent = status.error || '无法读取 Git 状态'
  else {
    const source = info.gitSource === 'configured' ? '手动指定' : '自动探测'
    const parts = [`Git：${info.gitPath}（${source}）`]
    if (info.fetchError) parts.push(`远端刷新失败：${info.fetchError}`)
    hint.textContent = parts.join('；')
  }

  $('#env-git-path').textContent = info ? info.gitPath || '—' : '—'
  $('#env-git-source').textContent = info ? (info.gitSource === 'configured' ? '手动指定' : '自动探测') : '—'
  $('#env-root').textContent = info ? info.root || root : '—'
  $('#env-upstream').textContent = info ? info.upstream || '未设置' : '—'

  const box = $('#conflict-box')
  const conflicts = Number(info && info.conflicted) || 0
  box.hidden = conflicts <= 0
  $('#conflict-count').textContent = String(conflicts)
  $('#conflict-hint').textContent = info && info.mergeInProgress
    ? '合并尚未完成。可以统一选择一侧，或自行编辑文件去掉冲突标记后勾选提交。'
    : '可以统一选择一侧，或自行编辑文件去掉冲突标记后勾选提交。'
}

// ---------------------------------------------------------------------------
// 改动目录树
// ---------------------------------------------------------------------------

function buildTree(files) {
  const node = { dirs: new Map(), files: [] }
  for (const file of files) {
    const parts = String(file.path).split('/')
    let cur = node
    for (let i = 0; i < parts.length - 1; i++) {
      const name = parts[i]
      if (!cur.dirs.has(name)) cur.dirs.set(name, { name, path: parts.slice(0, i + 1).join('/'), dirs: new Map(), files: [] })
      cur = cur.dirs.get(name)
    }
    cur.files.push(file)
  }
  return node
}

function collectFiles(node, out = []) {
  for (const f of node.files) out.push(f)
  for (const d of node.dirs.values()) collectFiles(d, out)
  return out
}

const byName = (a, b) => a.name.localeCompare(b.name, 'zh-CN')

function renderTree(node, depth, host, files) {
  const dirs = [...node.dirs.values()].sort(byName)
  const entries = [...node.files].sort((a, b) => a.path.localeCompare(b.path, 'zh-CN'))

  for (const dir of dirs) {
    const subtree = collectFiles(dir)
    const checked = subtree.filter((f) => selected.has(f.path)).length
    const checkbox = el('input', { type: 'checkbox', title: '勾选目录下全部文件' })
    checkbox.checked = subtree.length > 0 && checked === subtree.length
    checkbox.addEventListener('change', () => {
      for (const f of subtree) {
        if (checkbox.checked) selected.add(f.path)
        else selected.delete(f.path)
      }
      renderChanges(files)
      updateActions()
    })
    const row = el('div', { class: 'change-row is-dir' }, [
      checkbox,
      el('span', { class: 'change-kind kind-directory', text: '目录' }),
      el('span', { class: 'change-path', text: dir.name, title: dir.path }),
      el('span', { class: 'dir-count', text: checked === subtree.length ? `全部 ${subtree.length}` : `${checked}/${subtree.length}` }),
    ])
    row.style.paddingLeft = `${12 + depth * 18}px`
    host.appendChild(row)
    renderTree(dir, depth + 1, host, files)
  }

  for (const file of entries) {
    const checkbox = el('input', { type: 'checkbox', title: '勾选后参与本次提交' })
    checkbox.checked = selected.has(file.path)
    checkbox.addEventListener('change', () => {
      if (checkbox.checked) selected.add(file.path)
      else selected.delete(file.path)
      renderChanges(files)
      updateActions()
    })
    const row = el('div', { class: 'change-row' }, [
      checkbox,
      el('span', {
        class: `change-kind kind-${file.kind}`,
        text: KIND_LABELS[file.kind] || '改动',
        title: file.conflicted ? '需要解决冲突后才能提交' : '',
      }),
      el('span', {
        class: 'change-path',
        text: basename(file.path),
        title: file.original ? `${file.original} → ${file.path}` : file.path,
      }),
    ])
    row.style.paddingLeft = `${12 + depth * 18}px`
    host.appendChild(row)
  }
}

function renderChanges(files) {
  const host = $('#changes-tree')
  if (!host) return
  const scrollTop = host.scrollTop
  host.textContent = ''
  renderTree(buildTree(files), 0, host, files)
  $('#changes-empty').hidden = files.length > 0
  setTabBadge('changes', files.length)

  const total = files.length
  $('#changes-count').textContent = total ? `已选 ${selected.size} / ${total}` : ''
  const all = $('#select-all')
  all.checked = total > 0 && selected.size === total
  all.indeterminate = selected.size > 0 && selected.size < total
  all.disabled = !total || !!busy
  host.scrollTop = scrollTop
}

// ---------------------------------------------------------------------------
// 分支
// ---------------------------------------------------------------------------

function branchFromStatus(info) {
  const value = info ? info.branch || '' : ''
  return value && value !== '（尚无提交）' ? value : ''
}
function currentBranchOf(info) { return branchFromStatus(info) || currentBranch || '' }

function updateBranchControls() {
  const info = infoOf()
  const select = $('#branch-select')
  if (!select) return
  const current = currentBranchOf(info)
  const target = select.value
  const blocked = !info || !!busy
  select.disabled = blocked || select.options.length === 0
  $('#act-branch-switch').disabled = blocked || !target || target === current
  $('#act-branch-delete').disabled = blocked || !target || target === current
  $('#act-branch-new').disabled = blocked
  $('#act-branch-confirm').disabled = blocked
  $('#act-branch-cancel').disabled = !!busy
  $('#branch-count').textContent = info && select.options.length
    ? `${select.options.length} 个本地分支${info.mergeInProgress ? ' · 合并进行中' : ''}`
    : ''
}

function renderBranches() {
  const select = $('#branch-select')
  if (!select) return
  const info = infoOf()
  const current = currentBranchOf(info)
  const list = branches.map((b) => ({ ...b }))
  if (current && !list.some((b) => b.name === current)) list.unshift({ name: current, current: true, upstream: '' })
  list.sort((a, b) => (Number(b.current) - Number(a.current)) || a.name.localeCompare(b.name, 'zh-CN'))

  const names = list.map((b) => b.name)
  if (!names.includes(branchSelection)) branchSelection = current || names[0] || ''
  select.textContent = ''
  for (const b of list) {
    const option = el('option', { value: b.name, text: b.name + (b.name === current ? '  ·  当前' : '') })
    if (b.name === branchSelection) option.selected = true
    select.appendChild(option)
  }
  setTabBadge('branches', list.length)
  updateBranchControls()
}

async function refreshBranches() {
  const myRoot = root
  const info = infoOf()
  if (!myRoot || !info) {
    branches = []; branchSelection = ''; currentBranch = ''
    renderBranches()
    return
  }
  let result = null
  try { result = await App.GitBranches(myRoot) } catch { result = null }
  if (myRoot !== root) return
  if (result && result.ok && Array.isArray(result.branches)) {
    const previous = currentBranch
    branches = result.branches
    currentBranch = result.branch || ''
    if (!branches.some((b) => b.name === branchSelection)) branchSelection = currentBranch
    else if (branchSelection === previous && branchSelection !== currentBranch) branchSelection = currentBranch
  } else {
    branches = []; currentBranch = ''
  }
  renderBranches()
}

function showBranchCreate() {
  const row = $('#branch-create')
  row.hidden = false
  $('#branch-name').value = ''
  $('#branch-name').focus()
  updateBranchControls()
}
function hideBranchCreate() {
  const row = $('#branch-create')
  if (!row) return
  row.hidden = true
  $('#branch-name').value = ''
  updateBranchControls()
}

// ---------------------------------------------------------------------------
// 提交历史
// ---------------------------------------------------------------------------

function renderHistory() {
  const host = $('#history-list')
  if (!host) return
  const info = infoOf()
  const items = info ? history : []
  host.textContent = ''
  $('#history-empty').hidden = !info || items.length > 0
  $('#history-count').textContent = info && items.length ? `已加载 ${items.length} 条` : ''
  const more = $('#act-history-more')
  more.hidden = !info || items.length === 0 || historyExhausted
  more.disabled = !!busy || loadingHistory
  more.textContent = loadingHistory ? '加载中…' : '加载更多'
  if (!info) return

  for (const commit of items) {
    const refs = Array.isArray(commit.refs) ? commit.refs : []
    const isHead = refs.some((r) => r.startsWith('HEAD'))
    const content = el('div', { class: 'history-content' }, [
      el('div', { class: 'history-subject', text: commit.subject || '（无提交说明）', title: commit.subject || '' }),
      el('div', {
        class: 'history-meta',
        text: [commit.author, commit.date].filter(Boolean).join(' · '),
        title: commit.hash || '',
      }),
    ])
    if (refs.length) {
      const refRow = el('div', { class: 'history-refs' })
      for (const ref of refs) {
        const isTag = ref.startsWith('tag: ')
        refRow.appendChild(el('span', {
          class: `history-ref${isTag ? ' is-tag' : ''}${ref.startsWith('HEAD') ? ' is-head' : ''}`,
          text: isTag ? ref.slice(5) : ref.replace('HEAD ->', 'HEAD →'),
          title: ref,
        }))
      }
      content.appendChild(refRow)
    }
    host.appendChild(el('div', { class: `history-item${isHead ? ' is-head' : ''}` }, [
      el('div', { class: 'history-hash', text: commit.shortHash || '', title: commit.hash || '' }),
      content,
    ]))
  }
}

async function loadHistory(reset = true) {
  const myRoot = root
  const info = infoOf()
  if (!myRoot || !info) {
    historyRequest++
    history = []; historyExhausted = false; loadingHistory = false
    renderHistory()
    return
  }
  if (!reset && (historyExhausted || loadingHistory)) return
  historyRequest++
  const requestId = historyRequest
  if (reset) { history = []; historyExhausted = false }
  const offset = history.length
  loadingHistory = true
  renderHistory()
  try {
    const result = await App.GitHistory(myRoot, offset, HISTORY_PAGE)
    if (requestId !== historyRequest || myRoot !== root) return
    if (result?.ok) {
      const commits = Array.isArray(result.commits) ? result.commits : []
      history = reset ? commits : history.concat(commits)
      historyExhausted = !result.hasMore || commits.length === 0
    } else {
      historyExhausted = true
      if (result?.error) toast(result.error, 'error', 4600)
    }
  } catch (err) {
    if (requestId !== historyRequest) return
    historyExhausted = true
    toast(String(err?.message || err), 'error', 4600)
  } finally {
    if (requestId === historyRequest) { loadingHistory = false; renderHistory() }
  }
}

// ---------------------------------------------------------------------------
// 按钮状态
// ---------------------------------------------------------------------------

function updateActions() {
  const info = infoOf()
  const conflicts = Number(info && info.conflicted) || 0
  const ahead = Math.max(0, Number(info && info.ahead) || 0)
  const behind = Math.max(0, Number(info && info.behind) || 0)
  const message = String($('#commit-message')?.value || '').trim()

  const pull = $('#act-pull')
  pull.disabled = !info || !!busy || !info.upstream || conflicts > 0 || !!info.mergeInProgress
  pull.innerHTML = busy === 'pull' ? '拉取中…' : '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4v12m0 0 5-5m-5 5-5-5"/><path d="M4 19h16"/></svg>拉取更新'
  pull.title = conflicts > 0 || (info && info.mergeInProgress) ? '请先解决冲突'
    : (!info || !info.upstream ? '未设置上游分支' : '从远端拉取更新')

  const push = $('#act-push')
  push.hidden = !info || ahead <= 0
  push.disabled = !info || !!busy || ahead <= 0
  push.textContent = busy === 'push' ? '推送中…' : (ahead > 1 ? `推送 ${ahead} 个提交` : '推送 1 个提交')
  push.title = ahead > 0 ? `推送 ${ahead} 个本地提交到远端` : '没有待推送的提交'

  const commit = $('#act-commit')
  commit.disabled = !info || !!busy || selected.size === 0 || !message
  commit.textContent = busy === 'commit' ? '提交中…' : '提交并推送'
  commit.title = !selected.size ? '请勾选要提交的文件' : (!message ? '请填写提交说明' : '提交已勾选的文件并推送到远端')

  $('#act-refresh').disabled = !!busy
  $('#ws-refresh').disabled = !!busy
  $('#act-resolve-ours').disabled = !!busy
  $('#act-resolve-theirs').disabled = !!busy
  $('#act-resolve-abort').disabled = !!busy
  $('#act-history-more').disabled = !!busy || loadingHistory

  $('#sync-note').textContent = info
    ? (behind > 0 ? `远端有 ${behind} 个新提交，建议先拉取` : (ahead > 0 ? `有 ${ahead} 个本地提交待推送` : '本地与远端保持同步'))
    : ''

  updateBranchControls()
}

function renderAll() {
  renderOverview()
  renderSync()
  const info = infoOf()
  renderChanges(filesOf(info))
  renderBranches()
  renderHistory()
  updateActions()
}

// ---------------------------------------------------------------------------
// 打开 / 刷新
// ---------------------------------------------------------------------------

export async function openRepo(path) {
  root = path
  token++
  status = null
  resetSelection()
  branches = []; branchSelection = ''; currentBranch = ''
  history = []; historyExhausted = false; loadingHistory = false
  busy = null
  setLog('')
  setStatus('正在读取仓库状态…')
  $('#commit-message').value = ''
  $('#ws-repo-name').textContent = basename(path)
  $('#ws-repo-path').textContent = path
  $('#ws-repo-path').title = path
  hideBranchCreate()
  setActiveTab('sync')
  renderAll()
  await refreshStatus({ fetch: false })
}

export function closeRepo() {
  token++
  root = ''
  status = null
  busy = null
  resetSelection()
  branches = []; branchSelection = ''; currentBranch = ''
  history = []; historyExhausted = false
  setLog('')
  setStatus('就绪')
}

export async function refreshStatus(options = {}) {
  if (!root) return
  const myToken = ++token
  const myRoot = root
  setStatus('正在刷新仓库状态…')
  try {
    status = await App.GitStatus(myRoot, options.fetch === true)
  } catch (err) {
    status = { ok: false, isRepo: true, error: String(err?.message || err) }
  }
  if (myToken !== token || myRoot !== root) return
  applyStatus(status && status.isRepo ? status : null)
  if (!options.keepLog) setLog('')
  renderAll()
  setStatus(infoOf() ? `上次刷新 ${new Date().toLocaleTimeString('zh-CN', { hour12: false })}` : (status?.error || '无法读取仓库状态'))
  await Promise.all([refreshBranches(), loadHistory(true)])
}

function setBusy(kind) {
  busy = kind
  updateActions()
  renderHistory()
}

// ---------------------------------------------------------------------------
// 操作
// ---------------------------------------------------------------------------

async function afterAction(keepLog = true) {
  setBusy(null)
  await refreshStatus({ keepLog, fetch: false })
}

async function doPull() {
  if (busy || !infoOf()) return
  setBusy('pull'); setLog('正在拉取更新…')
  try {
    const r = await App.GitPull(root)
    if (r?.ok) { setLog(r.output || '已是最新。'); toast('已拉取最新更新', 'ok') }
    else if (r?.conflicted) { setLog(r.output || ''); toast('拉取产生冲突，请选择处理方式', 'error', 4200) }
    else { setLog(r?.output || r?.error || '拉取失败'); toast('拉取失败，详见输出', 'error', 4200) }
  } catch (err) { setLog(String(err?.message || err)); toast('拉取失败', 'error', 4200) }
  await afterAction()
}

async function doPush() {
  if (busy || !infoOf()) return
  setBusy('push'); setLog('正在推送…')
  try {
    const r = await App.GitPush(root)
    setLog(r?.output || '')
    if (r?.ok) toast('已推送到远端', 'ok')
    else toast(r?.error || '推送失败，详见输出', 'error', 4600)
  } catch (err) { setLog(String(err?.message || err)); toast('推送失败', 'error', 4200) }
  await afterAction()
}

async function doCommitPush() {
  if (busy || !infoOf()) return
  const message = String($('#commit-message').value || '').trim()
  if (!message) { toast('请填写提交说明', 'error'); return }
  if (!selected.size) { toast('请勾选要提交的文件', 'error'); return }
  setBusy('commit'); setLog('正在提交…')
  try {
    const r = await App.GitCommitPush(root, message, [...selected])
    setLog(r?.output || '')
    if (r?.ok) { toast('已提交并推送', 'ok'); $('#commit-message').value = ''; resetSelection() }
    else if (r?.committed) { toast(r.error || '提交成功，但推送失败', 'error', 4600); $('#commit-message').value = ''; resetSelection() }
    else toast(r?.error || '提交失败，详见输出', 'error', 4600)
  } catch (err) { setLog(String(err?.message || err)); toast('提交失败', 'error', 4200) }
  await afterAction()
}

async function doResolve(mode) {
  if (busy || !infoOf()) return
  if (mode === 'abort') {
    const ok = await confirmDialog('放弃本次合并？已拉取的合并结果会被撤销。', { title: '放弃合并', okText: '放弃合并', danger: true })
    if (!ok) return
  }
  setBusy('resolve'); setLog('正在处理冲突…')
  try {
    const r = await App.GitResolveConflict(root, mode)
    setLog(r?.output || '')
    if (r?.ok) {
      const label = mode === 'ours' ? '已使用本地版本' : mode === 'theirs' ? '已使用远程版本' : '已放弃本次合并'
      toast(label, 'ok'); resetSelection()
    } else toast(r?.error || '处理冲突失败', 'error', 4600)
  } catch (err) { setLog(String(err?.message || err)); toast('处理冲突失败', 'error', 4200) }
  await afterAction()
}

async function doCreateBranch() {
  if (busy || !infoOf()) return
  const name = String($('#branch-name').value || '').trim()
  if (!name) { toast('请填写分支名称', 'error'); $('#branch-name').focus(); return }
  setBusy('branch'); setLog(`正在创建并切换到分支 ${name}…`)
  try {
    const r = await App.GitBranchCreate(root, name)
    setLog(r?.output || '')
    if (r?.ok) { branchSelection = r.branch || name; hideBranchCreate(); toast(`已创建并切换到分支 ${name}`, 'ok') }
    else toast(r?.error || '创建分支失败', 'error', 4600)
  } catch (err) { setLog(String(err?.message || err)); toast('创建分支失败', 'error', 4200) }
  await afterAction()
}

async function doSwitchBranch() {
  if (busy || !infoOf()) return
  const target = String($('#branch-select').value || '').trim()
  if (!target) return
  if (target === currentBranchOf(infoOf())) { toast('已经是当前分支'); return }
  const changes = Math.max(0, Number(infoOf().changes) || 0)
  if (changes > 0) {
    const ok = await confirmDialog(`工作区有 ${changes} 个未提交改动，切换分支会一并带到「${target}」，确定继续？`, { title: '切换分支', okText: '继续切换' })
    if (!ok) return
  }
  setBusy('branch'); setLog(`正在切换到分支 ${target}…`)
  try {
    const r = await App.GitBranchSwitch(root, target)
    setLog(r?.output || '')
    if (r?.ok) { branchSelection = target; toast(`已切换到分支 ${target}`, 'ok') }
    else toast(r?.error || '切换分支失败', 'error', 4600)
  } catch (err) { setLog(String(err?.message || err)); toast('切换分支失败', 'error', 4200) }
  await afterAction()
}

async function doDeleteBranch() {
  if (busy || !infoOf()) return
  const target = String($('#branch-select').value || '').trim()
  if (!target) return
  if (target === currentBranchOf(infoOf())) { toast('不能删除当前所在的分支', 'error'); return }
  let ok = await confirmDialog(`确定删除本地分支「${target}」？该操作不会影响远端分支。`, { title: '删除分支', okText: '删除', danger: true })
  if (!ok) return
  setBusy('branch'); setLog(`正在删除分支 ${target}…`)
  try {
    let r = await App.GitBranchDelete(root, target, false)
    if (!r?.ok && r?.notMerged) {
      ok = await confirmDialog(`分支「${target}」还有未合并的提交，强制删除会丢失这些提交，确定继续？`, { title: '强制删除', okText: '强制删除', danger: true })
      if (!ok) { setLog(''); toast('已取消删除'); setBusy(null); return }
      r = await App.GitBranchDelete(root, target, true)
    }
    setLog(r?.output || '')
    if (r?.ok) { toast(`已删除本地分支 ${target}`, 'ok'); if (branchSelection === target) branchSelection = '' }
    else toast(r?.error || '删除分支失败', 'error', 4600)
  } catch (err) { setLog(String(err?.message || err)); toast('删除分支失败', 'error', 4200) }
  await afterAction()
}

// ---------------------------------------------------------------------------
// 初始化
// ---------------------------------------------------------------------------

export function initGitView() {
  for (const t of TABS) {
    $(`.tab[data-tab="${t}"]`)?.addEventListener('click', () => setActiveTab(t))
  }

  $('#act-pull').addEventListener('click', () => void doPull())
  $('#act-push').addEventListener('click', () => void doPush())
  $('#act-commit').addEventListener('click', () => void doCommitPush())
  $('#commit-message').addEventListener('input', updateActions)
  $('#commit-message').addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !$('#act-commit').disabled) void doCommitPush()
  })
  $('#act-resolve-ours').addEventListener('click', () => void doResolve('ours'))
  $('#act-resolve-theirs').addEventListener('click', () => void doResolve('theirs'))
  $('#act-resolve-abort').addEventListener('click', () => void doResolve('abort'))

  $('#branch-select').addEventListener('change', (e) => { branchSelection = e.target.value; updateBranchControls() })
  $('#act-branch-switch').addEventListener('click', () => void doSwitchBranch())
  $('#act-branch-new').addEventListener('click', showBranchCreate)
  $('#act-branch-cancel').addEventListener('click', hideBranchCreate)
  $('#act-branch-confirm').addEventListener('click', () => void doCreateBranch())
  $('#act-branch-delete').addEventListener('click', () => void doDeleteBranch())
  $('#branch-name').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); void doCreateBranch() }
    else if (e.key === 'Escape') { e.stopPropagation(); hideBranchCreate() }
  })
  $('#act-history-more').addEventListener('click', () => void loadHistory(false))

  $('#select-all').addEventListener('change', (e) => {
    const files = filesOf(infoOf())
    if (e.target.checked) for (const f of files) selected.add(f.path)
    else selected.clear()
    renderChanges(files)
    updateActions()
  })

  $('#ws-refresh').addEventListener('click', () => void refreshStatus({ fetch: true }))
  $('#act-refresh').addEventListener('click', () => void refreshStatus({ fetch: true }))
}
