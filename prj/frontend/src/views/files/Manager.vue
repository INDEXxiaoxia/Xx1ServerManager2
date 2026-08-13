<script setup>
import {ref, onMounted, onBeforeUnmount} from 'vue'
import {useRoute} from 'vue-router'
import {ElMessage, ElMessageBox} from 'element-plus'
import {api, events} from '@/api'

const route = useRoute()
const servers = ref([])
const selectedId = ref('')
const currentPath = ref('/')
const files = ref([])
const loading = ref(false)

const newDirDialog = ref(false)
const newDirName = ref('')
const renameDialog = ref(false)
const renameOld = ref('')
const renameNew = ref('')
const chmodDialog = ref(false)
const chmodTarget = ref('')
const chmodMode = ref('644')

async function loadServers() {
  servers.value = await api.listServers()
  if (route.query.serverId && servers.value.some(s => s.id === route.query.serverId)) {
    selectedId.value = route.query.serverId
  } else if (servers.value.length) {
    selectedId.value = servers.value[0].id
  }
}

async function loadDir() {
  if (!selectedId.value) return
  loading.value = true
  try {
    files.value = await api.listRemoteDir(selectedId.value, currentPath.value)
  } catch (e) {
    ElMessage.error('加载目录失败：' + e)
  } finally {
    loading.value = false
  }
}

async function onChange() {
  currentPath.value = '/'
  await loadDir()
}

function openDir(entry) {
  if (entry.isDir) {
    currentPath.value = entry.path
    loadDir()
  }
}

function goUp() {
  if (currentPath.value === '/') return
  const parts = currentPath.value.split('/').filter(Boolean)
  parts.pop()
  currentPath.value = '/' + parts.join('/')
  loadDir()
}

function formatSize(size) {
  if (size < 1024) return size + ' B'
  if (size < 1024 * 1024) return (size / 1024).toFixed(1) + ' KB'
  if (size < 1024 * 1024 * 1024) return (size / 1024 / 1024).toFixed(1) + ' MB'
  return (size / 1024 / 1024 / 1024).toFixed(2) + ' GB'
}

// 上传
const uploadLoading = ref(false)
async function pickUpload() {
  const localPath = await api.selectLocalFile()
  if (!localPath) return
  const name = localPath.split(/[\\/]/).pop()
  uploadLoading.value = true
  try {
    const remotePath = (currentPath.value === '/' ? '' : currentPath.value) + '/' + name
    await api.uploadToServer(selectedId.value, localPath, remotePath)
    ElMessage.success('上传成功：' + name)
    loadDir()
  } catch (err) {
    ElMessage.error('上传失败：' + err)
  } finally {
    uploadLoading.value = false
  }
}

// 下载
async function download(entry) {
  if (entry.isDir) {
    ElMessage.info('目录下载暂不支持，请使用远程命令打包')
    return
  }
  const localPath = await api.selectLocalSavePath(entry.name)
  if (!localPath) return
  try {
    await api.downloadFromServer(selectedId.value, entry.path, localPath)
    ElMessage.success('下载成功：' + localPath)
  } catch (err) {
    ElMessage.error('下载失败：' + err)
  }
}

// 删除
async function remove(entry) {
  await ElMessageBox.confirm(`确定删除「${entry.path}」吗？`, '提示', {type: 'warning'})
  try {
    await api.deleteRemote(selectedId.value, entry.path, entry.isDir)
    ElMessage.success('已删除')
    loadDir()
  } catch (e) {
    ElMessage.error('删除失败：' + e)
  }
}

// 新建目录
async function createDir() {
  if (!newDirName.value) { ElMessage.warning('请输入目录名'); return }
  const path = (currentPath.value === '/' ? '' : currentPath.value) + '/' + newDirName.value
  try {
    await api.makeRemoteDir(selectedId.value, path)
    ElMessage.success('创建成功')
    newDirDialog.value = false
    newDirName.value = ''
    loadDir()
  } catch (e) {
    ElMessage.error('创建失败：' + e)
  }
}

