<script setup>
import {computed, ref, watch, onMounted, onBeforeUnmount, onActivated, onDeactivated} from 'vue'
import {api} from '@/api'

const props = defineProps({serverId: String})
const metric = ref('rss'), auto = ref(false), loading = ref(false), error = ref('')
const memory = ref({}), rows = ref([]), sampled = ref('')
const oomText = ref(''), oomOpen = ref(false), oomBusy = ref(false)
let history = new Map(), generation = 0, timer, active = true
const cards = computed(() => {
  const m = memory.value
  return [['物理内存', m.MemTotal], ['可用内存', m.MemAvailable], ['已用交换空间',
    m.SwapTotal == null ? null : m.SwapTotal - m.SwapFree]]
})
function size(kib) {
  if (kib == null) return '—'
  return kib >= 1048576 ? (kib / 1048576).toFixed(2) + ' GiB' : (kib / 1024).toFixed(1) + ' MiB'
}
async function refresh() {
  const rev = ++generation
  loading.value = true
  error.value = ''
  const sort = metric.value === 'cpu' ? '-pcpu' : '-rss'
  const command = "export LC_ALL=C; awk '/^(MemTotal|MemAvailable|SwapTotal|SwapFree):/{print $1,$2}' /proc/meminfo && " +
    "ps -eo pid=,lstart=,user=,pcpu=,rss=,stat=,comm= --sort=" + sort
    + " | head -n 100"
  try {
    const output = await api.runRemoteCommand(props.serverId, command, 15)
    if (rev !== generation) return
    const mem = {}, next = [], samples = new Map()
    for (const line of output.trim().split('\n')) {
      const m = line.match(/^(\w+):\s+(\d+)$/)
      if (m) { mem[m[1]] = Number(m[2]); continue }
      const p = line.trim().match(/^(\d+)\s+(\S+\s+\S+\s+\d+\s+[\d:]+\s+\d+)\s+(\S+)\s+([\d.]+)\s+(\d+)\s+(\S+)\s+(.+)$/)
      if (!p) continue
      const [,pid,start,user,cpu,rss,state,name] = p
      const key = pid + ':' + start
      const trend = [...(history.get(key) || []).slice(-7), Number(rss)]
      samples.set(key, trend)
      next.push({pid,name,user,state,cpu,rss:Number(rss),trend})
    }
    if (!mem.MemTotal || !next.length) throw new Error('未能读取 /proc/meminfo 或进程列表，请确认服务器提供 Linux procps。')
    history = samples
    memory.value = mem
    rows.value = next
    sampled.value = new Date().toLocaleTimeString()
  } catch (e) { if (rev === generation) error.value = String(e) }
  finally { if (rev === generation) loading.value = false }
}
async function showOOM() {
  oomOpen.value = true
  oomBusy.value = true
  try {
    oomText.value = (await api.runRemoteCommand(props.serverId,
      "journalctl -k -b --no-pager -o short-iso -n 50 --case-sensitive=no -g 'out of memory|oom.kill|killed process'", 15)).trim()
    if (!oomText.value || oomText.value === '-- No entries --') oomText.value = '当前账户可见的本次启动日志中没有 OOM 记录'
  } catch (e) { oomText.value = '读取内核日志失败：' + e }
  finally { oomBusy.value = false }
}
function schedule() {
  clearInterval(timer)
  if (active && auto.value) timer = setInterval(() => { if (!loading.value) refresh() }, 5000)
}
watch(auto, schedule)
watch(metric, refresh)
onMounted(refresh)
onActivated(() => { active = true; schedule() })
onDeactivated(() => { active = false; clearInterval(timer) })
onBeforeUnmount(() => { generation++; clearInterval(timer) })
</script>
<template>
  <section>
    <div class="mem-summary mb-12">
      <div v-for="[label,kib] in cards" :key="label" class="card"><span>{{label}}</span><strong>{{size(kib)}}</strong></div>
    </div>
    <div class="card">
      <div class="controls mb-12">
        <el-radio-group v-model="metric" size="small"><el-radio-button value="rss">按内存</el-radio-button><el-radio-button value="cpu">按 CPU</el-radio-button></el-radio-group>
        <span class="text-muted">前 100 个进程 · {{ sampled || '等待采样' }}</span>
        <el-checkbox v-model="auto">每 5 秒刷新</el-checkbox>
        <el-button :loading="loading" @click="refresh">刷新</el-button>
        <el-button @click="showOOM" :disabled="oomBusy">OOM 记录</el-button>
      </div>
      <el-alert v-if="error" :title="error" type="error" :closable="false" />
      <el-table :data="rows" row-key="pid" stripe v-loading="loading && !rows.length">
        <el-table-column prop="pid" label="PID" width="85"/>
        <el-table-column prop="name" label="进程" min-width="150" show-overflow-tooltip/>
        <el-table-column prop="user" label="用户" width="110"/>
        <el-table-column prop="state" label="状态" width="72"/>
        <el-table-column prop="cpu" label="CPU (%)" width="96"/>
        <el-table-column label="常驻内存" width="112"><template #default="{row}">{{ size(row.rss) }}</template></el-table-column>
        <el-table-column label="内存变化" min-width="112"><template #default="{row}"><span :title="row.trend.map(size).join(' → ')">{{ row.trend.length < 2 ? '—' : (row.rss >= row.trend[0] ? '+' : '−') + size(Math.abs(row.rss-row.trend[0])) }}</span></template></el-table-column>
      </el-table>
    </div>
    <el-dialog v-model="oomOpen" title="本次启动的 OOM 记录 · 最近 50 条" width="min(920px, 94vw)" top="10vh">
      <div class="oom-body" v-loading="oomBusy" element-loading-text="正在读取内核日志…">
        <pre v-if="!oomBusy">{{ oomText }}</pre>
      </div>
    </el-dialog>
  </section>
</template>
<style scoped>
.mem-summary {display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:12px}
.mem-summary span {color:var(--ink-3);font-size:13px}
.mem-summary strong {display:block;font-size:23px;margin-top:5px;color:var(--ink)}
.controls {display:flex;align-items:center;gap:10px;flex-wrap:wrap}
.oom-body {min-height:min(340px,52vh);max-height:62vh;overflow:auto}
pre {margin:0;padding:12px;white-space:pre-wrap;overflow-wrap:anywhere}
</style>
