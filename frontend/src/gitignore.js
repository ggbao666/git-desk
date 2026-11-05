import { App } from './api.js'
import { $, el, basename, toast } from './ui.js'

const FOLDER_ICON = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>'
const FILE_ICON = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/></svg>'
const CARET_ICON = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 6l6 6-6 6"/></svg>'

const STATE_LABEL = {
  untracked: '未跟踪',
  ignored: '已忽略',
  modified: '已修改',
  tracked: '已跟踪',
  rule: '仅规则',
}

const keyOf = (name) => String(name).toLowerCase()

let targetRoot = ''
let entries = [] // 扁平池：{ name, isDir, ignored, state, tracked, checked }
let byKey = new Map()
let expanded = new Set() // 默认空 = 所有目录折叠
let loaded = new Set()   // 已向后端请求过子项的目录
let loading = new Set()
let filter = 'all'
let query = ''
let truncated = false
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
    truncated = !!ov.truncated
    expanded = new Set()
    loaded = new Set()
    loading = new Set()
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

    rebuildIndex()
    render()
    $('#ignore-modal').hidden = false
    $('#ignore-search').focus()
  } catch (err) {
    toast(String(err?.message || err), 'error', 5000)
  }
}

// 建索引时补齐缺失的中间目录（例如列表被 600 条上限截断），保证深层路径一定挂得上
function rebuildIndex() {
  byKey = new Map()
  for (const item of entries) byKey.set(keyOf(item.name), item)
  for (const item of [...entries]) {
    const parts = item.name.split('/')
    for (let i = 1; i < parts.length; i++) {
      const parent = parts.slice(0, i).join('/')
      if (byKey.has(keyOf(parent))) continue
      const synthetic = {
        name: parent, isDir: true, ignored: item.ignored,
        state: item.ignored ? 'ignored' : 'rule', tracked: false,
        checked: !!item.ignored,
      }
      entries.push(synthetic)
      byKey.set(keyOf(parent), synthetic)
    }
  }
}

function childrenOf(parent) {
  const prefix = parent ? parent + '/' : ''
  const out = []
  for (const item of entries) {
    if (!item.name.startsWith(prefix)) continue
    const rest = item.name.slice(prefix.length)
    if (!rest || rest.includes('/')) continue
    out.push(item)
  }
  out.sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
    return keyOf(a.name).localeCompare(keyOf(b.name))
  })
  return out
}

function ancestorsOf(item) {
  const parts = item.name.split('/')
  return parts.slice(0, -1).map((_, i) => parts.slice(0, i + 1).join('/'))
}

function matchedItems() {
  return entries.filter((e) =>
    (filter === 'all' || e.state === filter) &&
    (!query || keyOf(e.name).includes(query)))
}

// 计算要显示的行：正常按展开状态逐层走；搜索 / 筛选时自动展开命中项的祖先路径
function visibleRows() {
  const filtering = filter !== 'all' || !!query
  const show = new Set()
  const open = new Set()
  if (filtering) {
    for (const item of matchedItems()) {
      show.add(keyOf(item.name))
      for (const ancestor of ancestorsOf(item)) {
        show.add(keyOf(ancestor))
        open.add(keyOf(ancestor))
      }
    }
  } else {
    const collect = (parent, depth) => {
      if (depth > 24) return
      for (const child of childrenOf(parent)) {
        show.add(keyOf(child.name))
        if (child.isDir && expanded.has(keyOf(child.name))) collect(child.name, depth + 1)
      }
    }
    collect('', 0)
  }

  const rows = []
  const walk = (parent, depth) => {
    if (depth > 24) return
    for (const item of childrenOf(parent)) {
      const k = keyOf(item.name)
      if (!show.has(k)) continue
      const isOpen = filtering ? open.has(k) : expanded.has(k)
      rows.push({ item, depth, open: isOpen })
      if (!item.isDir || !isOpen) continue
      const kids = childrenOf(item.name)
      if (!kids.length) {
        rows.push({
          placeholder: true, depth: depth + 1,
          text: loading.has(k) ? '正在读取目录…' : '（空目录）',
        })
      } else {
        walk(item.name, depth + 1)
      }
    }
  }
  walk('', 0)
  return rows
}

