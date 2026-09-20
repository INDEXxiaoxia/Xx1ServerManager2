<script setup>
import {ref, onMounted, onBeforeUnmount, computed} from 'vue'
import {useRouter} from 'vue-router'
import {api, events} from '@/api'

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
              <span>磁盘</span>
              <el-progress :percentage="Number(statsMap[s.id].diskUsage || 0).toFixed(1)" :stroke-width="8"
                           :color="statsMap[s.id].diskUsage > 80 ? 'var(--red)' : 'var(--yellow)'" />
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
</style>