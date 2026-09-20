<script setup>
import {ref, onMounted, onBeforeUnmount, computed} from 'vue'
import {useRouter} from 'vue-router'
import {api, events} from '@/api'
import {usageColor} from '@/theme'

const router = useRouter()
const servers = ref([])
const statsMap = ref({})
let timers = []

const onlineCount = computed(() => servers.value.filter(s => s.status === 'connected').length)
const errorCount = computed(() => servers.value.filter(s => s.status === 'error').length)
const dataListeners = new Map()

async function load() {
  servers.value = await api.listServers()
  // 为每台服务器启动监控并订阅
  for (const s of servers.value) {
    api.startMonitor(s.id)
  }
}

function onData(serverId) {
  return (stats) => { statsMap.value[serverId] = stats }
}

onMounted(async () => {
  await load()
  for (const s of servers.value) {
    const cb = onData(s.id)
    dataListeners.set(s.id, cb)
    events.on('monitor:data:' + s.id, cb)
  }
  // 定期刷新服务器列表状态
  timers.push(setInterval(async () => {
    servers.value = await api.listServers()
  }, 10000))
})

onBeforeUnmount(() => {
  timers.forEach(t => clearInterval(t))
  dataListeners.forEach((cb, id) => events.off('monitor:data:' + id, cb))
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
</script>

<template>
  <div class="page-container">
    <el-row :gutter="16" class="mb-12">
      <el-col :span="8">
        <div class="stat-card">
          <div class="num">{{ servers.length }}</div>
          <div class="label">服务器总数</div>
        </div>
      </el-col>
      <el-col :span="8">
        <div class="stat-card success">
          <div class="num">{{ onlineCount }}</div>
          <div class="label">在线</div>
        </div>
      </el-col>
      <el-col :span="8">
        <div class="stat-card danger">
          <div class="num">{{ errorCount }}</div>
          <div class="label">异常</div>
        </div>
      </el-col>
    </el-row>

    <el-row :gutter="16">
      <el-col v-for="s in servers" :key="s.id" :span="8" class="mb-12">
        <div class="card server-card" @click="goMonitor(s.id)">
          <div class="flex-between">
            <span class="server-name"><span class="status-dot" :class="s.status === 'connected' ? 'is-on' : ''" />{{ s.name }}</span>
            <el-tag size="small" :type="s.status === 'connected' ? 'success' : (s.status === 'error' ? 'danger' : 'info')">
              {{ s.status === 'connected' ? '在线' : (s.status === 'error' ? '异常' : '离线') }}
            </el-tag>
          </div>
          <div class="text-muted mt-12">{{ s.host }}:{{ s.port }}</div>
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
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
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
.el-row:first-child > .el-col:nth-child(1) .stat-card { animation-delay: 0s; }
.el-row:first-child > .el-col:nth-child(2) .stat-card { animation-delay: .06s; }
.el-row:first-child > .el-col:nth-child(3) .stat-card { animation-delay: .12s; }
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
.server-card {
  cursor: pointer;
  transition: transform .3s cubic-bezier(.16, 1, .3, 1), box-shadow .3s ease;
  animation: rise .45s cubic-bezier(.16, 1, .3, 1) both;
}
.el-row:last-child > .el-col:nth-child(1) .card { animation-delay: .04s; }
.el-row:last-child > .el-col:nth-child(2) .card { animation-delay: .08s; }
.el-row:last-child > .el-col:nth-child(3) .card { animation-delay: .12s; }
.el-row:last-child > .el-col:nth-child(4) .card { animation-delay: .16s; }
.el-row:last-child > .el-col:nth-child(5) .card { animation-delay: .20s; }
.el-row:last-child > .el-col:nth-child(6) .card { animation-delay: .24s; }
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
}
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--ink-3);
  flex-shrink: 0;
}
.status-dot.is-on {
  background: var(--green);
  animation: breathe 4s ease-in-out infinite;
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
</style>