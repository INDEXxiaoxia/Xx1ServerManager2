<script setup>
import {ref, computed, watch, onMounted, onBeforeUnmount, nextTick} from 'vue'
import * as echarts from 'echarts'
import {useRoute} from 'vue-router'
import {api, events} from '@/api'
import {themeState, getChartTheme, diskColorOf} from '@/theme'

const route = useRoute()
const servers = ref([])
const selectedId = ref(route.query.serverId || '')
const loading = ref(false)
const processes = ref([])

// ---- 磁盘状态 ----
const currentDisks = ref([])     // 最新一次采集的磁盘列表（含伪文件系统，是否显示由开关决定）
const showAllFs = ref(false)     // 显示 tmpfs 等伪文件系统（排查容器环境时用）
const diskFocus = ref('all')     // 折线图聚焦的挂载点，all 表示按规则自动选
const diskPoints = ref([])       // 磁盘时间序列：{t, map:{挂载点: 使用率}}

let charts = {}
let historyTimer = null
let dataCb = null
const maxPoints = 60

const cpuChartRef = ref(null)
const memChartRef = ref(null)
const diskChartRef = ref(null)
const netChartRef = ref(null)

// 可见磁盘：默认过滤伪文件系统
const visibleDisks = computed(() => {
  const all = currentDisks.value || []
  return showAllFs.value ? all : all.filter(d => !d.isPseudo)
})
// 已知挂载点（用于给每块盘分配稳定颜色，伪文件系统也参与，保证开关切换时颜色不跳）
const allMounts = computed(() => (currentDisks.value || []).map(d => d.mount))
const maxUsage = computed(() => visibleDisks.value.reduce((m, d) => Math.max(m, Number(d.usage) || 0), 0))

// 折线图实际绘制的盘：聚焦单个时只画它；超过 6 块时只画使用率最高的 5 块
const chartDisks = computed(() => {
  const list = visibleDisks.value
  if (diskFocus.value !== 'all') {
    const one = list.find(d => d.mount === diskFocus.value)
    return one ? [one] : []
  }
  if (list.length > 6) {
    return list.slice().sort((a, b) => (Number(b.usage) || 0) - (Number(a.usage) || 0)).slice(0, 5)
  }
  return list
})
const diskHiddenCount = computed(() =>
  diskFocus.value === 'all' ? Math.max(0, visibleDisks.value.length - chartDisks.value.length) : 0
)

async function loadServers() {
  servers.value = await api.listServers()
  if (!selectedId.value && servers.value.length) selectedId.value = servers.value[0].id
}

function initCharts() {
  charts.cpu = echarts.init(cpuChartRef.value)
  charts.mem = echarts.init(memChartRef.value)
  charts.net = echarts.init(netChartRef.value)
  charts.disk = echarts.init(diskChartRef.value)
  ;['cpu', 'mem', 'net'].forEach(k => {
    charts[k].setOption(buildOption(k))
  })
  renderDiskChart(true)
}

function buildOption(kind) {
  const ct = getChartTheme(themeState.name)
  const color = ct.series[kind] || ct.series.cpu
  const cfg = {
    cpu: {name: 'CPU%', max: 100},
    mem: {name: '内存%', max: 100},
    disk: {name: '磁盘%', max: 100},
    net: {name: '网络 KB/s'}
  }[kind] || {name: kind}
  return {
    tooltip: {
      trigger: 'axis',
      backgroundColor: ct.tooltipBg,
      borderColor: ct.tooltipBorder,
      textStyle: {color: ct.tooltipText}
    },
    grid: {left: 44, right: 20, top: 30, bottom: 30},
    xAxis: {
      type: 'category',
      data: [],
      boundaryGap: false,
      axisLabel: {color: ct.axis},
      axisLine: {lineStyle: {color: ct.line}},
      axisTick: {lineStyle: {color: ct.line}}
    },
    yAxis: {
      type: 'value',
      max: cfg.max,
      axisLabel: {color: ct.axis},
      axisLine: {lineStyle: {color: ct.line}},
      splitLine: {lineStyle: {color: ct.split}}
    },
    series: [{
      type: 'line',
      name: cfg.name,
      color,
      smooth: true,
      showSymbol: false,
      data: [],
      lineStyle: {width: 2},
      areaStyle: {
        color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
          {offset: 0, color: color + '2E'},
          {offset: 1, color: color + '00'}
        ])
      }
    }]
  }
}

