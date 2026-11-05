import { App } from './api.js'
import { $, el, toast } from './ui.js'

const FOLDER_ICON = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>'
const FILE_ICON = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/></svg>'

const STATE_LABEL = {
  untracked: '未跟踪',
  ignored: '已忽略',
  modified: '已修改',
  tracked: '已跟踪',
  rule: '仅规则',
}

let targetRoot = ''
let entries = [] // { name, isDir, ignored, state, tracked, checked }
let filter = 'all'
let query = ''
let onSaved = () => {}

export function initGitignore(handlers = {}) {
  onSaved = handlers.onSaved || onSaved
  $('#ignore-close').addEventListener('click', closeGitignore)
  $('#ignore-cancel').addEventListener('click', closeGitignore)
  $('#ignore-modal').addEventListener('mousedown', (e) => {
    if (e.target === $('#ignore-modal')) closeGitignore()
  })
  $('#ignore-save').addEventListener('click', () => void save())
  $('#ignore-select-all').addEventListener('click', () => setVisible(true))
  $('#ignore-select-none').addEventListener('click', () => setVisible(false))
  $('#ignore-search').addEventListener('input', (e) => {
    query = e.target.value.trim().toLowerCase()
    render()
  })
  $('#ignore-filter').addEventListener('change', (e) => {
    filter = e.target.value
    render()
  })
}

export function closeGitignore() { $('#ignore-modal').hidden = true }

export function isGitignoreOpen() { return !$('#ignore-modal').hidden }

export async function openGitignore(dir) {
  if (!dir) return
  try {
    const ov = await App.GetGitignoreOverview(dir)
    targetRoot = ov.root || ov.dir
    entries = (ov.entries || []).map((e) => ({ ...e, checked: !!e.ignored }))
    filter = 'all'
    query = ''
    $('#ignore-search').value = ''
    $('#ignore-filter').value = 'all'

    const dirNode = $('#ignore-dir')
    dirNode.textContent = `${targetRoot}/.gitignore`
    dirNode.title = `${targetRoot}/.gitignore`

    const state = $('#ignore-state')
    state.className = 'ignore-state' + (ov.exists ? ' exists' : '')
    state.textContent = ov.exists
      ? '已存在 .gitignore，取消勾选即可把文件移出忽略'
      : '当前仓库没有 .gitignore，保存时会自动创建'

    const count = (s) => entries.filter((e) => e.state === s).length
    $('#ignore-counts').textContent =
      `未跟踪 ${count('untracked')} · 已忽略 ${count('ignored')} · 已跟踪 ${count('tracked') + count('modified')}` +
      (ov.truncated ? ' · 仅显示前 600 项' : '')

    render()
    $('#ignore-modal').hidden = false
    $('#ignore-search').focus()
  } catch (err) {
    toast(String(err?.message || err), 'error', 5000)
  }
}

function visibleEntries() {
  return entries.filter((e) =>
    (filter === 'all' || e.state === filter) &&
    (!query || e.name.toLowerCase().includes(query)))
}

function setVisible(value) {
  const shown = visibleEntries()
  if (!shown.length) return
  if (value && shown.some((e) => e.tracked && !e.checked)) {
    toast('全选只作用于当前筛选结果；已跟踪文件会按下方开关处理', 'info', 4200)
  }
  for (const item of shown) item.checked = value
  render()
}

function render() {
  const list = $('#ignore-list')
  list.textContent = ''
  const shown = visibleEntries()
  if (!shown.length) {
    list.appendChild(el('div', {
      class: 'ignore-list-empty',
      text: entries.length ? '没有匹配的文件' : '这个仓库没有可管理的文件',
    }))
    return
  }

  for (const item of shown) {
    const box = el('input', { type: 'checkbox' })
    box.checked = item.checked
    const row = el('label', {
      class: 'ignore-item',
      title: `${item.name}${item.tracked ? '（含已跟踪文件）' : ''}`,
    }, [
      box,
      el('span', { class: 'ii-icon', html: item.isDir ? FOLDER_ICON : FILE_ICON }),
      el('span', { class: 'ii-name', text: item.isDir ? `${item.name}/` : item.name }),
      el('span', { class: `ii-state s-${item.state}`, text: STATE_LABEL[item.state] || item.state }),
      item.checked !== item.ignored
        ? el('span', { class: 'ii-pending', text: item.checked ? '待加入' : '待移出' })
        : null,
    ])

    box.addEventListener('change', () => {
      item.checked = box.checked
      const pending = row.querySelector('.ii-pending')
      if (pending) pending.remove()
      if (item.checked !== item.ignored) {
        row.appendChild(el('span', {
          class: 'ii-pending', text: item.checked ? '待加入' : '待移出',
        }))
      }
    })
    list.appendChild(row)
  }
}

async function save() {
  if (!targetRoot) return
  // 保存的是全量快照：筛选/搜索不会丢掉未显示条目的勾选状态
  const checked = entries.filter((e) => e.checked).map((e) => e.name)
  const untrack = $('#ignore-untrack').checked
  const btn = $('#ignore-save')
  btn.disabled = true
  try {
    const result = await App.SaveGitignore(targetRoot, checked, untrack)
    const parts = [result?.created ? '已创建 .gitignore' : '已更新 .gitignore']
    if (result?.added) parts.push(`加入 ${result.added} 项`)
    if (result?.removed) parts.push(`移出 ${result.removed} 项`)
    toast(parts.join(' · '), 'ok')
    if (result?.untracked?.length) {
      toast(`${result.untracked.length} 个已跟踪文件已从 Git 索引移除（文件保留），请在下次提交中确认`, 'warn', 6500)
    }
    for (const warning of result?.warnings || []) {
      toast('未能取消跟踪：' + warning, 'warn', 6500)
    }
    closeGitignore()
    onSaved(targetRoot)
  } catch (err) {
    toast(String(err?.message || err), 'error', 5000)
  } finally {
    btn.disabled = false
  }
}