async function toggleDir(item) {
  const k = keyOf(item.name)
  if (expanded.has(k)) {
    expanded.delete(k)
    render()
    return
  }
  expanded.add(k)
  if (loaded.has(k)) { render(); return }
  loaded.add(k)
  loading.add(k)
  render()
  try {
    const children = await App.GetGitignoreChildren(targetRoot, item.name)
    if ((children || []).length >= 400) {
      toast('该目录内容较多，仅显示前 400 项', 'info', 4000)
    }
    mergeChildren(children)
  } catch (err) {
    loaded.delete(k) // 允许再次点开时重试
    toast(String(err?.message || err), 'error', 4000)
  } finally {
    loading.delete(k)
    render()
  }
}

function mergeChildren(children) {
  let added = 0
  for (const e of children || []) {
    const k = keyOf(e.name)
    if (byKey.has(k)) continue
    const item = { ...e, checked: !!e.ignored }
    entries.push(item)
    byKey.set(k, item)
    added++
  }
  if (added) rebuildIndex()
}

function renderRow({ item, depth, open }) {
  const box = el('input', { type: 'checkbox' })
  box.checked = item.checked
  const main = el('label', { class: 'ii-main' }, [
    box,
    el('span', { class: 'ii-icon', html: item.isDir ? FOLDER_ICON : FILE_ICON }),
    el('span', {
      class: 'ii-name',
      text: basename(item.name) + (item.isDir ? '/' : ''),
      title: item.name,
    }),
  ])
  const row = el('div', {
    class: 'ignore-item',
    style: `padding-left:${9 + depth * 16}px`,
    title: `${item.name}${item.tracked ? '（含已跟踪文件）' : ''}`,
  }, [
    item.isDir
      ? el('button', {
          class: 'ii-caret' + (open ? ' is-open' : ''),
          type: 'button',
          title: open ? '折叠子内容' : '展开子内容',
          'aria-label': open ? '折叠子内容' : '展开子内容',
          html: CARET_ICON,
          onclick: (e) => { e.preventDefault(); e.stopPropagation(); void toggleDir(item) },
        })
      : el('span', { class: 'ii-caret-spacer' }),
    main,
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
  return row
}

function updateCounts() {
  const count = (s) => entries.filter((e) => e.state === s).length
  $('#ignore-counts').textContent =
    `未跟踪 ${count('untracked')} · 已忽略 ${count('ignored')} · 已跟踪 ${count('tracked') + count('modified')}` +
    (truncated ? ' · 仅显示前 600 项' : '')
}

function render() {
  const list = $('#ignore-list')
  const scroll = list.scrollTop
  const rows = visibleRows()
  list.textContent = ''
  if (!rows.length) {
    list.appendChild(el('div', {
      class: 'ignore-list-empty',
      text: entries.length ? '没有匹配的文件' : '这个仓库没有可管理的文件',
    }))
  } else {
    for (const row of rows) {
      list.appendChild(row.placeholder
        ? el('div', {
            class: 'ignore-tree-empty',
            style: `padding-left:${9 + row.depth * 16}px`,
            text: row.text,
          })
        : renderRow(row))
    }
  }
  list.scrollTop = scroll
  updateCounts()
}

// 全选 / 全不选只作用于当前显示出来的行，避免误改未展开目录里的文件
function setVisible(value) {
  const shown = visibleRows().filter((r) => !r.placeholder).map((r) => r.item)
  if (!shown.length) return
  if (value && shown.some((e) => e.tracked && !e.checked)) {
    toast('全选只作用于当前显示的文件；已跟踪文件会按下方开关处理', 'info', 4200)
  }
  for (const item of shown) item.checked = value
  render()
}

async function save() {
  if (!targetRoot) return
  // 保存的是全量快照：筛选/搜索/折叠都不会丢掉已加载条目的勾选状态
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
      toast(warning, 'warn', 6500)
    }
    closeGitignore()
    onSaved(targetRoot)
  } catch (err) {
    toast(String(err?.message || err), 'error', 5000)
  } finally {
    btn.disabled = false
  }
}
