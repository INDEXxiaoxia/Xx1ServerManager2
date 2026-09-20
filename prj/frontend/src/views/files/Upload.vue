<script setup>
import {ref, onMounted, onBeforeUnmount} from 'vue'
import {ElMessage} from 'element-plus'
import {api, events} from '@/api'
import ServerSelector from '@/components/ServerSelector.vue'

const selectedServers = ref([])
const remoteDir = ref('/opt/myapp/')
const presetPaths = ref([])
const tasks = ref(new Map())
const sessionId = ref('')
const running = ref(false)
const localFiles = ref([])
const groups = ref([])

async function load() {
  presetPaths.value = await api.listPresetPaths()
  groups.value = await api.listGroups()
}
onMounted(load)

function getServerGroupColor(server) {
  if (!server.groupIds || !server.groupIds.length) return 'var(--ink-3)'
  const g = groups.value.find(x => x.id === server.groupIds[0])
  return g?.color || 'var(--ink-3)'
}

async function pickFile() {
  const path = await api.selectLocalFile()
  if (!path) return
  const name = path.split(/[\\/]/).pop()
  if (!localFiles.value.some(x => x.path === path)) {
    localFiles.value.push({path, name})
  }
}

function removeLocalFile(path) {
  localFiles.value = localFiles.value.filter(x => x.path !== path)
}

function taskKey(t) {
  return t.serverId + '|' + t.localPath
}

function onProgress(data) {
  if (sessionId.value && data.sessionId !== sessionId.value) return
  const key = data.taskId || taskKey(data)
  tasks.value.set(key, {...tasks.value.get(key), ...data})
}

function onCompleted(sid) {
  if (sid === sessionId.value) running.value = false
}

async function start() {
  if (!selectedServers.value.length) { ElMessage.warning('请选择服务器'); return }
  if (!localFiles.value.length) { ElMessage.warning('请选择本地文件'); return }
  if (!remoteDir.value) { ElMessage.warning('请输入远程目录'); return }
  running.value = true
  tasks.value = new Map()
  try {
    sessionId.value = await api.startBatchUpload(
      selectedServers.value.map(s => s.id),
      localFiles.value.map(f => f.path),
      remoteDir.value
    )
    ElMessage.success('上传任务已启动')
  } catch (e) {
    ElMessage.error('启动失败：' + e)
    running.value = false
  }
}

function statusType(status) {
  return {running: 'warning', success: 'success', failed: 'danger', waiting: 'info'}[status] || 'info'
}

onMounted(() => {
  events.on('upload:progress', onProgress)
  events.on('upload:completed', onCompleted)
})
onBeforeUnmount(() => {
  events.off('upload:progress')
  events.off('upload:completed')
})
</script>

<template>
  <div class="page-container">
    <div class="layout-container">
      <!-- 左侧：服务器多选 -->
      <div class="left-panel">
        <div class="card selector-card">
          <ServerSelector v-model:selected-servers="selectedServers" />
        </div>
      </div>

      <!-- 右侧：上传操作 -->
      <div class="right-panel">
        <div class="card mb-12">
          <h3 class="card-title">批量文件上传</h3>
          <el-alert v-if="!selectedServers.length" title="请在左侧选择目标服务器" type="warning" show-icon :closable="false" class="mt-12" />
          <template v-else>
            <div class="mt-12">
              <span class="text-muted">目标服务器：</span>
              <el-tag v-for="s in selectedServers" :key="s.id" class="server-tag"
                      :style="{borderColor: getServerGroupColor(s), borderWidth: '2px'}">
                {{ s.name }} ({{ s.host }})
              </el-tag>
            </div>
            <el-form label-width="100px" class="mt-12">
              <el-form-item label="远程目录">
                <el-input v-model="remoteDir" placeholder="/opt/myapp/">
                  <template #append>
                    <el-select v-model="remoteDir" style="width: 140px" placeholder="预设">
                      <el-option v-for="p in presetPaths" :key="p.id" :label="p.name" :value="p.path" />
                    </el-select>
                  </template>
                </el-input>
              </el-form-item>
              <el-form-item label="本地文件">
                <div class="file-wrap">
                  <el-button type="primary" plain @click="pickFile">选择文件</el-button>
                  <el-tag v-for="f in localFiles" :key="f.path" closable class="file-tag" @close="removeLocalFile(f.path)">
                    {{ f.name }}
                  </el-tag>
                </div>
              </el-form-item>
              <el-form-item>
                <el-button type="primary" :loading="running" @click="start">开始上传</el-button>
              </el-form-item>
            </el-form>
          </template>
        </div>

        <div class="card" v-if="tasks.size">
          <h3 class="card-title">上传进度</h3>
          <el-table :data="Array.from(tasks.values())" stripe class="mt-12">
            <el-table-column prop="serverName" label="服务器" width="140" />
            <el-table-column prop="localPath" label="本地文件" min-width="200" show-overflow-tooltip />
            <el-table-column prop="remotePath" label="远程路径" min-width="200" show-overflow-tooltip />
            <el-table-column label="进度" width="220">
              <template #default="{row}">
                <el-progress :percentage="Math.round(row.progress || 0)" :status="row.status === 'failed' ? 'exception' : ''" />
              </template>
            </el-table-column>
            <el-table-column label="状态" width="100">
              <template #default="{row}">
                <el-tag :type="statusType(row.status)" size="small">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="errorMsg" label="错误" min-width="160" show-overflow-tooltip />
          </el-table>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.layout-container {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}
.left-panel {
  width: 320px;
  flex-shrink: 0;
  position: sticky;
  top: 0;
}
.selector-card {
  padding: 12px;
  height: calc(100vh - 160px);
}
.right-panel { flex: 1; min-width: 0; }
.card-title {
  position: relative;
  display: inline-block;
  padding-left: 12px;
  font-size: 16px;
  font-weight: 600;
  margin: 0;
}
.card-title::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 14px;
  border-radius: 999px;
  background: linear-gradient(180deg, var(--blue), var(--pink));
}
.tag-item { margin-right: 8px; margin-bottom: 4px; }
.server-tag {
  margin-right: 8px;
  margin-bottom: 4px;
  border-style: solid;
}
.file-wrap { width: 100%; }
.file-tag { margin: 4px 6px 4px 0; }
</style>