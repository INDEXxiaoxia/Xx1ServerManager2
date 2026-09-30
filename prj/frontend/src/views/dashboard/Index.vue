<script setup>
import {ref, onMounted, onBeforeUnmount, computed} from 'vue'
import {useRouter} from 'vue-router'
import {api, events} from '@/api'
import {usageColor} from '@/theme'
import {UNGROUPED, groupIdOf, matchServer, orderedGroups, groupedSections, groupNameOf, groupColorOf} from '@/utils/groups'

const router = useRouter()
const servers = ref([])
const groups = ref([])
const statsMap = ref({})
// healthMap：id → {state:'pending'|'online'|'offline', message, at}
// 在线/离线只看监控事件——取到数据就是在线的唯一判据，
// 库里那个 status 字段从不更新，看了只会误报"离线"。
const healthMap = ref({})
const keyword = ref('')
const groupFilter = ref('') // '' = 全部分组，UNGROUPED = 未分组，其余为分组 id

let timers = []
const dataListeners = new Map()
const statusListeners = new Map()
const subscribed = new Set()

// 超过这个时间没有任何监控事件，说明采集循环停摆，按离线处理
const HEALTH_STALE_MS = 15000

// 服务器一多就不整页铺开：总数超过阈值（9 台）时，首次进入默认只显示
// 第一个有服务器的分组，其余靠下拉切换；用户手动改过筛选（含清空）后不再覆盖。
const AUTO_FILTER_THRESHOLD = 9
const autoFilterActive = ref(false)
let defaultFilterApplied = false

const groupOptions = computed(() => orderedGroups(servers.value, groups.value))
const isFiltered = computed(() => !!keyword.value.trim() || !!groupFilter.value)

function inGroupFilter(s) {
  if (!groupFilter.value) return true
  const id = groupIdOf(s)
  if (groupFilter.value === UNGROUPED) return !id
  return id === groupFilter.value
}

const filtered = computed(() => servers.value.filter(s => matchServer(s, keyword.value) && inGroupFilter(s)))

// 分区：不筛选时按分组分区（有服务器的分组在前，未分组最后）；
// 已筛选到具体分组时不再分区，改在卡片上挂分组标签。
const sections = computed(() => {
  if (groupFilter.value) {
    return [{key: 'filtered', name: '', color: '', count: filtered.value.length, servers: filtered.value, flat: true}]
  }
  return groupedSections(filtered.value, groups.value).map(sec => ({...sec, key: sec.id}))
})

function healthOf(id) {
  return healthMap.value[id] || {state: 'pending', message: '', at: 0}
}
const onlineCount = computed(() => servers.value.filter(s => healthOf(s.id).state === 'online').length)
const offlineCount = computed(() => servers.value.filter(s => healthOf(s.id).state === 'offline').length)
const pendingCount = computed(() => servers.value.filter(s => healthOf(s.id).state === 'pending').length)

const HEALTH_TEXT = {pending: '等待数据', online: '在线', offline: '离线'}
const HEALTH_TAG = {pending: 'info', online: 'success', offline: 'danger'}

function healthText(id) {
  return HEALTH_TEXT[healthOf(id).state] || '等待数据'
}
function healthTag(id) {
  return HEALTH_TAG[healthOf(id).state] || 'info'
}
// 原生 title：几十张卡片上不用组件化 tooltip，避免上百个浮层
function healthTitle(id) {
  const h = healthOf(id)
  if (h.state === 'offline') return h.message ? `离线：${h.message}` : '离线：连接或取数失败'
  if (h.state === 'online') return `在线：监控数据正常${h.at ? '（' + timeText(h.at) + '）' : ''}`
  return '等待第一轮监控数据'
}
function timeText(at) {
  try {
    return new Date(at).toLocaleTimeString()
  } catch (e) {
    return ''
  }
}

async function load() {
  const [svs, gs] = await Promise.all([api.listServers(), api.listGroups()])
  servers.value = svs
  groups.value = gs
  await syncSubscriptions()
  // 分组被删除/改名后，筛选值可能已失效，回落到"全部分组"
  if (groupFilter.value && groupFilter.value !== UNGROUPED && !gs.some(g => g.id === groupFilter.value)) {
    groupFilter.value = ''
    autoFilterActive.value = false
  }
  applyDefaultGroupFilter(gs, svs)
}

