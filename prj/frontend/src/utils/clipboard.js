// 复制到剪贴板：优先 Clipboard API，失败回退到隐藏 textarea + execCommand。
// Wails 的 WebView 在部分环境下 navigator.clipboard 不可用（或不在安全上下文），
// 因此保留回退路径（与 SSH 终端页里的实现一致）。
export async function writeClipboard(text) {
  const s = String(text == null ? '' : text)
  if (!s) return false
  if (navigator.clipboard && navigator.clipboard.writeText) {
    try {
      await navigator.clipboard.writeText(s)
      return true
    } catch (e) { /* 继续回退 */ }
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = s
    ta.setAttribute('readonly', '')
    ta.style.position = 'fixed'
    ta.style.top = '-1000px'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch (e) {
    return false
  }
}

// 体积展示：后端统一给 MB
export function fmtMB(mb) {
  const v = Number(mb) || 0
  if (v >= 1024 * 1024) return (v / 1024 / 1024).toFixed(2) + ' TB'
  if (v >= 1024) return (v / 1024).toFixed(2) + ' GB'
  return Math.round(v) + ' MB'
}

// 百分比展示：最多两位小数，去掉无意义的 .00
export function fmtPct(v) {
  const n = Number(v) || 0
  return String(Number(n.toFixed(2))) + '%'
}