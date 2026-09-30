<script setup>
import {ref, onMounted, computed} from 'vue'
import {useRouter} from 'vue-router'
import {ElMessage, ElMessageBox} from 'element-plus'
import {api} from '@/api'
import ServerFormDialog from '@/components/ServerFormDialog.vue'
import {UNGROUPED, groupIdOf, matchServer, orderedGroups, groupNameOf, groupColorOf} from '@/utils/groups'

const router = useRouter()

const list = ref([])
const groups = ref([])
const loading = ref(false)
const keyword = ref('')
const groupFilter = ref('')
const dialogVisible = ref(false)
const editing = ref(null)
const selected = ref([])

// 分组下拉与总览同一套顺序：有服务器的分组在前，未分组最后
const groupOptions = computed(() => orderedGroups(list.value, groups.value))

const filtered = computed(() => {
  return list.value.filter(r => {
    if (!matchServer(r, keyword.value)) return false
    if (!groupFilter.value) return true
    const id = groupIdOf(r)
    if (groupFilter.value === UNGROUPED) return !id
    return id === groupFilter.value
  })
})

async function load() {
  loading.value = true
  try {
    list.value = await api.listServers()
    groups.value = await api.listGroups()
    // 分组被删除后筛选值可能已失效
    if (groupFilter.value && groupFilter.value !== UNGROUPED && !groups.value.some(g => g.id === groupFilter.value)) {
      groupFilter.value = ''
    }
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

function goMonitor(row) {
  router.push({path: '/monitor', query: {serverId: row.id}})
}
function goTerminal(row) {
  router.push({path: '/terminal', query: {serverId: row.id}})
}
function goFiles(row) {
  router.push({path: '/files', query: {serverId: row.id}})
}
// 运维工具：直达磁盘空间检测并预选该服务器（不自动开始检测）
function goOps(row) {
  router.push({path: '/ops', query: {serverId: row.id, tool: 'disk'}})
}

// 「更多」下拉：测试连接 | 删除
function handleMore(command, row) {
  if (command === 'test') testRow(row)
  else if (command === 'delete') remove(row)
}
</script>

<template>
  <div class="page-container">
    <div class="card">
      <div class="flex-between mb-12">
        <div>
          <el-input v-model="keyword" placeholder="搜索名称/主机" clearable style="width: 220px; margin-right: 8px">
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-select v-model="groupFilter" placeholder="按分组筛选" clearable style="width: 180px">
            <el-option v-for="g in groupOptions" :key="g.id" :label="g.name + ' (' + g.count + ')'" :value="g.id" />
          </el-select>
          <span class="text-muted count-text">显示 {{ filtered.length }} / 共 {{ list.length }} 台</span>
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
            <span v-if="groupIdOf(row)" class="group-cell">
              <i class="dot" :style="{background: groupColorOf(groups, groupIdOf(row))}" />
              {{ groupNameOf(groups, groupIdOf(row)) }}
            </span>
            <span v-else class="text-muted">未分组</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="330" fixed="right">
          <template #default="{row}">
            <div class="op-bar">
              <el-button size="small" text type="primary" @click="goMonitor(row)">监控</el-button>
              <el-button size="small" text type="primary" @click="goTerminal(row)">终端</el-button>
              <el-button size="small" text type="primary" @click="goFiles(row)">文件</el-button>
              <el-button size="small" text type="primary" @click="goOps(row)">监测运维</el-button>
              <el-button size="small" text type="primary" @click="openEdit(row)">编辑</el-button>
              <el-dropdown trigger="click" @command="cmd => handleMore(cmd, row)">
                <el-button size="small" text class="more-btn">
                  更多<el-icon class="el-icon--right"><ArrowDown /></el-icon>
                </el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item command="test">测试连接</el-dropdown-item>
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
.count-text { margin-left: 4px; }
.group-cell {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.group-cell .dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  flex-shrink: 0;
}
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
  color: var(--ink-3);
}
</style>