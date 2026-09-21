<script setup>
import {ref, computed, watch, onMounted, onBeforeUnmount, nextTick} from 'vue'
import {useRoute} from 'vue-router'
import {ElMessage} from 'element-plus'
import {Terminal} from 'xterm'
import {FitAddon} from '@xterm/addon-fit'
import 'xterm/css/xterm.css'
import {api, events} from '@/api'
import {themeState, getTerminalTheme} from '@/theme'
import QuickPanel from './QuickPanel.vue'

const route = useRoute()
const servers = ref([])
const tabs = ref([])         // {key, serverId, serverName, title, termId, connected}
const activeKey = ref('')
const terms = new Map()      // key -> xterm instance
const fitAddons = new Map()  // key -> fit addon
const dialogVisible = ref(false)
const selectedServer = ref('')
const tabsRef = ref(null)

// 侧栏（展开状态本地记忆）与同步操作
const SIDEBAR_KEY = 'xx1.terminal.sidebar'
const sidebarOpen = ref(localStorage.getItem(SIDEBAR_KEY) === '1')
const syncMode = ref(false)
watch(sidebarOpen, (v) => localStorage.setItem(SIDEBAR_KEY, v ? '1' : '0'))

const activeTab = computed(() => tabs.value.find(t => t.key === activeKey.value) || null)

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
  tabs.value.push({key, serverId, serverName: sv.name, title: sv.name, connected: true})
  activeKey.value = key
  closeMenu()
  await nextTick()
  if (syncMode.value) await nextTick() // 网格模式下列宽变化，等布局稳定再初始化
  initTerminal(key)
  // 新标签可能把标签栏撑到可视区外，滚动到可见位置
  const el = document.getElementById('tab-' + key)
  if (el && el.scrollIntoView) el.scrollIntoView({block: 'nearest', inline: 'nearest'})
  // 同步模式下新开的终端也要参与广播
  await nextTick()
  refitAll()
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
    tab.connected = false
  })

  // 输入 -> 后端；开启同步操作时把输入广播给其他终端
  term.onData((data) => {
    api.terminalWrite(termId, data)
    if (syncMode.value) broadcastInput(key, data)
  })

  // 尺寸变化：所有终端统一重新 fit（同步网格下每格尺寸都不同）
  const onResize = () => {
    refitAll()
  }
  window.addEventListener('resize', onResize)
  tab._resize = onResize
}

// ---- 激活终端（输入焦点）----
// 点标签或点同步网格里的任意格子都会走这里：切换激活项 → 上方 tab 同步选中 →
// 终端获得键盘焦点 → 该格边框高亮。三处状态始终一致。
async function activateTab(key) {
  if (!key) return
  activeKey.value = key
  closeMenu()
  await nextTick()
  const term = terms.get(key)
  if (term) {
    try { term.focus() } catch (e) { /* 元素不可见时忽略 */ }
  }
}

// ---- 尺寸自适应 ----
// 单一入口：任何会改变终端可视尺寸的操作（展开/收起侧栏、切换同步网格、
// 新建/关闭标签、窗口 resize）之后都要重新 fit 并上报，否则远端仍按旧宽度排版
// ——表现就是"旧终端内容变形、新开的正常"。
function nextFrame() {
  return new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))
}

async function refitAll() {
  await nextTick()
  await nextFrame()
  await nextTick()
  tabs.value.forEach((t) => {
    const term = terms.get(t.key)
    const fit = fitAddons.get(t.key)
    if (!term || !fit || !t.termId) return
    try {
      fit.fit()
      api.terminalResize(t.termId, term.cols, term.rows)
    } catch (e) { /* 元素不可见时忽略 */ }
  })
}

// 侧栏展开/收起会改变终端可用宽度：等布局落定后再对所有终端重新 fit。
// flush: 'post' 确保在 DOM 更新之后执行。
watch(sidebarOpen, () => { refitAll() }, {flush: 'post'})

// ---- 多终端同步 ----
// 只写给「其他」终端：发起者自身的显示由 xterm 本地回显 + SSH 回显保证，避免重复输入
function broadcastInput(fromKey, data) {
  if (!data) return
  const others = tabs.value
    .filter(t => t.key !== fromKey && t.termId && t.connected !== false)
    .map(t => t.termId)
  if (!others.length) return
  api.terminalWriteMany(others, data).catch(() => { /* 单个会话断开由标签状态提示 */ })
}

