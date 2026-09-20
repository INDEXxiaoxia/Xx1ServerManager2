<script setup>
import {ref, watch, onMounted, onBeforeUnmount, nextTick} from 'vue'
import {useRoute} from 'vue-router'
import {ElMessage} from 'element-plus'
import {Terminal} from 'xterm'
import {FitAddon} from '@xterm/addon-fit'
import 'xterm/css/xterm.css'
import {api, events} from '@/api'
import {themeState, getTerminalTheme} from '@/theme'

const route = useRoute()
const servers = ref([])
const tabs = ref([])         // {key, serverId, serverName, title, termId}
const activeKey = ref('')
const terms = new Map()      // key -> xterm instance
const fitAddons = new Map()  // key -> fit addon
const dialogVisible = ref(false)
const selectedServer = ref('')
const tabsRef = ref(null)

// 右键菜单
const menuVisible = ref(false)
const menuPos = ref({x: 0, y: 0})
const hasSelection = ref(false)

async function loadServers() {
  servers.value = await api.listServers()
}

async function openNew(serverId) {
  const sv = servers.value.find(s => s.id === serverId)
  if (!sv) return
  const key = 't' + Date.now() + Math.random().toString(36).slice(2, 6)
  tabs.value.push({key, serverId, serverName: sv.name, title: sv.name})
  activeKey.value = key
  closeMenu()
  await nextTick()
  initTerminal(key)
  // 新标签可能把标签栏撑到可视区外，滚动到可见位置
  const el = document.getElementById('tab-' + key)
  if (el && el.scrollIntoView) el.scrollIntoView({block: 'nearest', inline: 'nearest'})
}

async function initTerminal(key) {
  const tab = tabs.value.find(t => t.key === key)
  const el = document.getElementById('term-' + key)
  if (!el) return
  const term = new Terminal({
    fontSize: 13,
    fontFamily: 'Cascadia Code, Consolas, monospace',
    cursorBlink: true,
    // 保留鼠标拖拽选择文本的能力（关掉后左键选择交给 xterm，而不是被应用独占）
    rightClickSelectsWord: false,
    theme: getTerminalTheme(themeState.name)
  })
  const fit = new FitAddon()
  term.loadAddon(fit)
  term.open(el)
  fit.fit()
  terms.set(key, term)
  fitAddons.set(key, fit)

  // 右键：弹出 复制 / 粘贴 菜单（同时阻止 WebView 默认菜单）
  term.element?.addEventListener('contextmenu', (ev) => {
    ev.preventDefault()
    openMenu(ev, term)
  })

  // Ctrl+Shift+C 复制选中内容（Ctrl+C 保持为发送给远端的中断信号 SIGINT）
  term.attachCustomKeyEventHandler((ev) => {
    if (ev.type === 'keydown' && ev.ctrlKey && ev.shiftKey && (ev.key === 'C' || ev.key === 'c')) {
      copySelection(term)
      return false
    }
    return true
  })

  const cols = term.cols
  const rows = term.rows
  const termId = await api.terminalOpen(tab.serverId, cols, rows)
  tab.termId = termId

  // 输出 -> 终端
  events.on('terminal:data:' + termId, (data) => {
    term.write(data)
  })
  events.on('terminal:close:' + termId, () => {
    // 会话关闭
  })

  // 输入 -> 后端
  term.onData((data) => {
    api.terminalWrite(termId, data)
  })

  // 尺寸变化
  const onResize = () => {
    try {
      fit.fit()
      api.terminalResize(termId, term.cols, term.rows)
    } catch (e) { /* ignore */ }
  }
  window.addEventListener('resize', onResize)
  tab._resize = onResize
}

// ---- 复制 / 粘贴 ----
function activeTerm() {
  return terms.get(activeKey.value) || null
}

function writeToTerm(text) {
  const tab = tabs.value.find(t => t.key === activeKey.value)
  const term = activeTerm()
  if (!tab || !tab.termId || !text) return
  // 走与键盘输入同一条通道，远端会按 bracketed paste 模式处理
  api.terminalWrite(tab.termId, text)
  term?.focus()
}

// 复制：优先 Clipboard API，失败回退到临时 textarea + execCommand
async function copySelection(term = activeTerm()) {
  const text = term ? term.getSelection() : ''
  if (!text) {
    ElMessage.warning('请先用鼠标选中要复制的内容')
    return false
  }
  if (await writeClipboard(text)) {
    ElMessage.success('已复制到剪贴板')
    return true
  }
  ElMessage.error('复制失败，请重试')
  return false
}