// applyDefaultGroupFilter 首次进入时的默认筛选（只做一次，且只在服务器较多时）
function applyDefaultGroupFilter(gs, svs) {
  if (defaultFilterApplied || !svs.length) return
  defaultFilterApplied = true
  if (svs.length <= AUTO_FILTER_THRESHOLD) return
  // 第一个"有服务器的真实分组"（未分组不算分组，此时宁可不筛选、按分区全展示）
  const first = orderedGroups(svs, gs).find(g => g.count > 0 && !g.isUngrouped)
  if (!first) return
  groupFilter.value = first.id
  autoFilterActive.value = true
}

function onData(serverId) {
  return (stats) => { statsMap.value[serverId] = stats }
}

function onStatus(serverId) {
  return (st) => {
    if (!st) return
    healthMap.value[serverId] = {
      state: st.ok ? 'online' : 'offline',
      message: st.message || '',
      at: Number(st.at) || Date.now()
    }
  }
}

function subscribe(server) {
  subscribed.add(server.id)
  if (!healthMap.value[server.id]) {
    healthMap.value[server.id] = {state: 'pending', message: '', at: 0}
  }
  const dcb = onData(server.id)
  const scb = onStatus(server.id)
  dataListeners.set(server.id, dcb)
  statusListeners.set(server.id, scb)
  events.on('monitor:data:' + server.id, dcb)
  events.on('monitor:status:' + server.id, scb)
  api.startMonitor(server.id)
}

function unsubscribe(serverId) {
  subscribed.delete(serverId)
  const dcb = dataListeners.get(serverId)
  const scb = statusListeners.get(serverId)
  if (dcb) events.off('monitor:data:' + serverId, dcb)
  if (scb) events.off('monitor:status:' + serverId, scb)
  dataListeners.delete(serverId)
  statusListeners.delete(serverId)
  api.stopMonitor(serverId)
}

// syncSubscriptions 补齐新增服务器的订阅与监控；列表里已经没有的退订
async function syncSubscriptions() {
  const ids = new Set(servers.value.map(s => s.id))
  for (const s of servers.value) {
    if (!subscribed.has(s.id)) subscribe(s)
  }
  for (const id of [...subscribed]) {
    if (!ids.has(id)) unsubscribe(id)
  }
}

// checkStale 看门狗：采集循环停摆时把状态降级为离线，避免界面一直显示"在线"
function checkStale() {
  const now = Date.now()
  for (const s of servers.value) {
    const h = healthMap.value[s.id]
    if (!h || !h.at || h.state === 'pending') continue
    if (now - h.at > HEALTH_STALE_MS) {
      healthMap.value[s.id] = {...h, state: 'offline', message: '监控数据已停止更新'}
    }
  }
}

onMounted(async () => {
  await load()
  timers.push(setInterval(load, 10000))
  timers.push(setInterval(checkStale, 5000))
})

onBeforeUnmount(() => {
  timers.forEach(t => clearInterval(t))
  timers = []
  for (const id of [...subscribed]) unsubscribe(id)
  api.stopAllMonitors()
})

function fmtBytes(mb) {
  if (!mb) return '0 MB'
  if (mb >= 1024) return (mb / 1024).toFixed(1) + ' GB'
  return mb.toFixed(0) + ' MB'
}

// ---- 磁盘：卡片上只突出"关键磁盘"，多盘时附一行迷你条（卡片高度与磁盘数无关） ----

function disksOf(id) {
  return (statsMap.value[id] && statsMap.value[id].disks) || []
}

function realDisksOf(id) {
  return disksOf(id).filter(d => !d.isPseudo)
}

// pickKeyDisk 与后端同规则：真实可写盘中占用最高的一块，逐级兜底
function pickKeyDisk(disks) {
  const preds = [d => !d.isPseudo && !d.isReadOnly, d => !d.isPseudo, () => true]
  for (const ok of preds) {
    let best = null
    for (const d of disks || []) {
      if (!ok(d)) continue
      if (!best || (Number(d.usage) || 0) > (Number(best.usage) || 0)) best = d
    }
    if (best) return best
  }
  return null
}