// 磁盘折线图：一块盘一条线，颜色按挂载点稳定分配；reset=true 时整体替换系列（系列集合变了）
function renderDiskChart(reset = false) {
  const c = charts.disk
  if (!c) return
  const ct = getChartTheme(themeState.name)
  const mounts = allMounts.value
  const picked = chartDisks.value
  const series = picked.map(d => ({
    name: d.mount,
    type: 'line',
    smooth: true,
    showSymbol: false,
    lineStyle: {width: 2},
    itemStyle: {color: diskColorOf(themeState.name, d.mount, mounts)},
    data: diskPoints.value.map(p => (p.map[d.mount] ?? null))
  }))
  if (series.length) {
    // 80% 警戒虚线只挂在第一条线上，避免多系列各画一条
    series[0].markLine = {
      silent: true,
      symbol: 'none',
      label: {formatter: '80%', color: ct.axis, fontSize: 11},
      lineStyle: {type: 'dashed', color: ct.line, width: 1},
      data: [{yAxis: 80}]
    }
  }
  const option = {
    tooltip: {
      trigger: 'axis',
      order: 'valueDesc', // 按使用率降序，一眼看出谁最紧张
      backgroundColor: ct.tooltipBg,
      borderColor: ct.tooltipBorder,
      textStyle: {color: ct.tooltipText}
    },
    legend: {
      show: picked.length > 1,
      type: 'scroll',
      top: 0,
      right: 8,
      icon: 'roundRect',
      itemWidth: 10,
      itemHeight: 8,
      textStyle: {color: ct.axis, fontSize: 11},
      data: picked.map(d => d.mount)
    },
    grid: {left: 44, right: 20, top: picked.length > 1 ? 34 : 20, bottom: 30},
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: diskPoints.value.map(p => p.t),
      axisLabel: {color: ct.axis},
      axisLine: {lineStyle: {color: ct.line}},
      axisTick: {lineStyle: {color: ct.line}}
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: 100,
      axisLabel: {color: ct.axis, formatter: '{value}%'},
      axisLine: {lineStyle: {color: ct.line}},
      splitLine: {lineStyle: {color: ct.split}}
    },
    series
  }
  if (reset) c.setOption(option, {replaceMerge: ['series', 'legend']})
  else c.setOption(option)
}

// 主题切换：图表坐标轴 / 网格 / 系列色即时刷新（保留曲线数据）
watch(() => themeState.name, () => {
  const ct = getChartTheme(themeState.name)
  ;['cpu', 'mem', 'net'].forEach(k => {
    const c = charts[k]
    if (!c) return
    c.setOption({
      xAxis: {
        axisLabel: {color: ct.axis},
        axisLine: {lineStyle: {color: ct.line}},
        axisTick: {lineStyle: {color: ct.line}}
      },
      yAxis: {
        axisLabel: {color: ct.axis},
        axisLine: {lineStyle: {color: ct.line}},
        splitLine: {lineStyle: {color: ct.split}}
      },
      tooltip: {
        backgroundColor: ct.tooltipBg,
        borderColor: ct.tooltipBorder,
        textStyle: {color: ct.tooltipText}
      },
      series: [{color: ct.series[k] || ct.series.cpu}]
    })
  })
  renderDiskChart(true)
})

// 系列集合的指纹：只有挂载点集合真的变了才整体替换系列，逐次采集走合并更新（曲线不重画）
const chartKey = computed(() => chartDisks.value.map(d => d.mount).join('|'))
watch(chartKey, () => renderDiskChart(true))

// 关掉"显示全部"后，若聚焦的盘已被隐藏则回到自动模式
watch(showAllFs, () => {
  if (diskFocus.value !== 'all' && !visibleDisks.value.some(d => d.mount === diskFocus.value)) {
    diskFocus.value = 'all'
  }
})

function pushPoint(kind, time, value, unit = 1) {
  const opt = charts[kind].getOption()
  opt.xAxis[0].data.push(time)
  opt.series[0].data.push(Number((value * unit).toFixed(1)))
  if (opt.xAxis[0].data.length > maxPoints) {
    opt.xAxis[0].data.shift()
    opt.series[0].data.shift()
  }
  charts[kind].setOption(opt)
}