async function toggleSync() {
  syncMode.value = !syncMode.value
  if (syncMode.value) {
    if (!tabs.value.length) {
      ElMessage.info('还没有打开的终端，同步视图会在新建终端后生效')
    } else {
      ElMessage.success(`同步操作已开启，共 ${tabs.value.length} 个终端`)
    }
  } else {
    ElMessage.info('同步操作已关闭')
  }
  await refitAll()
}

// ---- 侧栏：快捷命令 / 路径 ----
// 目标集合：同步开启时全部终端，否则当前激活终端
function targetTabIds() {
  if (syncMode.value) {
    return tabs.value.filter(t => t.termId && t.connected !== false).map(t => t.termId)
  }
  const cur = activeTab.value
  return cur && cur.termId && cur.connected !== false ? [cur.termId] : []
}

async function sendToTargets(text) {
  const ids = targetTabIds()
  if (!ids.length) {
    ElMessage.warning('请先打开一个终端')
    return false
  }
  if (ids.length === 1) {
    await api.terminalWrite(ids[0], text)
  } else {
    await api.terminalWriteMany(ids, text)
  }
  // 焦点保持在当前激活终端上，方便直接敲回车；
  // 但不要抢走用户刚点过的那个格子（activateTab 已负责聚焦）
  const cur = activeKey.value
  if (cur && terms.has(cur)) {
    try { terms.get(cur).focus() } catch (e) { /* ignore */ }
  }
  return true
}

// 点击预设命令：键入但不回车（避免误执行）
async function onQuickInsert(command) {
  if (!command) return
  const ok = await sendToTargets(command)
  if (ok) {
    const n = targetTabIds().length
    ElMessage.success(n > 1 ? `已同步键入到 ${n} 个终端，按回车执行` : '已键入命令，按回车执行')
  }
}