function keyDiskOf(id) {
  return pickKeyDisk(disksOf(id))
}

function keyMountOf(id) {
  const k = keyDiskOf(id)
  return k ? k.mount : ''
}

function isDiskWarn(id) {
  const k = keyDiskOf(id)
  return !!k && !k.isReadOnly && (Number(k.usage) || 0) >= 80
}

// 迷你条：仅当真实磁盘 ≥ 2 时出现，最多显示 4 段，其余以 +N 收口
function miniDisks(id) {
  const list = realDisksOf(id)
  return list.length >= 2 ? list.slice(0, 4) : []
}

function hiddenDisks(id) {
  return miniDisks(id).length ? Math.max(0, realDisksOf(id).length - 4) : 0
}

// 原生 title：多台服务器 × 多磁盘时不用组件化 tooltip，避免上百个浮层
function diskTitle(d) {
  const pct = (Number(d.usage) || 0).toFixed(1)
  const fs = d.fstype ? `，${d.fstype}` : ''
  const ro = d.isReadOnly ? ' · 只读' : ''
  return `${d.mount} · ${pct}%（${fmtBytes(d.usedMB)} / ${fmtBytes(d.totalMB)}${fs}）${ro}`
}

function goMonitor(id) {
  router.push({path: '/monitor', query: {serverId: id}})
}

function clearFilter() {
  keyword.value = ''
  groupFilter.value = ''
  autoFilterActive.value = false
}
</script>

