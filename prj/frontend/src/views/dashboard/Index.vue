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
            <span class="server-name">{{ s.name }}</span>
            <el-tag size="small" :type="s.status === 'connected' ? 'success' : (s.status === 'error' ? 'danger' : 'info')">
              {{ s.status === 'connected' ? '在线' : (s.status === 'error' ? '异常' : '离线') }}
            </el-tag>
          </div>
          <div class="text-muted mt-12">{{ s.host }}:{{ s.port }}</div>
          <template v-if="statsMap[s.id]">
            <div class="metric mt-12">
              <span>CPU</span>
              <el-progress :percentage="Number(statsMap[s.id].cpuUsage || 0).toFixed(1)" :stroke-width="8"
                           :color="statsMap[s.id].cpuUsage > 80 ? '#f56c6c' : '#409eff'" />
            </div>
            <div class="metric">
              <span>内存</span>
              <el-progress :percentage="Number(statsMap[s.id].memUsage || 0).toFixed(1)" :stroke-width="8"
                           :color="statsMap[s.id].memUsage > 80 ? '#f56c6c' : '#67c23a'" />
            </div>
            <div class="metric">
              <span>磁盘</span>
              <el-progress :percentage="Number(statsMap[s.id].diskUsage || 0).toFixed(1)" :stroke-width="8"
                           :color="statsMap[s.id].diskUsage > 80 ? '#f56c6c' : '#e6a23c'" />
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
  background: #fff;
  border-radius: 8px;
  padding: 20px;
  text-align: center;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.06);
}
.stat-card .num { font-size: 32px; font-weight: 700; color: #409eff; }
.stat-card.success .num { color: #67c23a; }
.stat-card.danger .num { color: #f56c6c; }
.stat-card .label { color: #909399; margin-top: 4px; }
.server-card { cursor: pointer; transition: all 0.2s; }
.server-card:hover { transform: translateY(-2px); box-shadow: 0 4px 12px rgba(0,0,0,0.1); }
.server-name { font-weight: 600; font-size: 15px; }
.metric { margin-top: 8px; display: flex; align-items: center; gap: 10px; }
.metric span { width: 34px; color: #606266; font-size: 13px; flex-shrink: 0; }
.metric .el-progress { flex: 1; }
</style>