// recordDiskPoint 记录一次磁盘采样；采集失败（disks 为空）时保留上一份列表，避免面板闪成"暂无数据"
function recordDiskPoint(disks, t) {
  if (Array.isArray(disks) && disks.length) currentDisks.value = disks
  // 聚焦的挂载点已经消失（卸载 / 被"显示全部"关掉）时回到自动模式，避免下拉停在空值
  if (diskFocus.value !== 'all' && !visibleDisks.value.some(d => d.mount === diskFocus.value)) {
    diskFocus.value = 'all'
  }
  const map = {}
  ;(disks || []).forEach(d => {
    map[d.mount] = Number(d.usage) || 0
  })
  diskPoints.value.push({t, map})
  if (diskPoints.value.length > maxPoints) diskPoints.value.shift()
}

function onData(stats) {
  const t = new Date().toLocaleTimeString()
  pushPoint('cpu', t, stats.cpuUsage || 0)
  pushPoint('mem', t, stats.memUsage || 0)
  pushPoint('net', t, (stats.netRx || 0) / 1024)
  recordDiskPoint(stats.disks, t)
  renderDiskChart()
}

// historyDisks 历史点的磁盘快照；极端情况下（后端未回填）退化为旧字段构成的根分区一条
function historyDisks(p) {
  if (Array.isArray(p.disks) && p.disks.length) return p.disks
  if (p.diskUsage || p.diskTotal) {
    return [{
      mount: '/',
      totalMB: p.diskTotal || 0,
      usedMB: p.diskUsed || 0,
      usage: p.diskUsage || 0
    }]
  }
  return []
}

async function loadHistory() {
  if (!selectedId.value) return
  const points = await api.getMonitorHistory(selectedId.value, maxPoints)
  diskPoints.value = []
  points.forEach(p => {
    const t = new Date(p.timestamp).toLocaleTimeString()
    pushPoint('cpu', t, p.cpuUsage || 0)
    pushPoint('mem', t, p.memUsage || 0)
    pushPoint('net', t, (p.netRx || 0) / 1024)
    recordDiskPoint(historyDisks(p), t)
  })
  renderDiskChart(true)
}

async function loadProcesses() {
  if (!selectedId.value) return
  processes.value = await api.getTopProcesses(selectedId.value, 10)
}

function start() {
  if (!selectedId.value) return
  loading.value = true
  try {
    api.startMonitor(selectedId.value)
    loadHistory()
    loadProcesses()
  } finally {
    loading.value = false
  }
}

function subscribe() {
  if (dataCb) events.off('monitor:data:' + lastSubscribed, dataCb)
  dataCb = (stats) => onData(stats)
  lastSubscribed = selectedId.value
  events.on('monitor:data:' + lastSubscribed, dataCb)
}
let lastSubscribed = ''

function onChange() {
  api.stopAllMonitors()
  ;['cpu', 'mem', 'net'].forEach(k => {
    charts[k].setOption({xAxis: {data: []}, series: [{data: []}]})
  })
  diskPoints.value = []
  currentDisks.value = []
  diskFocus.value = 'all'
  renderDiskChart(true)
  subscribe()
  start()
}

function toggleFocus(mount) {
  diskFocus.value = diskFocus.value === mount ? 'all' : mount
}

function fmtBytes(mb) {
  const v = Number(mb) || 0
  if (v >= 1024) return (v / 1024).toFixed(1) + ' GB'
  return v.toFixed(0) + ' MB'
}

function fmtPct(v) {
  return (Number(v) || 0).toFixed(1)
}

function diskColor(mount) {
  return diskColorOf(themeState.name, mount, allMounts.value)
}

onMounted(async () => {
  await loadServers()
  await nextTick()
  initCharts()
  subscribe()
  start()
  historyTimer = setInterval(loadProcesses, 10000)
})

onBeforeUnmount(() => {
  if (historyTimer) clearInterval(historyTimer)
  api.stopAllMonitors()
  if (dataCb && lastSubscribed) events.off('monitor:data:' + lastSubscribed, dataCb)
  Object.values(charts).forEach(c => c.dispose())
})
</script>

