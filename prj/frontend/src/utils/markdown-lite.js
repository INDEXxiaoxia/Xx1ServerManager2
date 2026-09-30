// 极简 Markdown 渲染：只处理 AI 回复里真正会出现的语法
// （代码块、行内代码、粗体、标题、无序列表、换行）。
// 不引入 marked 之类的依赖：这里的输入永远来自模型，只需要"不难看、不执行脚本"，
// 所有文本都先转义 HTML，再拼出受控标签 —— 不存在注入风险。

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function inline(s) {
  let t = escapeHtml(s)
  // 行内代码
  t = t.replace(/`([^`]+)`/g, '<code>$1</code>')
  // 粗体
  t = t.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  return t
}

// renderMarkdown 返回 HTML 字符串（已转义）；调用方用 v-html 渲染
export function renderMarkdown(src) {
  const text = String(src == null ? '' : src)
  if (!text) return ''
  const out = []
  const lines = text.split('\n')
  let inCode = false
  let codeBuf = []
  let listOpen = false

  const closeList = () => {
    if (listOpen) {
      out.push('</ul>')
      listOpen = false
    }
  }

  for (const raw of lines) {
    const line = raw.replace(/\r$/, '')
    const fence = line.match(/^\s*```/)
    if (fence) {
      if (inCode) {
        out.push('<pre class="md-code">' + escapeHtml(codeBuf.join('\n')) + '</pre>')
        codeBuf = []
        inCode = false
      } else {
        closeList()
        inCode = true
      }
      continue
    }
    if (inCode) {
      codeBuf.push(line)
      continue
    }
    if (!line.trim()) {
      closeList()
      continue
    }
    const h = line.match(/^(#{1,4})\s+(.*)$/)
    if (h) {
      closeList()
      out.push('<div class="md-h md-h' + h[1].length + '">' + inline(h[2]) + '</div>')
      continue
    }
    const li = line.match(/^\s*[-*]\s+(.*)$/)
    if (li) {
      if (!listOpen) {
        out.push('<ul class="md-ul">')
        listOpen = true
      }
      out.push('<li>' + inline(li[1]) + '</li>')
      continue
    }
    closeList()
    out.push('<p class="md-p">' + inline(line) + '</p>')
  }
  if (inCode && codeBuf.length) {
    out.push('<pre class="md-code">' + escapeHtml(codeBuf.join('\n')) + '</pre>')
  }
  closeList()
  return out.join('')
}