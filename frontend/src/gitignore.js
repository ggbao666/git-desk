import { App } from './api.js'
import { $, $$, el, toast } from './ui.js'

const FOLDER_ICON = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>'
const FILE_ICON = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/></svg>'

let targetDir = ''
let entries = []
let onSaved = () => {}

export function initGitignore(handlers = {}) {
  onSaved = handlers.onSaved || onSaved
  $('#ignore-close').addEventListener('click', closeGitignore)
  $('#ignore-cancel').addEventListener('click', closeGitignore)
  $('#ignore-modal').addEventListener('mousedown', (e) => {
    if (e.target === $('#ignore-modal')) closeGitignore()
  })
  $('#ignore-save').addEventListener('click', () => void save())
  $('#ignore-select-all').addEventListener('click', () => setAll(true))
  $('#ignore-select-none').addEventListener('click', () => setAll(false))
}

export function closeGitignore() { $('#ignore-modal').hidden = true }

export function isGitignoreOpen() { return !$('#ignore-modal').hidden }

function setAll(value) {
  for (const box of $$('#ignore-list input[type="checkbox"]')) box.checked = value
}

export async function openGitignore(dir) {
  if (!dir) return
  try {
    const ov = await App.GetGitignoreOverview(dir)
    targetDir = ov.dir
    entries = ov.entries || []

    const dirNode = $('#ignore-dir')
    dirNode.textContent = ov.dir
    dirNode.title = ov.dir

    const state = $('#ignore-state')
    state.className = 'ignore-state' + (ov.exists ? ' exists' : '')
    state.textContent = ov.exists
      ? '已存在 .gitignore，取消勾选即可移除对应规则'
      : '当前目录没有 .gitignore，保存时会自动创建'

    render()
    $('#ignore-modal').hidden = false
  } catch (err) {
    toast(String(err?.message || err), 'error', 5000)
  }
}

function render() {
  const list = $('#ignore-list')
  list.textContent = ''
  if (!entries.length) {
    list.appendChild(el('div', { class: 'ignore-list-empty', text: '这个目录下没有可管理的文件' }))
    return
  }
  for (const item of entries) {
    const box = el('input', { type: 'checkbox' })
    box.value = item.name
    box.checked = !!item.ignored
    const row = el('label', { class: 'ignore-item', title: item.name }, [
      box,
      el('span', { class: 'ii-icon', html: item.isDir ? FOLDER_ICON : FILE_ICON }),
      el('span', { class: 'ii-name', text: item.isDir ? `${item.name}/` : item.name }),
      item.ignored ? el('span', { class: 'ii-tag', text: '已忽略' }) : null,
    ])
    box.addEventListener('change', () => {
      const tag = row.querySelector('.ii-tag')
      if (box.checked && !tag) row.appendChild(el('span', { class: 'ii-tag', text: '待加入' }))
      else if (!box.checked && tag) tag.remove()
    })
    list.appendChild(row)
  }
}

async function save() {
  if (!targetDir) return
  const checked = $$('#ignore-list input[type="checkbox"]')
    .filter((box) => box.checked)
    .map((box) => box.value)
  const btn = $('#ignore-save')
  btn.disabled = true
  try {
    const result = await App.SaveGitignore(targetDir, checked)
    toast(result?.created ? '已创建 .gitignore' : '已更新 .gitignore', 'ok')
    closeGitignore()
    onSaved(targetDir)
  } catch (err) {
    toast(String(err?.message || err), 'error', 5000)
  } finally {
    btn.disabled = false
  }
}
