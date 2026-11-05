import './style.css'
import { App, rt, onEvent } from './api.js'
import { $, toast, confirmDialog, initUI } from './ui.js'
import * as welcome from './welcome.js'
import * as gitview from './gitview.js'
import * as gitignore from './gitignore.js'

let settings = { gitPath: '', theme: 'dark' }
let pendingInitDir = ''
let dropFallbackTimer = null
let lastDropKey = ''
let lastDropAt = 0

// 启动页使用紧凑小窗口，进入工作台后放大
const WINDOW_SIZE = {
  welcome:   { w: 440, h: 420, minW: 400, minH: 380 },
  workspace: { w: 1120, h: 760, minW: 880, minH: 580 },
}

// ---------------------------------------------------------------------------
// 视图
// ---------------------------------------------------------------------------

function applyTheme(theme) {
  document.body.classList.toggle('light', theme === 'light')
}

function applyWindowSize(mode) {
  const r = rt()
  if (!r) return
  const s = WINDOW_SIZE[mode] || WINDOW_SIZE.welcome
  try {
    // 缩小要先放宽最小尺寸，放大则相反，否则会被 min size 撑住
    if (mode === 'welcome') {
      r.WindowSetMinSize?.(s.minW, s.minH)
      r.WindowSetSize?.(s.w, s.h)
    } else {
      r.WindowSetSize?.(s.w, s.h)
      r.WindowSetMinSize?.(s.minW, s.minH)
    }
    r.WindowCenter?.()
  } catch { /* 运行时未就绪时忽略 */ }
}

function showWorkspace() {
  $('#welcome').hidden = true
  $('#workspace').hidden = false
  applyWindowSize('workspace')
}

function showWelcome() {
  $('#workspace').hidden = true
  $('#welcome').hidden = false
  applyWindowSize('welcome')
  void welcome.renderRecent()
  void welcome.refreshGitInfo()
}

// ---------------------------------------------------------------------------
// 打开仓库（文件选择器 / 拖拽 / 历史记录 / 命令行参数 共用）
// ---------------------------------------------------------------------------

async function openFromPath(rawPath, { quiet = false } = {}) {
  if (!rawPath) return false
  let dir = rawPath
  try {
    dir = await App.ResolveDroppedPath(rawPath)
  } catch (err) {
    if (!quiet) toast(String(err?.message || err), 'error')
    return false
  }

  try {
    const root = await App.OpenDirectory(dir)
    showWorkspace()
    await gitview.openRepo(root)
    await welcome.renderRecent()
    return true
  } catch (err) {
    const message = String(err?.message || err)
    if (quiet) return false
    if (/不是 Git 仓库/.test(message)) {
      const ok = await confirmDialog(
        `${message}\n\n是否在此目录执行 git init 初始化一个新仓库？`,
        { title: '不是 Git 仓库', okText: '初始化并打开' },
      )
      if (!ok) return false
      try {
        const root = await App.InitRepo(dir)
        showWorkspace()
        await gitview.openRepo(root)
        await welcome.renderRecent()
        toast('仓库已初始化', 'ok')
        return true
      } catch (initErr) {
        toast(String(initErr?.message || initErr), 'error', 5000)
        return false
      }
    }
    toast(message, 'error', 5000)
    return false
  }
}

// ---------------------------------------------------------------------------
// 设置
// ---------------------------------------------------------------------------

function openSettings() {
  $('#set-git-path').value = settings.gitPath || ''
  $('#set-theme').value = settings.theme || 'dark'
  $('#set-init-dir').value = pendingInitDir || ''
  $('#settings-modal').hidden = false
  void refreshDetectStatus(false)
}

function closeSettings() { $('#settings-modal').hidden = true }

async function refreshDetectStatus(refresh) {
  const node = $('#set-git-status')
  node.className = 'detect-status'
  node.textContent = '检测中…'
  try {
    const r = await App.GitDetect(!!refresh)
    if (r?.ok) {
      node.className = 'detect-status ok'
      node.textContent = `${r.version} · ${r.path}`
      node.title = `${r.version}\n${r.path}`
    } else {
      node.className = 'detect-status bad'
      node.textContent = r?.error || '未检测到 Git'
      node.title = r?.error || ''
    }
  } catch (err) {
    node.className = 'detect-status bad'
    node.textContent = String(err?.message || err)
  }
}

