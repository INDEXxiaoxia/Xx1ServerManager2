<script setup>
import {ref, computed, watch, onMounted, onBeforeUnmount, nextTick} from 'vue'
import {useRouter} from 'vue-router'
import {ElMessage, ElMessageBox} from 'element-plus'
import {api} from '@/api'
import PresetFormDialog from '@/components/PresetFormDialog.vue'

const props = defineProps({
  // 当前激活终端（{key, serverId, serverName, termId, connected}），没有则为 null
  activeTerminal: {type: Object, default: null},
  // 所有已打开终端
  terminals: {type: Array, default: () => []},
  // 同步操作是否开启
  sync: {type: Boolean, default: false}
})
const emit = defineEmits(['update:sync', 'insert', 'jump', 'close'])

const router = useRouter()

const CATEGORIES = ['系统', '网络', '服务', '磁盘', '日志', '其他']
const category = ref('系统')
const commands = ref([])
const paths = ref([])
// 初值必须是 false：侧栏一打开就显示遮罩会闪一下（这层遮罩还会盖住整个侧栏）
const loading = ref(false)

// 目标终端：同步开启时为全部已打开终端，否则只有当前激活终端
const targets = computed(() => {
  const list = (props.terminals || []).filter(t => t.termId && t.connected !== false)
  if (props.sync) return list
  const cur = props.activeTerminal
  return cur && cur.termId && cur.connected !== false ? [cur] : []
})
const targetCount = computed(() => targets.value.length)
const targetLabel = computed(() => (props.sync ? `同步 ${targetCount.value} 个终端` : '当前终端'))

const filteredCommands = computed(() => {
  if (!category.value) return commands.value
  return commands.value.filter(c => (c.category || '其他') === category.value)
})