// 重命名
async function doRename() {
  if (!renameNew.value) { ElMessage.warning('请输入新名称'); return }
  const dir = renameOld.value.substring(0, renameOld.value.lastIndexOf('/'))
  const newPath = (dir || '') + '/' + renameNew.value
  try {
    await api.renameRemote(selectedId.value, renameOld.value, newPath)
    ElMessage.success('重命名成功')
    renameDialog.value = false
    loadDir()
  } catch (e) {
    ElMessage.error('重命名失败：' + e)
  }
}

function openRename(entry) {
  renameOld.value = entry.path
  renameNew.value = entry.name
  renameDialog.value = true
}

// 权限
async function doChmod() {
  try {
    const mode = parseInt(chmodMode.value, 8)
    await api.chmodRemote(selectedId.value, chmodTarget.value, mode)
    ElMessage.success('权限修改成功')
    chmodDialog.value = false
    loadDir()
  } catch (e) {
    ElMessage.error('修改失败：' + e)
  }
}

function openChmod(entry) {
  chmodTarget.value = entry.path
  chmodMode.value = '644'
  chmodDialog.value = true
}

// 进度事件
function onProgress(p) {
  if (p.serverId === selectedId.value && p.progress === 100) {
    loadDir()
  }
}

onMounted(async () => {
  await loadServers()
  await loadDir()
  events.on('file:progress', onProgress)
})
onBeforeUnmount(() => {
  events.off('file:progress')
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
          <el-breadcrumb separator="/" class="path">
            <el-breadcrumb-item v-for="(seg, i) in currentPath.split('/').filter(Boolean)" :key="i">
              {{ seg }}
            </el-breadcrumb-item>
            <el-breadcrumb-item v-if="currentPath === '/'">/</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div>
          <el-button text @click="goUp">上级目录</el-button>
          <el-button @click="newDirDialog = true">新建目录</el-button>
          <el-button type="primary" :loading="uploadLoading" @click="pickUpload">上传文件</el-button>
        </div>
      </div>
    </div>

    <div class="card">
      <el-table :data="files" v-loading="loading" stripe @row-dblclick="openDir">
        <el-table-column label="名称" min-width="260">
          <template #default="{row}">
            <span class="file-name" @click="openDir(row)">
              <el-icon style="margin-right: 6px; vertical-align: middle">
                <Folder v-if="row.isDir" /><Document v-else />
              </el-icon>
              {{ row.name }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="size" label="大小" width="120">
          <template #default="{row}">{{ row.isDir ? '-' : formatSize(row.size) }}</template>
        </el-table-column>
        <el-table-column prop="mode" label="权限" width="120" />
        <el-table-column prop="modTime" label="修改时间" width="160" />
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{row}">
            <el-button size="small" text type="primary" :disabled="row.isDir" @click="download(row)">下载</el-button>
            <el-button size="small" text type="primary" @click="openRename(row)">重命名</el-button>
            <el-button size="small" text @click="openChmod(row)">权限</el-button>
            <el-button size="small" text type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="newDirDialog" title="新建目录" width="380px">
      <el-input v-model="newDirName" placeholder="目录名" @keyup.enter="createDir" />
      <template #footer>
        <el-button @click="newDirDialog = false">取消</el-button>
        <el-button type="primary" @click="createDir">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="renameDialog" title="重命名" width="380px">
      <el-alert :title="renameOld" type="info" :closable="false" class="mb-12" />
      <el-input v-model="renameNew" placeholder="新名称" @keyup.enter="doRename" />
      <template #footer>
        <el-button @click="renameDialog = false">取消</el-button>
        <el-button type="primary" @click="doRename">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="chmodDialog" title="修改权限" width="380px">
      <el-alert :title="chmodTarget" type="info" :closable="false" class="mb-12" />
      <el-input v-model="chmodMode" placeholder="如 644 / 755 / 600" class="mono" />
      <template #footer>
        <el-button @click="chmodDialog = false">取消</el-button>
        <el-button type="primary" @click="doChmod">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.path { margin-left: 4px; }
.file-name { cursor: pointer; }
.file-name:hover { color: #409eff; }
</style>