function initSettings() {
  $('#settings-close').addEventListener('click', closeSettings)
  $('#settings-cancel').addEventListener('click', closeSettings)
  $('#settings-modal').addEventListener('mousedown', (e) => {
    if (e.target === $('#settings-modal')) closeSettings()
  })
  $('#set-git-detect').addEventListener('click', () => void refreshDetectStatus(true))
  $('#set-git-browse').addEventListener('click', async () => {
    try {
      const p = await App.PickGitExecutable()
      if (p) { $('#set-git-path').value = p; await refreshDetectStatus(true) }
    } catch (err) { toast(String(err?.message || err), 'error') }
  })
  $('#set-init-browse').addEventListener('click', async () => {
    try {
      const p = await App.PickDirectory()
      if (p) { pendingInitDir = p; $('#set-init-dir').value = p }
    } catch (err) { toast(String(err?.message || err), 'error') }
  })
  $('#set-init-run').addEventListener('click', async () => {
    if (!pendingInitDir) { toast('请先选择一个目录', 'error'); return }
    try {
      const root = await App.InitRepo(pendingInitDir)
      closeSettings()
      showWorkspace()
      await gitview.openRepo(root)
      await welcome.renderRecent()
      toast('仓库已初始化并打开', 'ok')
    } catch (err) { toast(String(err?.message || err), 'error', 5000) }
  })
  $('#settings-save').addEventListener('click', async () => {
    const next = {
      gitPath: String($('#set-git-path').value || '').trim(),
      theme: $('#set-theme').value === 'light' ? 'light' : 'dark',
    }
    try {
      settings = await App.SaveSettings(next)
      applyTheme(settings.theme)
      closeSettings()
      toast('设置已保存', 'ok')
      await welcome.refreshGitInfo()
      if (gitview.getRoot()) await gitview.refreshStatus({ fetch: false })
    } catch (err) { toast(String(err?.message || err), 'error') }
  })
}

// ---------------------------------------------------------------------------
// 拖拽：Wails 主链路 + 原生兜底
// ---------------------------------------------------------------------------

const hasFiles = (e) => !!(e.dataTransfer && [...(e.dataTransfer.types || [])].includes('Files'))

function showDropOverlay() { $('#drop-overlay').hidden = false; welcome.setDropActive(true) }
function hideDropOverlay() { $('#drop-overlay').hidden = true; welcome.setDropActive(false) }

function handleDroppedPaths(paths) {
  if (!Array.isArray(paths) || !paths.length) return
  clearTimeout(dropFallbackTimer)
  // 同一路径在 1 秒内可能因主链路 + 备选链路各触发一次，做幂等去重
  const key = paths.join('|')
  const now = Date.now()
  if (key === lastDropKey && now - lastDropAt < 1000) return
  lastDropKey = key
  lastDropAt = now
  hideDropOverlay()
  // 只取第一个：一个应用一次只管理一个仓库
  void openFromPath(paths[0])
}

function initDragAndDrop() {
  // 让 Wails runtime 安装 window 级 dragover/drop 监听并把 File 对象解析成真实路径。
  // 传 false 表示整窗都是拖放目标，不需要元素带 CSS 标记。
  try { rt()?.OnFileDrop?.(() => {}, false) } catch { /* 运行时可能尚未注入 */ }

  // 主链路：Go 端收到 wails:file-drop 后转发的干净路径列表
  onEvent('app:dropped-paths', (paths) => handleDroppedPaths(paths))
  // 备选链路：前端直接监听 Wails 原生事件 (x, y, paths)
  onEvent('wails:file-drop', (_x, _y, paths) => handleDroppedPaths(paths))

  let dragDepth = 0
  window.addEventListener('dragenter', (e) => {
    if (!hasFiles(e)) return
    e.preventDefault()
    dragDepth++
    showDropOverlay()
  }, { capture: true })

  window.addEventListener('dragover', (e) => {
    if (!hasFiles(e)) return
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'
  }, { capture: true })

  window.addEventListener('dragleave', (e) => {
    if (!hasFiles(e)) return
    dragDepth = Math.max(0, dragDepth - 1)
    if (dragDepth === 0) hideDropOverlay()
  }, { capture: true })

  window.addEventListener('drop', (e) => {
    if (!hasFiles(e)) return
    e.preventDefault()
    dragDepth = 0
    hideDropOverlay()

    // 兜底：若 700ms 内 Wails 路径链路没有回来，直接通过 WebView2 原生桥再请求一次。
    const files = [...(e.dataTransfer.items || [])]
      .filter((i) => i.kind === 'file')
      .map((i) => i.getAsFile())
      .filter(Boolean)
    if (!files.length) return
    clearTimeout(dropFallbackTimer)
    dropFallbackTimer = setTimeout(() => {
      if (window.chrome?.webview?.postMessageWithAdditionalObjects) {
        window.chrome.webview.postMessageWithAdditionalObjects(
          `file:drop:${Math.round(e.x)}:${Math.round(e.y)}`,
          files,
        )
      } else {
        toast('无法读取拖入内容的路径，请使用「选择仓库目录」按钮', 'warn', 5000)
      }
    }, 700)
  }, { capture: true })

  window.addEventListener('dragend', () => { dragDepth = 0; hideDropOverlay() }, { capture: true })
}

