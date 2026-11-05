export const $ = (sel, root = document) => root.querySelector(sel)
export const $$ = (sel, root = document) => [...root.querySelectorAll(sel)]

export function el(tag, attrs = {}, children = []) {
  const node = document.createElement(tag)
  for (const [key, value] of Object.entries(attrs)) {
    if (value === undefined || value === null) continue
    if (key === 'class') node.className = value
    else if (key === 'text') node.textContent = value
    else if (key === 'html') node.innerHTML = value
    else if (key === 'title') { node.title = value; node.setAttribute('title', value) }
    else if (key.startsWith('on') && typeof value === 'function') node.addEventListener(key.slice(2), value)
    else if (key === 'hidden') node.hidden = !!value
    else if (key === 'value') node.value = value
    else node.setAttribute(key, String(value))
  }
  for (const child of [].concat(children)) {
    if (child == null) continue
    node.appendChild(typeof child === 'string' ? document.createTextNode(child) : child)
  }
  return node
}

export function basename(p) {
  if (!p) return ''
  const parts = String(p).split(/[\\/]/).filter(Boolean)
  return parts[parts.length - 1] || p
}

export function dirName(p) {
  if (!p) return ''
  const i = Math.max(String(p).lastIndexOf('\\'), String(p).lastIndexOf('/'))
  return i > 0 ? String(p).slice(0, i) : ''
}

export function formatTime(unixSeconds) {
  if (!unixSeconds) return ''
  const d = new Date(unixSeconds * 1000)
  const now = new Date()
  const pad = (n) => String(n).padStart(2, '0')
  if (d.toDateString() === now.toDateString()) return `今天 ${pad(d.getHours())}:${pad(d.getMinutes())}`
  const yesterday = new Date(now.getTime() - 86400000)
  if (d.toDateString() === yesterday.toDateString()) return `昨天 ${pad(d.getHours())}:${pad(d.getMinutes())}`
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

export function toast(message, kind = 'info', duration = 3200) {
  const host = $('#toast-host')
  if (!host) return
  const node = el('div', { class: `toast ${kind}`, text: String(message || '') })
  host.appendChild(node)
  setTimeout(() => {
    node.style.transition = 'opacity .2s, transform .2s'
    node.style.opacity = '0'
    node.style.transform = 'translateX(12px)'
    setTimeout(() => node.remove(), 220)
  }, duration)
}

let confirmHandler = null
export function confirmDialog(message, { title = '请确认', okText = '确定', danger = false } = {}) {
  const modal = $('#confirm-modal')
  if (!modal) return Promise.resolve(window.confirm(message))
  $('#confirm-title').textContent = title
  $('#confirm-message').textContent = message
  const ok = $('#confirm-ok')
  ok.textContent = okText
  ok.className = `btn ${danger ? 'danger' : 'primary'}`
  modal.hidden = false
  return new Promise((resolve) => { confirmHandler = resolve })
}

function closeConfirm(result) {
  const modal = $('#confirm-modal')
  if (modal) modal.hidden = true
  if (confirmHandler) {
    const fn = confirmHandler
    confirmHandler = null
    fn(result)
  }
}

export function initUI() {
  $('#confirm-ok')?.addEventListener('click', () => closeConfirm(true))
  $('#confirm-cancel')?.addEventListener('click', () => closeConfirm(false))
  $('#confirm-modal')?.addEventListener('mousedown', (e) => {
    if (e.target === $('#confirm-modal')) closeConfirm(false)
  })
  window.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && $('#confirm-modal') && !$('#confirm-modal').hidden) {
      e.stopPropagation()
      closeConfirm(false)
    }
  }, true)
}