async function load() {
  loading.value = true
  try {
    const [cs, ps] = await Promise.all([api.listPresetCommands(), api.listPresetPaths()])
    commands.value = cs || []
    paths.value = ps || []
  } catch (e) {
    ElMessage.error('加载预设失败：' + e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

// ---- 对话框 ----
const dialogVisible = ref(false)
const dialogType = ref('command')
const dialogItem = ref(null)

function openAdd(type) {
  dialogType.value = type
  dialogItem.value = null
  dialogVisible.value = true
}
function openEdit(type, item) {
  dialogType.value = type
  dialogItem.value = item
  dialogVisible.value = true
}

async function removePreset(type, item) {
  try {
    await ElMessageBox.confirm(`确定删除「${item.name}」吗？`, '删除确认', {type: 'warning'})
  } catch (e) {
    return
  }
  try {
    if (type === 'command') await api.deletePresetCommand(item.id)
    else await api.deletePresetPath(item.id)
    ElMessage.success('已删除')
    await load()
    closeMenu()
  } catch (e) {
    ElMessage.error('删除失败：' + e)
  }
}

// ---- 右键菜单 ----
const menu = ref({visible: false, x: 0, y: 0, type: '', item: null})
function openMenu(ev, type, item) {
  menu.value = {visible: true, x: ev.clientX, y: ev.clientY, type, item}
  nextTick(() => {
    // 防止菜单超出窗口右下边界
    const el = document.querySelector('.quick-menu')
    if (!el) return
    const rect = el.getBoundingClientRect()
    const nx = Math.min(menu.value.x, window.innerWidth - rect.width - 8)
    const ny = Math.min(menu.value.y, window.innerHeight - rect.height - 8)
    menu.value = {...menu.value, x: Math.max(8, nx), y: Math.max(8, ny)}
  })
}
function closeMenu() {
  menu.value = {...menu.value, visible: false}
}
function onMenuCommand(cmd) {
  const {type, item} = menu.value
  closeMenu()
  if (cmd === 'insert') insertCommand(item)
  else if (cmd === 'edit') openEdit(type, item)
  else if (cmd === 'delete') removePreset(type, item)
}

// ---- 动作：键入命令 / 跳转路径 ----
function insertCommand(item) {
  if (!targetCount.value) { ElMessage.warning('请先打开一个终端'); return }
  emit('insert', item.command || '')
}

function jumpPath(item) {
  if (!targetCount.value) { ElMessage.warning('请先打开一个终端'); return }
  emit('jump', item.path || '')
}

// 路径区「取当前终端目录」：在当前终端所连服务器上执行一次 pwd
const fetchingPwd = ref(false)
async function addPathFromCurrentDir() {
  const cur = props.activeTerminal
  if (!cur || !cur.serverId) { ElMessage.warning('请先打开一个终端'); return }
  fetchingPwd.value = true
  try {
    const dir = await api.runRemoteCommand(cur.serverId, 'pwd', 10)
    dialogType.value = 'path'
    dialogItem.value = dir ? {name: '', path: dir, description: ''} : null
    dialogVisible.value = true
  } catch (e) {
    ElMessage.error('获取远端目录失败：' + e)
  } finally {
    fetchingPwd.value = false
  }
}

function goPresets() {
  router.push('/presets')
}

// 点到侧栏外部时收起右键菜单
function onDocClick() {
  if (menu.value.visible) closeMenu()
}
function onDocKeydown(ev) {
  if (ev.key === 'Escape' && menu.value.visible) closeMenu()
}
onMounted(() => {
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onDocKeydown)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onDocKeydown)
})

// 外部新增/删除预设后（预设页）切换回终端页时刷新一次
watch(() => props.terminals.length, () => { /* 保持轻量，不主动刷新 */ })
</script>

<template>
  <aside class="quick-panel" v-loading="loading">
    <!-- 顶部：同步操作 -->
    <div class="qp-head">
      <button
        type="button"
        class="sync-btn"
        :class="{active: sync}"
        :title="sync ? '关闭同步操作' : '开启同步操作：所有已打开终端排成 2×2 并同步输入'"
        @click="emit('update:sync', !sync)"
      >
        <el-icon><Connection /></el-icon>
        <span>同步操作</span>
        <el-tag v-if="sync" size="small" type="warning" effect="plain">{{ terminals.length }}</el-tag>
      </button>
      <el-button text size="small" title="收起侧栏" @click="emit('close')">
        <el-icon><DArrowRight /></el-icon>
      </el-button>
    </div>
    <div class="qp-tip" :class="{warn: !targetCount}">
      {{ targetCount ? `快捷操作目标：${targetLabel}` : '未打开终端，快捷操作不可用' }}
    </div>

    <!-- 预设命令 -->
    <section class="qp-section">
      <div class="qp-title">
        <span>预设命令</span>
        <span class="qp-count">{{ filteredCommands.length }}</span>
        <el-select v-model="category" size="small" class="qp-filter" placeholder="全部分类">
          <el-option label="全部分类" value="" />
          <el-option v-for="c in CATEGORIES" :key="c" :label="c" :value="c" />
        </el-select>
        <el-button text size="small" title="新增命令" @click="openAdd('command')">
          <el-icon><Plus /></el-icon>
        </el-button>
      </div>
      <div class="qp-list">
        <div v-if="!filteredCommands.length" class="qp-empty">
          暂无命令，点上方 <el-icon><Plus /></el-icon> 添加
        </div>
        <div
          v-for="c in filteredCommands"
          :key="c.id"
          class="qp-item"
          :title="c.command"
          @click="insertCommand(c)"
          @contextmenu.prevent="openMenu($event, 'command', c)"
        >
          <div class="qp-item-main">
            <div class="qp-item-name">{{ c.name }}</div>
            <div class="qp-item-sub mono">{{ c.command }}</div>
          </div>
          <el-icon class="qp-item-act" title="键入到终端"><Right /></el-icon>
        </div>
      </div>
    </section>

    <!-- 预设路径 -->
    <section class="qp-section">
      <div class="qp-title">
        <span>预设路径</span>
        <span class="qp-count">{{ paths.length }}</span>
        <el-tooltip content="以当前终端所在目录新建路径" placement="top">
          <el-button
            text
            size="small"
            :loading="fetchingPwd"
            @click="addPathFromCurrentDir"
          >
            <el-icon><Aim /></el-icon>
          </el-button>
        </el-tooltip>
        <el-button text size="small" title="新增路径" @click="openAdd('path')">
          <el-icon><Plus /></el-icon>
        </el-button>
      </div>
      <div class="qp-list">
        <div v-if="!paths.length" class="qp-empty">
          暂无路径，点上方 <el-icon><Plus /></el-icon> 添加
        </div>
        <div
          v-for="p in paths"
          :key="p.id"
          class="qp-item"
          :title="p.path"
          @contextmenu.prevent="openMenu($event, 'path', p)"
        >
          <div class="qp-item-main">
            <div class="qp-item-name">{{ p.name }}</div>
            <div class="qp-item-sub mono">{{ p.path }}</div>
          </div>
          <el-button class="qp-jump" size="small" text type="primary" @click.stop="jumpPath(p)">跳转</el-button>
        </div>
      </div>
    </section>

    <div class="qp-foot">
      <el-button text size="small" @click="goPresets">管理预设 →</el-button>
      <el-button text size="small" @click="load">
        <el-icon><Refresh /></el-icon>&nbsp;刷新
      </el-button>
    </div>

    <!-- 右键菜单 -->
    <div v-if="menu.visible" class="quick-menu" :style="{left: menu.x + 'px', top: menu.y + 'px'}" @click.stop>
      <div v-if="menu.type === 'command'" class="quick-menu-item" @click="onMenuCommand('insert')">
        <el-icon><Right /></el-icon>键入到终端
      </div>
      <div class="quick-menu-item" @click="onMenuCommand('edit')">
        <el-icon><Edit /></el-icon>编辑
      </div>
      <div class="quick-menu-item danger" @click="onMenuCommand('delete')">
        <el-icon><Delete /></el-icon>删除
      </div>
    </div>

    <PresetFormDialog
      v-model="dialogVisible"
      :type="dialogType"
      :item="dialogItem"
      :default-category="category || '其他'"
      @saved="load"
    />
  </aside>
</template>

<style scoped>
.quick-panel {
  position: relative;   /* v-loading 的定位上下文（否则遮罩会挂到侧栏外层） */
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  background: var(--aside-bg);
  border-left: 1px solid var(--line);
}

/* 顶部同步操作 */
.qp-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-bottom: 1px solid var(--line);
  flex: 0 0 auto;
}
.sync-btn {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  height: 30px;
  border-radius: 10px;
  border: 1px solid var(--line);
  background: var(--card);
  color: var(--ink-2);
  font-family: inherit;
  font-size: 13px;
  cursor: pointer;
  transition: background-color .2s ease, color .2s ease, border-color .2s ease;
}
.sync-btn:hover { color: var(--blue-deep); border-color: var(--blue); }
.sync-btn.active {
  background: var(--blue-soft);
  color: var(--blue-deep);
  border-color: var(--blue);
  font-weight: 600;
}
.qp-tip {
  padding: 6px 10px;
  font-size: 12px;
  color: var(--ink-3);
  border-bottom: 1px solid var(--line);
  flex: 0 0 auto;
}
.qp-tip.warn { color: var(--yellow); }

