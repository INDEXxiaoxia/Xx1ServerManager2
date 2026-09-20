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
const tabs = ref([])         // {key, serverId, serverName, title}
const activeKey = ref('')
const terms = new Map()      // key -> xterm instance
const fitAddons = new Map()  // key -> fit addon
const dialogVisible = ref(false)
const selectedServer = ref('')

async function loadServers() {
  servers.value = await api.listServers()
}

async function openNew(serverId) {
  const sv = servers.value.find(s => s.id === serverId)
  if (!sv) return
  const key = 't' + Date.now() + Math.random().toString(36).slice(2, 6)
  tabs.value.push({key, serverId, serverName: sv.name, title: sv.name})
  activeKey.value = key
  await nextTick()
  initTerminal(key)
}

async function initTerminal(key) {
  const tab = tabs.value.find(t => t.key === key)
  const el = document.getElementById('term-' + key)
  if (!el) return
  const term = new Terminal({
    fontSize: 13,
    fontFamily: 'Cascadia Code, Consolas, monospace',
    cursorBlink: true,
    theme: getTerminalTheme(themeState.name)
  })
  const fit = new FitAddon()
  term.loadAddon(fit)
  term.open(el)
  fit.fit()
  terms.set(key, term)
  fitAddons.set(key, fit)

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
  await loadServers()
  addTab()
})

onBeforeUnmount(() => {
  tabs.value.forEach(t => {
    if (t.termId) api.terminalClose(t.termId)
    if (t._resize) window.removeEventListener('resize', t._resize)
  })
  terms.forEach(t => t.dispose())
})
</script>

<template>
  <div class="terminal-page">
    <div class="term-toolbar">
      <el-button size="small" type="primary" @click="addTab">新建终端</el-button>
    </div>
    <div class="term-tabs">
      <div v-for="t in tabs" :key="t.key" class="term-tab" :class="{active: activeKey === t.key}"
           @click="activeKey = t.key">
        <span class="tab-title">{{ t.title }}</span>
        <el-icon class="tab-close" @click.stop="closeTab(t.key)"><Close /></el-icon>
      </div>
    </div>
    <div class="term-body">
      <div v-for="t in tabs" :key="t.key" class="term-pane" :class="{active: activeKey === t.key}">
        <div :id="'term-' + t.key" class="term-container" />
      </div>
    </div>

    <el-dialog v-model="dialogVisible" title="选择服务器" width="420px">
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
}
.term-toolbar {
  padding: 8px 12px;
  background: var(--terminal-bg);
  border-bottom: 1px solid var(--line);
}
.term-tabs {
  display: flex;
  background: var(--panel-2);
  overflow-x: auto;
}
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
.term-body {
  flex: 1;
  position: relative;
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
</style>