async function writeClipboard(text) {
  if (navigator.clipboard && navigator.clipboard.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch (e) { /* 继续回退 */ }
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
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

// 粘贴：优先 Clipboard API；不可用时退回到「窗口内粘贴」，仍支持 Ctrl+V 落到光标处
async function pasteClipboard() {
  closeMenu()
  if (navigator.clipboard && navigator.clipboard.readText) {
    try {
      const text = await navigator.clipboard.readText()
      if (text) {
        writeToTerm(text)
        return
      }
      ElMessage.warning('剪贴板为空')
      return
    } catch (e) {
      // 落到回退方案
    }
  }
  focusTerminalForPaste()
  ElMessage.info('当前环境无法直接读取系统剪贴板，请按 Ctrl+V 粘贴')
}

// 聚焦隐藏输入框，让 Ctrl+V 产生的粘贴事件能把内容写进终端
let pasteInput = null
function focusTerminalForPaste() {
  if (!pasteInput) {
    pasteInput = document.createElement('textarea')
    pasteInput.setAttribute('aria-hidden', 'true')
    pasteInput.style.position = 'fixed'
    pasteInput.style.top = '-1000px'
    pasteInput.style.opacity = '0'
    document.body.appendChild(pasteInput)
  }
  pasteInput.value = ''
  pasteInput.focus()
}

function onPasteEvent(ev) {
  const text = (ev.clipboardData && ev.clipboardData.getData('text')) || ''
  if (!text) return
  ev.preventDefault()
  writeToTerm(text)
  if (pasteInput) pasteInput.blur()
}

function openMenu(ev, term) {
  menuPos.value = {x: ev.clientX, y: ev.clientY}
  hasSelection.value = !!term.getSelection()
  menuVisible.value = true
}

function closeMenu() {
  menuVisible.value = false
}

function onMenuCopy() {
  const term = activeTerm()
  closeMenu()
  copySelection(term)
}

function onMenuPaste() {
  pasteClipboard()
}

function onMenuSelectAll() {
  const term = activeTerm()
  closeMenu()
  if (term && term.selectAll) {
    term.selectAll()
    ElMessage.success('已全选当前屏幕内容，可再次右键复制')
  } else {
    ElMessage.warning('当前终端不支持全选')
  }
}

function closeTab(key) {
  const tab = tabs.value.find(t => t.key === key)
  if (!tab) return
  if (tab.termId) api.terminalClose(tab.termId)
  if (tab._resize) window.removeEventListener('resize', tab._resize)
  const term = terms.get(key)
  if (term) term.dispose()
  terms.delete(key)
  fitAddons.delete(key)
  tabs.value = tabs.value.filter(t => t.key !== key)
  if (activeKey.value === key) {
    activeKey.value = tabs.value.length ? tabs.value[tabs.value.length - 1].key : ''
  }
}

function addTab() {
  if (!servers.value.length) { ElMessage.warning('请先在服务器列表添加服务器'); return }
  if (route.query.serverId && servers.value.some(s => s.id === route.query.serverId)) {
    openNew(route.query.serverId)
  } else {
    selectedServer.value = ''
    dialogVisible.value = true
  }
}

async function confirmAdd() {
  if (!selectedServer.value) { ElMessage.warning('请选择服务器'); return }
  dialogVisible.value = false
  openNew(selectedServer.value)
}

// 主题切换：对已打开的终端会话即时应用新配色
watch(() => themeState.name, () => {
  const t = getTerminalTheme(themeState.name)
  terms.forEach(term => {
    term.options.theme = t
  })
})

onMounted(async () => {
  // 粘贴事件走「剪贴板 API 不可用」时的回退路径（隐藏输入框承载 Ctrl+V）
  document.addEventListener('paste', onPasteEvent)
  document.addEventListener('keydown', onGlobalKeydown)
  await loadServers()
  addTab()
})

onBeforeUnmount(() => {
  document.removeEventListener('paste', onPasteEvent)
  document.removeEventListener('keydown', onGlobalKeydown)
  if (pasteInput && pasteInput.parentNode) pasteInput.parentNode.removeChild(pasteInput)
  pasteInput = null
  tabs.value.forEach(t => {
    if (t.termId) api.terminalClose(t.termId)
    if (t._resize) window.removeEventListener('resize', t._resize)
  })
  terms.forEach(t => t.dispose())
})

// Esc 收起右键菜单
function onGlobalKeydown(ev) {
  if (ev.key === 'Escape') closeMenu()
}
</script>

<template>
  <div class="terminal-page">
    <!-- 标签栏：新建按钮跟随在最后一个标签右侧，标签过多时贴着标签栏最右侧 -->
    <div class="term-tabbar">
      <div ref="tabsRef" class="term-tabs">
        <div
          v-for="t in tabs"
          :id="'tab-' + t.key"
          :key="t.key"
          class="term-tab"
          :class="{active: activeKey === t.key}"
          @click="activeKey = t.key"
        >
          <span class="tab-title">{{ t.title }}</span>
          <el-icon class="tab-close" @click.stop="closeTab(t.key)"><Close /></el-icon>
        </div>
        <el-tooltip content="新建终端" placement="bottom">
          <button class="tab-add" @click="addTab">
            <el-icon><Plus /></el-icon>
          </button>
        </el-tooltip>
      </div>
    </div>

    <div class="term-body">
      <div v-for="t in tabs" :key="t.key" class="term-pane" :class="{active: activeKey === t.key}">
        <div :id="'term-' + t.key" class="term-container" />
      </div>
      <div v-if="!tabs.length" class="term-empty">
        <el-empty description="还没有打开终端，点击标签栏的 + 新建" :image-size="70" />
      </div>
    </div>

    <!-- 右键菜单：复制 / 粘贴 / 全选 -->
    <div v-if="menuVisible" class="ctx-mask" @click="closeMenu" @contextmenu.prevent="closeMenu"></div>
    <div
      v-if="menuVisible"
      class="ctx-menu"
      :style="{left: menuPos.x + 'px', top: menuPos.y + 'px'}"
    >
      <div class="ctx-item" :class="{disabled: !hasSelection}" @click="onMenuCopy">
        <el-icon><CopyDocument /></el-icon>复制
        <span class="ctx-hint">Ctrl+Shift+C</span>
      </div>
      <div class="ctx-item" @click="onMenuPaste">
        <el-icon><DocumentCopy /></el-icon>粘贴
        <span class="ctx-hint">Ctrl+V</span>
      </div>
      <div class="ctx-item" @click="onMenuSelectAll">
        <el-icon><Select /></el-icon>全选屏幕内容
      </div>
    </div>

    <el-dialog v-model="dialogVisible" title="选择服务器" width="420px" @closed="closeMenu">
      <el-select v-model="selectedServer" placeholder="选择服务器" style="width: 100%">
        <el-option v-for="s in servers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
      </el-select>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="confirmAdd">连接</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.terminal-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--terminal-bg);
  position: relative;
}
.term-tabbar {
  display: flex;
  align-items: stretch;
  background: var(--panel-2);
  border-bottom: 1px solid var(--line);
}
.term-tabs {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: stretch;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
}
.term-tabs::-webkit-scrollbar { height: 4px; }
.term-tabs::-webkit-scrollbar-thumb { background: var(--sel-thumb); border-radius: 999px; }
.term-tab {
  position: relative;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 14px;
  cursor: pointer;
  color: var(--ink-2);
  border-right: 1px solid var(--line);
  user-select: none;
  flex: 0 0 auto;
  transition: background-color .2s ease, color .2s ease;
}
.term-tab:hover {
  color: var(--ink);
}
.term-tab.active {
  background: var(--terminal-bg);
  color: var(--ink);
}
.term-tab.active::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 2px;
  background: var(--blue);
}
.term-tab .tab-title { font-size: 13px; max-width: 160px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.term-tab .tab-close:hover { color: var(--red); }
/* 新建按钮不单独占行：紧跟在最后一个标签右侧；标签过多被挤出时自动贴在最右 */
.tab-add {
  flex: 0 0 auto;
  width: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: transparent;
  color: var(--ink-3);
  cursor: pointer;
  border-right: 1px solid var(--line);
  transition: background-color .2s ease, color .2s ease;
}
.tab-add:hover {
  background: var(--hover-bg);
  color: var(--blue);
}
.term-body {
  flex: 1;
  position: relative;
  min-height: 0;
}
.term-pane {
  position: absolute;
  inset: 0;
  display: none;
}
.term-pane.active { display: block; }
.term-container {
  width: 100%;
  height: 100%;
  padding: 4px;
}
.term-empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

/* 右键菜单 */
.ctx-mask {
  position: fixed;
  inset: 0;
  z-index: 2000;
}
.ctx-menu {
  position: fixed;
  z-index: 2001;
  min-width: 190px;
  padding: 4px;
  background: var(--card);
  border: 1px solid var(--line);
  border-radius: 10px;
  box-shadow: var(--el-box-shadow, 0 8px 24px rgba(0, 0, 0, .18));
}
.ctx-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 10px;
  font-size: 13px;
  color: var(--ink);
  border-radius: 7px;
  cursor: pointer;
  transition: background-color .15s ease;
}
.ctx-item:hover { background: var(--hover-bg); }
.ctx-item.disabled {
  color: var(--ink-3);
  cursor: not-allowed;
}
.ctx-item.disabled:hover { background: transparent; }
.ctx-hint {
  margin-left: auto;
  font-size: 11px;
  color: var(--ink-3);
}
</style>