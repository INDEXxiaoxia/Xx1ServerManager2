<script setup>
import {ref, computed, watch, onMounted} from 'vue'
import {ElMessage, ElMessageBox} from 'element-plus'
import {api} from '@/api'
import {useRouter} from 'vue-router'
import {containerPorts, portDescription} from '@/utils/container-ports'
const router = useRouter()
const keyword = ref('')
const labels = {start:'启动',stop:'停止',restart:'重启',rm:'删除'}
import GroupedServerSelect from '@/components/GroupedServerSelect.vue'
const servers = ref([])
const serverId = ref(localStorage.getItem('xx1.container.server') || '')
const engine = ref('docker')
const rows = ref([])
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const detail = ref('')
const detailTitle = ref('')
const visible = ref(false)
const filteredRows = computed(() => rows.value.filter(r => `${r.Names} ${r.Image} ${r.ID}`.toLowerCase().includes(keyword.value.trim().toLowerCase())))
const isRunning = row => String(row.State).toLowerCase() === 'running' || (!row.State && /^Up\b/i.test(row.Status || ''))
function terminal(row) {
  if (busy.value || !isRunning(row)) return
  router.push({path:'/terminal',query:{serverId:serverId.value,containerId:row.ID,engine:engine.value,containerName: String(row.Names)}})
}
function more(command,row) { if (command === 'inspect') inspect(row,false); else action(row,command) }
let generation = 0
async function load() {
  const token = ++generation, id = serverId.value, bin = engine.value
  rows.value = []; error.value = ''
  if (!id) return
  loading.value = true
  try {
    const output = await api.runRemoteCommand(id, bin + " ps -a --no-trunc --format '{{json .}}'", 30)
    if (token !== generation) return
    rows.value = output.trim().split('\n').filter(Boolean).map(line => { const row = JSON.parse(line); return {...row, portMappings:containerPorts(row.Ports)} })
  } catch(e) { if (token === generation) error.value = String(e) }
  finally { if (token === generation) loading.value = false }
}
async function action(row, operation) {
  if (busy.value || !/^[a-f0-9]{12,64}$/.test(row.ID)) return
  const id = serverId.value, bin = engine.value
  if (['stop','restart','rm'].includes(operation)) {
    try { await ElMessageBox.confirm(`确认对 ${row.Names} 执行${labels[operation]}？${operation === 'rm' ? '仅删除已停止的容器，不删除卷。' : ''}`, '容器操作', {type:'warning'}) } catch { return }
  }
  busy.value = true
  try {
    await api.runRemoteCommand(id, `${bin} ${operation} ${row.ID}`, 60)
    ElMessage.success('操作完成')
    if (serverId.value === id && engine.value === bin) await load()
  } catch(e) { ElMessage.error(String(e)) }
  finally { busy.value = false }
}
async function inspect(row, logs) {
  if (busy.value || !/^[a-f0-9]{12,64}$/.test(row.ID)) return
  const id = serverId.value, bin = engine.value
  busy.value = true; detail.value = ''; visible.value = true
  detailTitle.value = row.Names + (logs ? ' · 最近 200 行日志' : ' · 容器详情')
  try { detail.value = await api.runRemoteCommand(id, `${bin} ${logs ? 'logs --tail 200 --timestamps' : 'inspect'} ${row.ID} 2>&1`, 30) }
  catch(e) { detail.value = String(e) }
  finally { busy.value = false }
}
watch([serverId, engine], () => { localStorage.setItem('xx1.container.server',serverId.value); load() })
onMounted(async () => { servers.value = await api.listServers(); if (!servers.value.some(s => s.id === serverId.value)) serverId.value = servers.value[0]?.id || ''; else load() })
</script>
<template>
  <div class="page-container">
    <div class="card">
      <div class="container-head">
        <div class="container-filters">
          <GroupedServerSelect v-model="serverId" :servers="servers" :disabled="busy" class="server-select" />
          <el-select v-model="engine" class="engine-select" :disabled="busy"><el-option label="Docker" value="docker"/><el-option label="Podman" value="podman"/></el-select>
          <el-input v-model="keyword" placeholder="搜索容器 / 镜像" clearable class="container-search"><template #prefix><el-icon><Search /></el-icon></template></el-input>
          <span class="text-muted">共 {{ filteredRows.length }} 个</span>
        </div>
        <el-button @click="load" :loading="loading" :disabled="busy"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
      </div>
      <el-alert v-if="error" :title="error" description="请确认服务器已安装所选容器引擎，且 SSH 用户有访问权限。" type="error" :closable="false" />
      <el-table :data="filteredRows" v-loading="loading" empty-text="暂无匹配容器" stripe row-key="ID">
        <el-table-column prop="Names" label="容器" min-width="150" show-overflow-tooltip/>
        <el-table-column prop="Image" label="镜像" min-width="180" show-overflow-tooltip/>
        <el-table-column label="状态" min-width="160"><template #default="{row}">
          <el-tag :type="isRunning(row) ? 'success' : 'info'" size="small">{{ row.Status }}</el-tag>
        </template></el-table-column>
        <el-table-column label="端口映射" min-width="220"><template #header>
          <el-tooltip content="宿主机端口 → 容器端口；悬停查看监听地址"><span>端口映射 <span class="text-muted">ⓘ</span></span></el-tooltip>
        </template><template #default="{row}">
          <div v-if="row.portMappings.length" class="ports">
            <el-tooltip v-for="port in row.portMappings.slice(0,2)" :key="port.key" :content="portDescription(port)" placement="top">
              <span class="port-chip"><template v-if="!port.raw"><b v-if="port.published">{{ port.published }} <span class="port-arrow">→</span> </b>{{ port.target }}<small>{{ port.protocol }}</small><small v-if="!port.published">未发布</small></template><template v-else>{{ port.raw }}</template></span>
            </el-tooltip>
            <el-popover v-if="row.portMappings.length > 2" trigger="click" placement="bottom" :width="420">
              <template #reference><el-button size="small" text type="primary">+{{ row.portMappings.length - 2 }} 更多</el-button></template>
              <div v-for="port in row.portMappings" :key="port.key" class="port-detail">{{ portDescription(port) }}</div>
            </el-popover>
          </div>
          <span v-else class="text-muted">—</span>
        </template></el-table-column>
        <el-table-column label="操作" width="180" fixed="right"><template #default="{row}">
          <div class="op-bar">
            <el-button size="small" text type="primary" :disabled="busy" @click="inspect(row,true)">日志</el-button>
            <el-tooltip :disabled="isRunning(row)" content="请先启动容器"><span><el-button size="small" text type="primary" :disabled="busy || !isRunning(row)" @click="terminal(row)">终端</el-button></span></el-tooltip>
            <el-dropdown trigger="click" :disabled="busy" @command="cmd => more(cmd,row)">
              <el-button size="small" text class="more-btn" :disabled="busy">更多<el-icon class="el-icon--right"><ArrowDown /></el-icon></el-button>
              <template #dropdown><el-dropdown-menu>
                <el-dropdown-item command="inspect">容器详情</el-dropdown-item>
                <el-dropdown-item command="start" :disabled="isRunning(row)" divided>启动</el-dropdown-item>
                <el-dropdown-item command="stop" :disabled="!isRunning(row)">停止</el-dropdown-item>
                <el-dropdown-item command="restart">重启</el-dropdown-item>
                <el-dropdown-item command="rm" :disabled="isRunning(row)" divided>删除容器</el-dropdown-item>
              </el-dropdown-menu></template>
            </el-dropdown>
          </div>
        </template></el-table-column>
      </el-table>
    </div>
    <el-drawer v-model="visible" :title="detailTitle" size="65%" append-to-body><pre class="container-output" v-loading="busy">{{ detail }}</pre></el-drawer>
  </div>
</template>
<style scoped>
.container-head {display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;margin-bottom:12px}
.container-filters {display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.server-select {width:240px}.engine-select {width:110px}.container-search {width:220px}
.op-bar {display:flex;align-items:center;white-space:nowrap}
.op-bar .el-button {padding-left:6px;padding-right:6px;margin-left:0}
.more-btn {color:var(--ink-3)}
.ports {display:flex;align-items:center;gap:4px;flex-wrap:wrap;padding:3px 0}
.port-chip {display:inline-flex;align-items:center;gap:5px;padding:2px 7px;border:1px solid var(--line);border-radius:5px;background:var(--panel-2);color:var(--ink-2);font:12px/1.6 Consolas,monospace}
.port-chip b {font-weight:500;color:var(--blue)}
.port-chip small,.port-arrow {color:var(--ink-3);font-size:10px}
.port-detail {overflow-wrap:anywhere;padding:6px 0;border-bottom:1px solid var(--line);font:12px/1.6 Consolas,monospace}
.container-output {white-space:pre-wrap;overflow-wrap:anywhere;background:var(--terminal-bg);color:var(--terminal-fg);padding:16px;border-radius:10px;min-height:160px}
</style>
