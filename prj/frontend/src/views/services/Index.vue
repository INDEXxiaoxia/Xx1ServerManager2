<script setup>
import {ref, onMounted} from 'vue'
import {useRoute} from 'vue-router'
import {ElMessage, ElMessageBox} from 'element-plus'
import {api} from '@/api'

const route = useRoute()
const servers = ref([])
const selectedId = ref(route.query.serverId || '')
const services = ref([])
const loading = ref(false)
const keyword = ref('')
const statusDialog = ref(false)
const statusText = ref('')
const statusTitle = ref('')

const filtered = () => {
  const rows = services.value
  if (!keyword.value) return rows
  const k = keyword.value.toLowerCase()
  return rows.filter(r => r.name.toLowerCase().includes(k) || r.description.toLowerCase().includes(k))
}

async function loadServers() {
  servers.value = await api.listServers()
  if (!selectedId.value && servers.value.length) selectedId.value = servers.value[0].id
}

async function loadServices() {
  if (!selectedId.value) return
  loading.value = true
  try {
    services.value = await api.getServicesWithEnabled(selectedId.value)
  } catch (e) {
    ElMessage.error('加载服务失败：' + e)
  } finally {
    loading.value = false
  }
}

async function onChange() {
  await loadServices()
}

function stateType(srv) {
  if (srv.activeState === 'active') return 'success'
  if (srv.activeState === 'failed') return 'danger'
  return 'info'
}

async function action(row, act) {
  const label = {start: '启动', stop: '停止', restart: '重启', enable: '设置自启', disable: '取消自启'}[act]
  if (['stop'].includes(act)) {
    await ElMessageBox.confirm(`确定${label}服务「${row.name}」吗？`, '提示', {type: 'warning'})
  }
  try {
    const r = await api.serviceAction(selectedId.value, row.name, act)
    if (r.status === 'success') ElMessage.success(`${row.name} ${label}成功`)
    else ElMessage.error(`${row.name} ${label}失败：` + (r.stderr || r.stdout))
    loadServices()
  } catch (e) {
    ElMessage.error(`${label}失败：` + e)
  }
}

async function showStatus(row) {
  statusTitle.value = row.name
  statusText.value = await api.getServiceStatus(selectedId.value, row.name)
  statusDialog.value = true
}

onMounted(async () => {
  await loadServers()
  await loadServices()
})
</script>

<template>
  <div class="page-container">
    <div class="card mb-12">
      <div class="flex-between">
        <div class="flex-between" style="gap: 12px">
          <el-select v-model="selectedId" placeholder="选择服务器" @change="onChange" style="width: 240px">
            <el-option v-for="s in servers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
          </el-select>
          <el-input v-model="keyword" placeholder="搜索服务" clearable style="width: 220px" />
        </div>
        <el-button :loading="loading" @click="loadServices">刷新</el-button>
      </div>
    </div>

    <div class="card">
      <el-table :data="filtered()" v-loading="loading" stripe>
        <el-table-column prop="name" label="服务名" min-width="200" class-name="mono" />
        <el-table-column prop="description" label="描述" min-width="200" show-overflow-tooltip />
        <el-table-column label="状态" width="110">
          <template #default="{row}">
            <el-tag :type="stateType(row)" size="small">{{ row.activeState }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="自启" width="80">
          <template #default="{row}">
            <el-tag :type="row.enabled === 'enabled' ? 'success' : 'info'" size="small">
              {{ row.enabled === 'enabled' ? '是' : '否' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="300" fixed="right">
          <template #default="{row}">
            <el-button size="small" text type="success" @click="action(row, 'start')">启动</el-button>
            <el-button size="small" text type="warning" @click="action(row, 'restart')">重启</el-button>
            <el-button size="small" text type="danger" @click="action(row, 'stop')">停止</el-button>
            <el-button size="small" text @click="action(row, row.enabled === 'enabled' ? 'disable' : 'enable')">自启</el-button>
            <el-button size="small" text type="primary" @click="showStatus(row)">状态</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="statusDialog" :title="`服务状态 - ${statusTitle}`" width="600px">
      <pre class="mono status-text">{{ statusText || '（无输出）' }}</pre>
    </el-dialog>
  </div>
</template>

<style scoped>
.status-text {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 12px;
  border-radius: 6px;
  max-height: 400px;
  overflow-y: auto;
}
</style>