<template>
  <div class="page-container">
    <div class="card mb-12">
      <div class="flex-between">
        <h3>实时监控</h3>
        <el-select v-model="selectedId" placeholder="选择服务器" @change="onChange" style="width: 260px">
          <el-option v-for="s in servers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
        </el-select>
      </div>
    </div>

    <el-row :gutter="16">
      <el-col :span="12" class="mb-12">
        <div class="card"><h4>CPU 使用率</h4><div ref="cpuChartRef" class="chart" /></div>
      </el-col>
      <el-col :span="12" class="mb-12">
        <div class="card"><h4>内存使用率</h4><div ref="memChartRef" class="chart" /></div>
      </el-col>
    </el-row>

    <!-- 磁盘：整行卡片，左侧各挂载点明细，右侧一块盘一条曲线 -->
    <div class="card mb-12">
      <div class="flex-between disk-head">
        <h4>磁盘使用率</h4>
        <div class="disk-tools">
          <span v-if="diskHiddenCount" class="disk-hint">另有 {{ diskHiddenCount }} 个文件系统未显示</span>
          <el-select v-model="diskFocus" size="small" style="width: 176px">
            <el-option label="全部磁盘" value="all" />
            <el-option v-for="d in visibleDisks" :key="d.mount" :label="`${d.mount} · ${fmtPct(d.usage)}%`" :value="d.mount" />
          </el-select>
          <el-checkbox v-model="showAllFs" size="small">显示全部文件系统</el-checkbox>
        </div>
      </div>
      <div class="disk-body mt-12">
        <div class="disk-list">
          <template v-if="visibleDisks.length">
            <div v-for="d in visibleDisks" :key="d.mount" class="disk-row"
                 :class="{active: diskFocus === d.mount}" @click="toggleFocus(d.mount)">
              <div class="disk-row-top">
                <span class="disk-mount" :title="d.device || d.mount">
                  {{ d.mount }}<span v-if="d.isReadOnly" class="disk-ro">只读</span>
                </span>
                <span class="disk-pct" :style="{color: (d.usage >= 80 && !d.isReadOnly) ? 'var(--red)' : 'var(--ink-2)'}">
                  {{ fmtPct(d.usage) }}%<span v-if="d.usage >= 80 && !d.isReadOnly"> ⚠</span>
                </span>
              </div>
              <el-progress :percentage="Number(Number(d.usage).toFixed(1))" :stroke-width="6" :show-text="false"
                           :color="diskColor(d.mount)" />
              <div class="disk-sub">
                {{ fmtBytes(d.usedMB) }} / {{ fmtBytes(d.totalMB) }}<span v-if="d.fstype"> · {{ d.fstype }}</span>
              </div>
            </div>
            <div class="disk-sum">{{ visibleDisks.length }} 个文件系统 · 最大 {{ fmtPct(maxUsage) }}%</div>
          </template>
          <div v-else class="text-muted">暂无磁盘数据</div>
        </div>
        <div ref="diskChartRef" class="disk-chart" />
      </div>
    </div>

    <div class="card mb-12">
      <h4>网络传入速率</h4><div ref="netChartRef" class="chart" />
    </div>

    <div class="card">
      <h4>内存占用 TOP10 进程</h4>
      <el-table :data="processes" stripe size="small" v-loading="loading" class="mt-12">
        <el-table-column prop="user" label="用户" width="100" />
        <el-table-column prop="pid" label="PID" width="80" />
        <el-table-column prop="memPercent" label="内存%" width="90" />
        <el-table-column prop="memMB" label="内存(MB)" width="100" />
        <el-table-column prop="cpuPercent" label="CPU%" width="90" />
        <el-table-column prop="command" label="命令" show-overflow-tooltip />
      </el-table>
    </div>
  </div>
</template>

<style scoped>
.chart { height: 260px; }
h4 { margin-bottom: 8px; color: var(--ink-2); }
.disk-head { align-items: center; }
.disk-tools { display: flex; align-items: center; gap: 10px; }
.disk-hint { font-size: 12px; color: var(--ink-3); }
.disk-body { display: flex; gap: 18px; }
.disk-list { width: 264px; flex-shrink: 0; max-height: 260px; overflow-y: auto; padding-right: 4px; }
.disk-row {
  padding: 6px 8px;
  border-radius: 8px;
  cursor: pointer;
  transition: background .2s ease;
}
.disk-row + .disk-row { margin-top: 2px; }
.disk-row:hover { background: var(--panel-2); }
.disk-row.active { background: var(--panel-2); box-shadow: inset 0 0 0 1px var(--line); }
.disk-row-top { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 4px; font-size: 12px; }
.disk-mount { font-weight: 600; color: var(--ink-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.disk-ro { margin-left: 6px; font-weight: 400; color: var(--ink-3); }
.disk-pct { flex-shrink: 0; }
.disk-sub { margin-top: 3px; font-size: 11px; color: var(--ink-3); }
.disk-sum { margin-top: 8px; font-size: 12px; color: var(--ink-3); }
.disk-chart { flex: 1; height: 260px; min-width: 0; }
</style>