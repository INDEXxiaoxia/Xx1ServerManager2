<script setup>
import {ref, watch, onMounted, onBeforeUnmount, nextTick} from 'vue'
import * as echarts from 'echarts'
import {useRoute} from 'vue-router'
import {api, events} from '@/api'
import {themeState, getChartTheme} from '@/theme'

const route = useRoute()
const servers = ref([])
const selectedId = ref(route.query.serverId || '')
const loading = ref(false)
const processes = ref([])

let charts = {}
let historyTimer = null
let dataCb = null
const maxPoints = 60

const cpuChartRef = ref(null)
const memChartRef = ref(null)
const diskChartRef = ref(null)
const netChartRef = ref(null)

async function loadServers() {
  servers.value = await api.listServers()
  if (!selectedId.value && servers.value.length) selectedId.value = servers.value[0].id
}

function initCharts() {
  charts.cpu = echarts.init(cpuChartRef.value)
  charts.mem = echarts.init(memChartRef.value)
  charts.disk = echarts.init(diskChartRef.value)
  charts.net = echarts.init(netChartRef.value)
  ;['cpu', 'mem', 'disk', 'net'].forEach(k => {
    charts[k].setOption(buildOption(k))
  })
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

// 主题切换：图表坐标轴 / 网格 / 系列色即时刷新（保留曲线数据）
watch(() => themeState.name, () => {
  const ct = getChartTheme(themeState.name)
  ;['cpu', 'mem', 'disk', 'net'].forEach(k => {
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

function onData(stats) {
  const t = new Date().toLocaleTimeString()
  pushPoint('cpu', t, stats.cpuUsage || 0)
  pushPoint('mem', t, stats.memUsage || 0)
  pushPoint('disk', t, stats.diskUsage || 0)
  pushPoint('net', t, (stats.netRx || 0) / 1024)
}

async function loadHistory() {
  if (!selectedId.value) return
  const points = await api.getMonitorHistory(selectedId.value, maxPoints)
  points.forEach(p => {
    const t = new Date(p.timestamp).toLocaleTimeString()
    pushPoint('cpu', t, p.cpuUsage || 0)
    pushPoint('mem', t, p.memUsage || 0)
    pushPoint('disk', t, p.diskUsage || 0)
    pushPoint('net', t, (p.netRx || 0) / 1024)
  })
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
  ;['cpu', 'mem', 'disk', 'net'].forEach(k => {
    charts[k].setOption({xAxis: {data: []}, series: [{data: []}]})
  })
  subscribe()
  start()
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
      <el-col :span="12" class="mb-12">
        <div class="card"><h4>磁盘使用率</h4><div ref="diskChartRef" class="chart" /></div>
      </el-col>
      <el-col :span="12" class="mb-12">
        <div class="card"><h4>网络传入速率</h4><div ref="netChartRef" class="chart" /></div>
      </el-col>
    </el-row>

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
</style>