/* 分区 */
.qp-section {
  display: flex;
  flex-direction: column;
  flex: 1 1 0;
  min-height: 120px;
  border-bottom: 1px solid var(--line);
}
.qp-title {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 600;
  color: var(--ink-2);
  flex: 0 0 auto;
}
.qp-count {
  font-weight: 400;
  color: var(--ink-3);
}
.qp-filter { width: 96px; margin-left: auto; }
.qp-list {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 0 6px 6px;
}
.qp-empty {
  padding: 14px 8px;
  font-size: 12px;
  color: var(--ink-3);
  text-align: center;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  flex-wrap: wrap;
}
.qp-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border-radius: 8px;
  cursor: pointer;
  transition: background-color .15s ease;
}
.qp-item:hover { background: var(--hover-bg); }
.qp-item-main { flex: 1; min-width: 0; }
.qp-item-name {
  font-size: 13px;
  color: var(--ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.qp-item-sub {
  font-size: 11px;
  color: var(--ink-3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.qp-item-act {
  color: var(--ink-3);
  flex: 0 0 auto;
}
.qp-item:hover .qp-item-act { color: var(--blue); }
.qp-jump { flex: 0 0 auto; }

/* 底部 */
.qp-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 8px;
  flex: 0 0 auto;
}

/* 右键菜单 */
.quick-menu {
  position: fixed;
  z-index: 3000;
  min-width: 150px;
  padding: 4px;
  background: var(--card);
  border: 1px solid var(--line);
  border-radius: 10px;
  box-shadow: var(--el-box-shadow, 0 8px 24px rgba(0, 0, 0, .18));
}
.quick-menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 10px;
  font-size: 13px;
  color: var(--ink);
  border-radius: 7px;
  cursor: pointer;
}
.quick-menu-item:hover { background: var(--hover-bg); }
.quick-menu-item.danger { color: var(--red); }

.mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
</style>