<template>
  <div class="page-container">
    <div class="stat-grid mb-12">
      <div class="stat-card">
        <div class="num">{{ servers.length }}</div>
        <div class="label">服务器总数</div>
        <div v-if="pendingCount" class="sub">等待数据 {{ pendingCount }}</div>
      </div>
      <div class="stat-card success">
        <div class="num">{{ onlineCount }}</div>
        <div class="label">在线</div>
      </div>
      <div class="stat-card danger">
        <div class="num">{{ offlineCount }}</div>
        <div class="label">离线</div>
      </div>
    </div>

    <div class="card filter-bar mb-12">
      <el-input v-model="keyword" placeholder="搜索名称或主机" clearable class="kw">
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <el-select v-model="groupFilter" class="gf" placeholder="全部分组" @change="autoFilterActive = false">
        <el-option label="全部分组" :value="''" />
        <el-option v-for="g in groupOptions" :key="g.id" :value="g.id" :label="g.name">
          <span class="opt">
            <i class="opt-dot" :style="{background: g.color}" />
            <span class="opt-name">{{ g.name }}</span>
            <span class="opt-count">{{ g.count }} 台</span>
          </span>
        </el-option>
      </el-select>
      <div class="filter-right">
        <span v-if="autoFilterActive" class="auto-hint">服务器较多，默认只显示第一个分组</span>
        <span class="count-text">显示 {{ filtered.length }} / 共 {{ servers.length }} 台</span>
        <el-button v-if="isFiltered" text type="primary" @click="clearFilter">清空筛选</el-button>
      </div>
    </div>

    <div v-if="!servers.length" class="card empty">
      <div class="empty-title">还没有服务器</div>
      <div class="empty-desc">先添加一台服务器，这里就会显示它的实时负载。</div>
      <el-button type="primary" @click="router.push('/servers')">去服务器列表新增</el-button>
    </div>

    <div v-else-if="!filtered.length" class="card empty">
      <div class="empty-title">没有匹配的服务器</div>
      <div class="empty-desc">换个关键字，或切换到其他分组看看。</div>
      <el-button @click="clearFilter">清空筛选</el-button>
    </div>

    <section v-for="sec in sections" :key="sec.key" class="group-block">
      <header v-if="!sec.flat" class="group-head">
        <i class="group-dot" :style="{background: sec.color}" />
        <span class="group-name">{{ sec.name }}</span>
        <span class="group-count">{{ sec.count }} 台</span>
        <i class="group-line" />
      </header>
      <div class="server-grid">
        <div v-for="s in sec.servers" :key="s.id" class="card server-card" @click="goMonitor(s.id)">
          <div class="flex-between">
            <span class="server-name">
              <span class="status-dot" :class="'is-' + healthOf(s.id).state" />{{ s.name }}
            </span>
            <el-tag size="small" :type="healthTag(s.id)" :title="healthTitle(s.id)">
              {{ healthText(s.id) }}
            </el-tag>
          </div>
          <div class="meta mt-12">
            <span v-if="sec.flat" class="group-chip">
              <i class="opt-dot" :style="{background: groupColorOf(groups, groupIdOf(s))}" />
              {{ groupNameOf(groups, groupIdOf(s)) }}
            </span>
            <span class="text-muted">{{ s.host }}:{{ s.port }}</span>
          </div>
          <template v-if="statsMap[s.id]">
            <div class="metric mt-12">
              <span>CPU</span>
              <el-progress :percentage="Number(statsMap[s.id].cpuUsage || 0).toFixed(1)" :stroke-width="8"
                           :color="statsMap[s.id].cpuUsage > 80 ? 'var(--red)' : 'var(--blue)'" />
            </div>
            <div class="metric">
              <span>内存</span>
              <el-progress :percentage="Number(statsMap[s.id].memUsage || 0).toFixed(1)" :stroke-width="8"
                           :color="statsMap[s.id].memUsage > 80 ? 'var(--red)' : 'var(--green)'" />
            </div>
            <div class="metric">
              <span class="metric-key" :title="keyMountOf(s.id) ? '关键磁盘 ' + keyMountOf(s.id) : '磁盘'">
                磁盘 {{ keyMountOf(s.id) }}
              </span>
              <el-progress :percentage="Number(statsMap[s.id].diskUsage || 0).toFixed(1)" :stroke-width="8"
                           :color="isDiskWarn(s.id) ? 'var(--red)' : 'var(--yellow)'" />
              <span v-if="isDiskWarn(s.id)" class="disk-warn" title="使用率已达 80% 以上">⚠</span>
            </div>
            <div class="disk-mini" v-if="miniDisks(s.id).length">
              <span v-for="d in miniDisks(s.id)" :key="d.mount" class="disk-seg"
                    :style="{background: usageColor(d.usage)}" :title="diskTitle(d)" />
              <span v-if="hiddenDisks(s.id)" class="disk-more">+{{ hiddenDisks(s.id) }}</span>
            </div>
            <div class="text-muted mt-12">
              内存 {{ fmtBytes(statsMap[s.id].memUsed) }} / {{ fmtBytes(statsMap[s.id].memTotal) }}
            </div>
          </template>
          <div v-else class="text-muted mt-12">等待数据...</div>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
/* ---- 顶部统计 ---- */
.stat-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}
.stat-card {
  position: relative;
  background: linear-gradient(135deg, var(--blue-soft), var(--stat-grad-1));
  border-radius: 18px;
  padding: 18px 16px;
  text-align: center;
  box-shadow: var(--card-shadow);
  overflow: hidden;
  animation: rise .5s cubic-bezier(.16, 1, .3, 1) both;
}
.stat-card::before {
  content: '';
  position: absolute;
  top: 16px;
  left: 18px;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--blue);
  animation: breathe 4.5s ease-in-out infinite;
}
.stat-grid > .stat-card:nth-child(1) { animation-delay: 0s; }
.stat-grid > .stat-card:nth-child(2) { animation-delay: .06s; }
.stat-grid > .stat-card:nth-child(3) { animation-delay: .12s; }
.stat-card.success {
  background: linear-gradient(135deg, var(--green-soft), var(--stat-grad-2));
}
.stat-card.success::before { background: var(--green); }
.stat-card.success .num { color: var(--green); }
.stat-card.danger {
  background: linear-gradient(135deg, var(--red-soft), var(--stat-grad-3));
}
.stat-card.danger::before { background: var(--red); }
.stat-card.danger .num { color: var(--red); }
.stat-card .num {
  font-size: 34px;
  font-weight: 700;
  letter-spacing: 1px;
  color: var(--blue-deep);
}
.stat-card .label {
  color: var(--ink-2);
  margin-top: 4px;
  font-size: 13px;
}
.stat-card .sub {
  margin-top: 2px;
  font-size: 11px;
  color: var(--ink-3);
}