// 点击预设路径：cd 并回车执行（跳转本身就是意图）。
// 关键：模拟回车必须发 \r（CR），不能发 \n —— SSH 的 PTY 会做 ICRNL 把 CR 转成 NL，
// 而直接写入的 NL 只会被行规程当作换行，命令不执行（表现为"只输入了 cd 没跳转"）。
async function onQuickJump(path) {
  if (!path) return
  const quoted = "'" + String(path).replace(/'/g, "'\\''") + "'"
  const ok = await sendToTargets(`cd ${quoted}\r`)
  if (ok) {
    const n = targetTabIds().length
    ElMessage.success(n > 1 ? `已同步跳转到 ${n} 个终端：${path}` : `已跳转到：${path}`)
  }
}

function toggleSidebar() {
  // 侧栏宽度变化后的重新 fit 交给 sidebarOpen 的 watch（flush: 'post'）统一处理
  sidebarOpen.value = !sidebarOpen.value
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
  // 同步开启时，粘贴内容也要同步给其他终端
  if (syncMode.value) broadcastInput(tab.key, text)
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
  if (!hasSelection.value) {
    ElMessage.warning('请先用鼠标选中要复制的内容')
    return
  }
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
  // 同步网格下减少一格，其余格子的所属行列会变化，需要重新 fit
  refitAll()
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
          @click="activateTab(t.key)"
        >
          <span class="tab-title">{{ t.title }}</span>
          <el-icon v-if="t.connected === false" class="tab-off" title="会话已断开"><WarningFilled /></el-icon>
          <el-icon class="tab-close" @click.stop="closeTab(t.key)"><Close /></el-icon>
        </div>
        <el-tooltip content="新建终端" placement="bottom">
          <button class="tab-add" @click="addTab">
            <el-icon><Plus /></el-icon>
          </button>
        </el-tooltip>
      </div>
    </div>

    <div class="term-main">
      <!-- 终端区域：单窗口叠层 或 同步 2×2 网格 -->
      <div class="term-body" :class="{sync: syncMode}">
        <div
          v-for="(t, i) in tabs"
          :key="t.key"
          class="term-pane"
          :class="{active: activeKey === t.key, 'grid-cell': syncMode}"
          @mousedown="activateTab(t.key)"
          @click="activateTab(t.key)"
        >
          <div v-if="syncMode" class="pane-badge">
            <span class="pane-idx">{{ i + 1 }}</span>
            <span class="pane-name">{{ t.serverName }}</span>
            <span v-if="t.connected === false" class="pane-off">已断开</span>
          </div>
          <div :id="'term-' + t.key" class="term-container" />
        </div>
        <div v-if="!tabs.length" class="term-empty">
          <el-empty description="还没有打开终端，点击标签栏的 + 新建" :image-size="70" />
        </div>
      </div>

      <!-- 右侧快捷侧边栏；收起时只留一条贴着右边缘的展开条 -->
      <button
        v-if="!sidebarOpen"
        class="sidebar-handle"
        title="展开快捷面板（预设命令 / 预设路径 / 同步操作）"
        @click="toggleSidebar"
      >
        <el-icon><DArrowLeft /></el-icon>
        <span class="handle-text">快捷面板</span>
      </button>
      <QuickPanel
        v-else
        class="sidebar"
        :active-terminal="activeTab"
        :terminals="tabs"
        :sync="syncMode"
        @update:sync="toggleSync"
        @insert="onQuickInsert"
        @jump="onQuickJump"
        @close="toggleSidebar"
      />
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
  /* 标签栏统一高度：标签、加号按钮、空态下都取同一个值 */
  --tabbar-h: 34px;
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
  /* 固定标签栏高度：没有标签时（.term-tabs 内容为空）也不会被压成只有 + 的高度 */
  height: var(--tabbar-h);
  flex: 0 0 auto;
}
.term-tabs {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: stretch;
  height: var(--tabbar-h);
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
  padding: 0 14px;
  height: var(--tabbar-h);
  box-sizing: border-box;
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
  /* 与未选中标签保持同底色（不再用终端深色底），
     选中态只靠顶部的高亮色条区分，浅色主题下不再出现“黑底灰字” */
  background: var(--panel-2);
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
.term-tab .tab-off { color: var(--red); }
/* 新建按钮不单独占行：紧跟在最后一个标签右侧；标签过多被挤出时自动贴在最右 */
.tab-add {
  flex: 0 0 auto;
  width: 34px;
  height: var(--tabbar-h);
  box-sizing: border-box;
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
/* 终端区 + 右侧快捷面板 */
.term-main {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  align-items: stretch;
}
.term-body {
  flex: 1 1 auto;
  position: relative;
  min-height: 0;
}
/* 单窗口模式：叠层，只显示激活的那个。
   注意用 visibility 而不是 display:none —— 隐藏的终端仍要保留布局尺寸，
   这样展开/收起侧栏、切换同步时对「非激活」终端调用 fit() 才能测到正确宽高，
   否则它们会按旧尺寸继续渲染，表现为内容变形。 */
.term-pane {
  position: absolute;
  inset: 0;
  visibility: hidden;
}
.term-pane.active { visibility: visible; }

/* 同步模式：2×2 网格。每格固定 1/4 大小（grid-auto-rows 固定行高），
   超过 4 个终端时外层出现滚动条，而不是把格子压小 */
.term-body.sync {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  grid-auto-rows: calc(50% - 4px);
  gap: 8px;
  padding: 8px;
  overflow-y: auto;
  overflow-x: hidden;
}
.term-body.sync .term-pane {
  position: relative;
  inset: auto;
  display: block;
  visibility: visible;
  min-width: 0;
  min-height: 0;
  border: 1px solid var(--line);
  border-radius: 10px;
  overflow: hidden;
  background: var(--terminal-bg);
}
/* 激活格：加粗高亮边框，和 tab 选中态一起标识「当前输入焦点」 */
.term-body.sync .term-pane.active {
  border-color: var(--blue);
  box-shadow: 0 0 0 1px var(--blue) inset, 0 0 10px var(--blue-glow);
}
.term-body.sync .term-pane.active .pane-badge {
  border-color: var(--blue);
  color: var(--blue-deep);
}
.pane-badge {
  position: absolute;
  z-index: 3;
  top: 4px;
  left: 6px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 1px 7px;
  border-radius: 999px;
  font-size: 11px;
  color: var(--ink-2);
  background: var(--panel-2);
  border: 1px solid var(--line);
  pointer-events: none;
}
.pane-idx {
  font-weight: 700;
  color: var(--blue-deep);
}
.pane-off { color: var(--red); }
.term-container {
  width: 100%;
  height: 100%;
  padding: 4px;
}
.term-body.sync .term-container { padding-top: 20px; }
.term-empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

/* 侧栏收起时的展开条：贴在终端区右边缘 */
.sidebar-handle {
  flex: 0 0 auto;
  width: 22px;
  border: none;
  border-left: 1px solid var(--line);
  background: var(--panel-2);
  color: var(--ink-3);
  cursor: pointer;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  transition: background-color .2s ease, color .2s ease;
}
.sidebar-handle:hover {
  background: var(--hover-bg);
  color: var(--blue);
}
.handle-text {
  writing-mode: vertical-rl;
  font-size: 11px;
  letter-spacing: 2px;
}
.sidebar {
  flex: 0 0 auto;
  width: 300px;
}
@media (min-width: 1400px) {
  .sidebar { width: 340px; }
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