// ---------------------------------------------------------------------------
// 启动
// ---------------------------------------------------------------------------

function initShortcuts() {
  window.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      if (gitignore.isGitignoreOpen()) { gitignore.closeGitignore(); return }
      if (!$('#settings-modal').hidden) { closeSettings(); return }
      if (!$('#workspace').hidden && !gitview.hasBusy()) {
        $('#branch-create').hidden = true
      }
    }
    if (e.key === 'F5' && $('#workspace').hidden === false) {
      e.preventDefault()
      void gitview.refreshStatus({ fetch: true })
    }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'o') {
      e.preventDefault()
      showWelcome()
      gitview.closeRepo()
    }
  })
}

function waitForWails() {
  return new Promise((resolve) => {
    const start = Date.now()
    const tick = () => {
      if (window.go?.main?.App) return resolve(true)
      if (Date.now() - start > 4000) return resolve(false)
      setTimeout(tick, 30)
    }
    tick()
  })
}

async function boot() {
  initUI()
  applyTheme('dark')
  initSettings()
  initShortcuts()
  gitview.initGitView()
  gitignore.initGitignore({
    onSaved: (dir) => {
      const root = gitview.getRoot()
      if (root && !$('#workspace').hidden && root.toLowerCase() === String(dir).toLowerCase()) {
        void gitview.refreshStatus()
      }
    },
  })

  $('#ws-ignore').addEventListener('click', () => {
    const dir = gitview.getRoot()
    if (dir) void gitignore.openGitignore(dir)
  })
  $('#ws-back').addEventListener('click', () => {
    gitview.closeRepo()
    showWelcome()
  })
  $('#ws-reveal').addEventListener('click', async () => {
    const dir = gitview.getRoot()
    if (!dir) return
    try { await App.RevealInExplorer(dir) } catch (err) { toast(String(err?.message || err), 'error') }
  })
  $('#ws-theme').addEventListener('click', async () => {
    const next = settings.theme === 'light' ? 'dark' : 'light'
    try {
      settings = await App.SaveSettings({ ...settings, theme: next })
      applyTheme(settings.theme)
    } catch (err) { toast(String(err?.message || err), 'error') }
  })

  welcome.initWelcome({
    onOpenRepo: (path) => openFromPath(path),
    onSettings: openSettings,
    onEditIgnore: (path) => gitignore.openGitignore(path),
  })

  const ready = await waitForWails()
  if (!ready) {
    toast('Wails 运行时未就绪，请通过 wails dev 或打包后的程序打开', 'warn', 8000)
    return
  }

  try {
    settings = await App.GetSettings()
    applyTheme(settings.theme)
  } catch { /* 使用默认设置 */ }

  initDragAndDrop()
  await welcome.refreshGitInfo()
  await welcome.renderRecent()

  // 命令行参数 / 拖到 exe 图标上启动
  try {
    const startupDir = await App.GetStartupDir()
    if (startupDir) {
      const ok = await openFromPath(startupDir, { quiet: true })
      if (!ok) applyWindowSize('welcome') // 启动参数无效时回到紧凑窗口
    }
  } catch { /* 忽略无效的启动参数 */ }
}

void boot()