/* ---- 筛选栏：搜索框 + 分组下拉 + 结果计数，窄窗口自动换行 ---- */
.filter-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  padding: 10px 12px;
}
.filter-bar .kw { width: 240px; }
.filter-bar .gf { width: 200px; }
.filter-right {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 6px;
}
.count-text {
  font-size: 12px;
  color: var(--ink-3);
}
.auto-hint {
  font-size: 12px;
  color: var(--ink-3);
  background: var(--hover-bg);
  border-radius: 999px;
  padding: 2px 9px;
}
.opt {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.opt-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  flex-shrink: 0;
}
.opt-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.opt-count {
  margin-left: auto;
  font-size: 12px;
  color: var(--ink-3);
}

/* ---- 分组分区 ---- */
.group-block + .group-block {
  margin-top: 14px;
}
.group-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  padding: 0 2px;
}
.group-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  flex-shrink: 0;
}
.group-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink-2);
}
.group-count {
  font-size: 12px;
  color: var(--ink-3);
}
.group-line {
  flex: 1;
  height: 1px;
  background: var(--line);
}
.server-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(310px, 1fr));
  gap: 12px;
}

/* ---- 服务器卡片 ---- */
.server-card {
  cursor: pointer;
  transition: transform .3s cubic-bezier(.16, 1, .3, 1), box-shadow .3s ease;
  animation: rise .45s cubic-bezier(.16, 1, .3, 1) both;
}
.server-grid > .server-card:nth-child(1) { animation-delay: .04s; }
.server-grid > .server-card:nth-child(2) { animation-delay: .08s; }
.server-grid > .server-card:nth-child(3) { animation-delay: .12s; }
.server-grid > .server-card:nth-child(4) { animation-delay: .16s; }
.server-grid > .server-card:nth-child(5) { animation-delay: .20s; }
.server-grid > .server-card:nth-child(6) { animation-delay: .24s; }
.server-card:hover {
  transform: translateY(-3px);
  box-shadow: var(--card-shadow-hover);
}
.server-name {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-weight: 600;
  font-size: 15px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--ink-3);
  flex-shrink: 0;
}
.status-dot.is-online {
  background: var(--green);
  animation: breathe 4s ease-in-out infinite;
}
.status-dot.is-offline {
  background: var(--red);
}
.meta {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.meta .text-muted {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.group-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  flex-shrink: 0;
  max-width: 50%;
  padding: 1px 8px;
  border-radius: 999px;
  background: var(--hover-bg);
  font-size: 12px;
  color: var(--ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.metric { margin-top: 8px; display: flex; align-items: center; gap: 10px; }
.metric span { width: 34px; color: var(--ink-2); font-size: 13px; flex-shrink: 0; }
.metric .el-progress { flex: 1; }
/* 关键磁盘标签带挂载点，比 CPU/内存 标签宽，过长时省略 */
.metric .metric-key {
  width: auto;
  min-width: 34px;
  max-width: 116px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.metric .disk-warn { width: auto; color: var(--red); font-size: 12px; }
/* 多磁盘迷你条：固定一行 4px 细条，卡片高度与磁盘数量无关 */
.disk-mini { margin-top: 7px; display: flex; align-items: center; gap: 3px; height: 4px; }
.disk-mini .disk-seg { flex: 1; height: 4px; border-radius: 2px; opacity: .85; }
.disk-mini .disk-more { font-size: 10px; line-height: 1; color: var(--ink-3); flex-shrink: 0; }

/* ---- 空状态 ---- */
.empty {
  text-align: center;
  padding: 36px 16px;
}
.empty-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--ink);
}
.empty-desc {
  margin: 6px 0 16px;
  font-size: 13px;
  color: var(--ink-3);
}
</style>