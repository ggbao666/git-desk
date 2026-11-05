import { App } from './api.js'
import { $, el, basename, formatTime, toast, confirmDialog } from './ui.js'

let onOpenRepo = () => {}
let onSettings = () => {}
let onEditIgnore = () => {}

export function initWelcome(handlers = {}) {
  onOpenRepo = handlers.onOpenRepo || onOpenRepo
  onSettings = handlers.onSettings || onSettings
  onEditIgnore = handlers.onEditIgnore || onEditIgnore

  $('#wc-pick').addEventListener('click', async () => {
    try {
      const path = await App.PickDirectory()
      if (!path) return
      await onOpenRepo(path)
    } catch (err) {
      toast(String(err?.message || err), 'error')
    }
  })

  $('#wc-settings').addEventListener('click', onSettings)

  // 启动页也能选仓库编辑 .gitignore（后端会自动定位到仓库根并校验）
  $('#wc-ignore').addEventListener('click', async () => {
    try {
      const path = await App.PickDirectory()
      if (!path) return
      onEditIgnore(path)
    } catch (err) {
      toast(String(err?.message || err), 'error')
    }
  })

  $('#wc-recent-clear').addEventListener('click', async () => {
    const ok = await confirmDialog('确定清空所有最近打开记录？此操作不会删除任何仓库文件。', {
      title: '清空历史记录', okText: '清空', danger: true,
    })
    if (!ok) return
    try {
      await App.ClearRecentDirs()
      await renderRecent()
      toast('已清空历史记录', 'ok')
    } catch (err) {
      toast(String(err?.message || err), 'error')
    }
  })

  // 拖拽时只负责视觉反馈，真实路径由全局 Wails 拖放事件统一处理
  const zone = $('#drop-zone')
  ;['dragenter', 'dragover'].forEach((type) => {
    zone.addEventListener(type, (e) => { e.preventDefault(); zone.classList.add('is-over') })
  })
  zone.addEventListener('dragleave', (e) => {
    if (!zone.contains(e.relatedTarget)) zone.classList.remove('is-over')
  })
  zone.addEventListener('drop', (e) => { e.preventDefault(); zone.classList.remove('is-over') })

  void renderRecent()
  void refreshGitInfo()
}

export function setDropActive(active) {
  $('#drop-zone')?.classList.toggle('is-over', active)
}

function setGitInfo(node, state, first, second, title) {
  node.className = `git-info ${state}`
  node.innerHTML = ''
  node.appendChild(el('div', { class: 'git-info-row' }, [
    el('span', { class: 'dot' }),
    el('span', { class: 'git-info-text', text: first }),
  ]))
  node.appendChild(el('div', { class: 'git-info-row sub' }, [
    el('span', { class: 'git-info-text', text: second }),
  ]))
  node.title = title || `${first}\n${second}`
}

export async function refreshGitInfo() {
  const info = $('#wc-git-info')
  if (!info) return
  try {
    const r = await App.GitDetect(false)
    if (r?.ok) {
      setGitInfo(info, 'ok', r.version, r.path,
        `${r.version}\n${r.path}\n来源：${r.source === 'auto' ? '自动探测' : '手动指定'}`)
    } else {
      setGitInfo(info, 'bad', r?.error || '未检测到 Git', '可在「设置」中手动指定 Git 路径')
    }
  } catch (err) {
    setGitInfo(info, 'bad', 'Git 检测失败：' + String(err?.message || err), '请在打包后的程序中运行，或通过「设置」指定 Git 路径')
  }
}

export async function renderRecent() {
  const list = $('#recent-list')
  const empty = $('#recent-empty')
  if (!list || !empty) return
  let items = []
  try {
    items = (await App.GetRecentDirs()) || []
  } catch {
    items = []
  }
  list.textContent = ''
  empty.hidden = items.length > 0

  for (const item of items) {
    const main = el('div', { class: 'recent-main' }, [
      el('div', { class: 'recent-name', text: item.name || basename(item.path), title: item.path }),
      el('div', { class: 'recent-path', text: item.path, title: item.path }),
      el('div', { class: 'recent-time', text: formatTime(item.openedAt) }),
    ])
    main.addEventListener('click', () => void onOpenRepo(item.path))

    const del = el('button', { class: 'recent-del', text: '×', title: '从历史中移除' })
    del.addEventListener('click', async (e) => {
      e.stopPropagation()
      try {
        await App.RemoveRecentDir(item.path)
        await renderRecent()
      } catch (err) {
        toast(String(err?.message || err), 'error')
      }
    })

    const icon = el('span', { class: 'ri-icon', html: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>' })
    list.appendChild(el('div', { class: 'recent-item' }, [icon, main, del]))
  }
}
