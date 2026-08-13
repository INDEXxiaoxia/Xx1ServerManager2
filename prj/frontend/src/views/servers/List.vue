<script setup>
import {ref, onMounted, computed} from 'vue'
import {useRouter} from 'vue-router'
import {ElMessage, ElMessageBox} from 'element-plus'
import {api} from '@/api'
import {useServerStore} from '@/store'
import ServerFormDialog from '@/components/ServerFormDialog.vue'

const router = useRouter()
const serverStore = useServerStore()

const list = ref([])
const groups = ref([])
const loading = ref(false)
const keyword = ref('')
const groupFilter = ref('')
const dialogVisible = ref(false)
const editing = ref(null)
const selected = ref([])

const groupMap = computed(() => {
  const m = {}
  groups.value.forEach(g => { m[g.id] = g.name })
  return m
})

const filtered = computed(() => {
  let rows = [...list.value]
  if (keyword.value) {
    const k = keyword.value.toLowerCase()
    rows = rows.filter(r => r.name.toLowerCase().includes(k) || r.host.toLowerCase().includes(k))
  }
  if (groupFilter.value) {
    rows = rows.filter(r => (r.groupIds || []).includes(groupFilter.value))
  }
  return rows
})

async function load() {
  loading.value = true
  try {
    list.value = await api.listServers()
    groups.value = await api.listGroups()
  } catch (e) {
    ElMessage.error('加载失败：' + e)
  } finally {
    loading.value = false
  }
}

onMounted(load)

function openAdd() {
  editing.value = null
  dialogVisible.value = true
}

function openEdit(row) {
  editing.value = row
  dialogVisible.value = true
}

async function remove(row) {
  await ElMessageBox.confirm(`确定删除服务器「${row.name}」吗？`, '提示', {type: 'warning'})
  await api.deleteServer(row.id)
  ElMessage.success('已删除')
  load()
}

async function batchDelete() {
  if (!selected.value.length) return
  await ElMessageBox.confirm(`确定删除选中的 ${selected.value.length} 台服务器吗？`, '提示', {type: 'warning'})
  await api.deleteServers(selected.value.map(r => r.id))
  ElMessage.success('已删除')
  load()
}

async function testRow(row) {
  try {
    const r = await api.testStoredServer(row.id)
    if (r.connected) ElMessage.success(`${row.name}：` + r.message)
    else ElMessage.error(`${row.name}：` + r.message)
  } catch (e) {
    ElMessage.error('测试失败：' + e)
  }
}

function statusTag(status) {
  const map = {
    connected: {type: 'success', text: '已连接'},
    disconnected: {type: 'info', text: '未连接'},
    connecting: {type: 'warning', text: '连接中'},
    error: {type: 'danger', text: '错误'}
  }
  return map[status] || {type: 'info', text: status}
}

function goMonitor(row) {
  router.push({path: '/monitor', query: {serverId: row.id}})
}
function goTerminal(row) {
  router.push({path: '/terminal', query: {serverId: row.id}})
}
function goFiles(row) {
  router.push({path: '/files', query: {serverId: row.id}})
}
function goServices(row) {
  router.push({path: '/services', query: {serverId: row.id}})
}

function handleMore(command, row) {
  if (command === 'test') testRow(row)
  else if (command === 'edit') openEdit(row)
  else if (command === 'delete') remove(row)
}
</script>

<template>
  <div class="page-container">
    <div class="card">
      <div class="flex-between mb-12">
        <div>
          <el-input v-model="keyword" placeholder="搜索名称/主机" clearable style="width: 220px; margin-right: 8px" />
          <el-select v-model="groupFilter" placeholder="按分组筛选" clearable style="width: 160px">
            <el-option v-for="g in groups" :key="g.id" :label="g.name" :value="g.id" />
          </el-select>
        </div>
        <div>
          <el-button type="danger" plain :disabled="!selected.length" @click="batchDelete">批量删除</el-button>
          <el-button type="primary" @click="openAdd">新增服务器</el-button>
        </div>
      </div>

      <el-table :data="filtered" v-loading="loading" @selection-change="v => selected = v" stripe>
        <el-table-column type="selection" width="45" />
        <el-table-column prop="name" label="名称" min-width="140" show-overflow-tooltip />
        <el-table-column prop="host" label="主机" min-width="140" />
        <el-table-column prop="port" label="端口" width="80" />
        <el-table-column prop="username" label="用户名" width="110" />
        <el-table-column label="分组" min-width="140">
          <template #default="{row}">
            <el-tag v-for="gid in (row.groupIds || [])" :key="gid" size="small" class="mr-1">
              {{ groupMap[gid] || gid }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{row}">
            <el-tag :type="statusTag(row.status).type" size="small">{{ statusTag(row.status).text }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="210" fixed="right">
          <template #default="{row}">
            <div class="op-bar">
              <el-button size="small" text type="primary" @click="goMonitor(row)">监控</el-button>
              <el-button size="small" text type="primary" @click="goTerminal(row)">终端</el-button>
              <el-button size="small" text type="primary" @click="goFiles(row)">文件</el-button>
              <el-button size="small" text type="primary" @click="goServices(row)">服务</el-button>
              <el-dropdown trigger="click" @command="cmd => handleMore(cmd, row)">
                <el-button size="small" text class="more-btn">
                  更多<el-icon class="el-icon--right"><ArrowDown /></el-icon>
                </el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item command="test">测试连接</el-dropdown-item>
                    <el-dropdown-item command="edit">编辑</el-dropdown-item>
                    <el-dropdown-item command="delete" divided>删除</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <ServerFormDialog v-model="dialogVisible" :server="editing" @saved="load" />
  </div>
</template>

<style scoped>
.mr-1 { margin-right: 4px; }
.op-bar {
  display: flex;
  align-items: center;
  white-space: nowrap;
}
.op-bar .el-button {
  padding-left: 6px;
  padding-right: 6px;
  margin-left: 0;
}
.more-btn {
  color: #909399;
}
</style>