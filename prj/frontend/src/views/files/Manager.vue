<script setup>
import {ref, reactive, computed, watch, onMounted, onBeforeUnmount} from 'vue'
import {useRoute} from 'vue-router'
import {ElMessage, ElMessageBox} from 'element-plus'
import {api, events} from '@/api'
import FileSearchPanel from '@/components/FileSearchPanel.vue'
import FileEditorDialog from '@/components/FileEditorDialog.vue'

const route = useRoute()

const servers = ref([])
const selectedId = ref('')
const currentPath = ref('/')
const files = ref([])
const loading = ref(false)

// 目录栏
const pathInput = ref('/')
const presetPaths = ref([])
const jumping = ref(false)

// 搜索：仅在当前目录下模糊匹配文件名（需要更大范围时先切到该目录）
const searchKeyword = ref('')
const searchVisible = ref(false)
const searchLoading = ref(false)
const searchResult = ref(null)
let searchTimer = null

const searchScope = computed(() => (searchResult.value && searchResult.value.root) || currentPath.value)

// 排序：prop 为 name/size/modTime，order 为 ascending/descending/null
const sortProp = ref('name')
const sortOrder = ref('ascending')

// 分页
const PAGE_SIZES = [10, 20, 50, 100]
const PAGE_SIZE_KEY = 'xx1.files.pageSize'
function readPageSize() {
  const v = parseInt(localStorage.getItem(PAGE_SIZE_KEY) || '', 10)
  return PAGE_SIZES.includes(v) ? v : 10
}
const pageSize = ref(readPageSize())
const currentPage = ref(1)
watch(pageSize, (v) => {
  localStorage.setItem(PAGE_SIZE_KEY, String(v))
  currentPage.value = 1
})

// 目录大小：key 为目录路径，值为 {status, size, method, error}
const dirSizes = reactive({})
const computingAll = ref(false)
const computeDone = ref(0)
const computeTotal = ref(0)

const newDirDialog = ref(false)
const newDirName = ref('')
const renameDialog = ref(false)
const renameOld = ref('')
const renameNew = ref('')
const chmodDialog = ref(false)
const chmodTarget = ref('')
const chmodMode = ref('644')

// 编辑器
const editorVisible = ref(false)
const editorFile = ref(null)

const breadcrumb = computed(() => {
  const p = currentPath.value
  if (p === '/' || !p) return []
  const segs = p.split('/').filter(Boolean)
  const out = []
  let cur = ''
  for (const s of segs) {
    cur += '/' + s
    out.push({name: s, path: cur})
  }
  return out
})

const dirCount = computed(() => files.value.filter(f => f.isDir).length)

const sortedFiles = computed(() => {
  const list = [...files.value]
  const prop = sortProp.value
  const order = sortOrder.value
  const dir = order === 'descending' ? -1 : 1
  list.sort((a, b) => {
    // 文件夹始终排在文件前面，避免排序后目录被冲散
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
    let r = 0
    if (prop === 'size') r = sizeOf(a) - sizeOf(b)
    else if (prop === 'modTime') r = String(a.modTime || '').localeCompare(String(b.modTime || ''))
    else r = String(a.name || '').localeCompare(String(b.name || ''), 'zh')
    if (r === 0) r = String(a.name || '').localeCompare(String(b.name || ''), 'zh')
    if (!order) return String(a.name || '').localeCompare(String(b.name || ''), 'zh') || (a.isDir ? -1 : 1)
    return r * dir
  })
  return list
})

const total = computed(() => sortedFiles.value.length)
const pagedFiles = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return sortedFiles.value.slice(start, start + pageSize.value)
})

// 页码越界（例如删除文件后当前页超出）时自动回到最后一页
watch([total, pageSize], () => {
  const maxPage = Math.max(1, Math.ceil(total.value / pageSize.value))
  if (currentPage.value > maxPage) currentPage.value = maxPage
})

async function loadServers() {
  servers.value = await api.listServers()
  if (route.query.serverId && servers.value.some(s => s.id === route.query.serverId)) {
    selectedId.value = route.query.serverId
  } else if (servers.value.length) {
    selectedId.value = servers.value[0].id
  }
}

async function loadPresets() {
  try {
    presetPaths.value = await api.listPresetPaths()
  } catch (e) {
    presetPaths.value = []
  }
}

function clearDirSizes() {
  for (const k of Object.keys(dirSizes)) delete dirSizes[k]
  computeDone.value = 0
  computeTotal.value = 0
}

async function loadDir() {
  if (!selectedId.value) return
  loading.value = true
  clearDirSizes()
  try {
    files.value = await api.listRemoteDir(selectedId.value, currentPath.value)
    pathInput.value = currentPath.value
  } catch (e) {
    ElMessage.error('加载目录失败：' + e)
    files.value = []
  } finally {
    loading.value = false
  }
}

function resetListView() {
  currentPage.value = 1
  sortProp.value = 'name'
  sortOrder.value = 'ascending'
}

// 刷新/跳转/切换服务器时清空搜索状态，避免结果与当前目录不一致
function clearSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchKeyword.value = ''
  searchVisible.value = false
  searchResult.value = null
}

// 回到当前目录内容（关闭搜索结果）
function exitSearch() {
  searchVisible.value = false
}

async function onChange() {
  currentPath.value = '/'
  resetListView()
  clearSearch()
  await loadDir()
}

// ---- 目录栏：输入跳转 ----
// 触发方式：输入框内回车，或点击输入框以外的地方（失焦）
async function jumpTo(target) {
  const raw = (target ?? pathInput.value ?? '').trim()
  if (!selectedId.value) {
    ElMessage.warning('请先选择服务器')
    return
  }
  if (!raw) {
    ElMessage.warning('请输入目录路径')
    return
  }
  jumping.value = true
  try {
    // 先解析（支持 ~ 与相对路径），再校验是否为可访问目录
    const abs = await api.resolveRemotePath(selectedId.value, raw)
    const check = await api.validateRemoteDir(selectedId.value, abs)
    if (!check || !check.ok) {
      throw new Error('目录校验失败')
    }
    currentPath.value = check.path
    pathInput.value = check.path
    resetListView()
    clearSearch()
    if (check.isSymlink && check.message) ElMessage.info(check.message)
    await loadDir()
  } catch (e) {
    // 跳转失败时保留当前目录与列表，只把输入框回滚，避免用户误以为已切换
    ElMessage.error(String(e))
    pathInput.value = currentPath.value
  } finally {
    jumping.value = false
  }
}

// 输入框回车 / 失焦统一入口：内容与当前目录相同则不重复请求
function commitPath() {
  if (jumping.value || loading.value) return
  const v = (pathInput.value || '').trim()
  if (v === currentPath.value) return
  jumpTo(v)
}

function goUp() {
  if (currentPath.value === '/') {
    ElMessage.info('已经是根目录')
    return
  }
  const parts = currentPath.value.split('/').filter(Boolean)
  parts.pop()
  jumpTo('/' + parts.join('/'))
}

function openDir(entry) {
  if (entry.isDir) jumpTo(entry.path)
}

// ---- 搜索：仅当前目录 ----
function onSearchInput() {
  if (searchTimer) clearTimeout(searchTimer)
  const kw = searchKeyword.value.trim()
  if (!kw) {
    searchVisible.value = false
    searchResult.value = null
    return
  }
  // 轻微防抖：连续输入时只发最后一次请求，回车会立即触发
  searchTimer = setTimeout(() => runSearch(), 400)
}

async function runSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  const kw = searchKeyword.value.trim()
  if (!selectedId.value) {
    ElMessage.warning('请先选择服务器')
    return
  }
  if (!kw) {
    searchVisible.value = false
    searchResult.value = null
    return
  }
  searchLoading.value = true
  searchVisible.value = true
  try {
    // recursive=false：只在当前目录这一层匹配，范围由用户切换目录决定
    searchResult.value = await api.searchRemoteDir(selectedId.value, currentPath.value, kw, false)
  } catch (e) {
    ElMessage.error('搜索失败：' + e)
    searchResult.value = null
  } finally {
    searchLoading.value = false
  }
}

function onSearchJump(item) {
  const dir = item.isDir ? item.path : dirOf(item.path)
  clearSearch()
  jumpTo(dir)
}

function dirOf(p) {
  if (!p) return '/'
  const idx = p.lastIndexOf('/')
  return idx <= 0 ? '/' : p.substring(0, idx)
}

// ---- 排序 ----
function onSortChange({prop, order}) {
  sortProp.value = prop || 'name'
  sortOrder.value = order || 'ascending'
  currentPage.value = 1
  // 按大小排序需要目录大小数据，否则目录一律按 0 参与比较会得到误导性顺序
  if (prop === 'size' && order && dirCount.value > 0 && !allDirSizesKnown()) {
    ElMessage.info('正在计算目录大小以便按大小排序…')
    computeAllSizes()
  }
}

function allDirSizesKnown() {
  return files.value.every(f => !f.isDir || (dirSizes[f.path] && dirSizes[f.path].status === 'done'))
}

function sizeOf(entry) {
  if (!entry.isDir) return entry.size || 0
  const s = dirSizes[entry.path]
  return s && s.status === 'done' ? s.size : 0
}

// ---- 目录大小计算 ----
function markSizeLoading(paths) {
  for (const p of paths) dirSizes[p] = {status: 'loading'}
}

async function computeSizes(paths) {
  if (!selectedId.value || !paths.length) return
  markSizeLoading(paths)
  try {
    const items = await api.getRemoteDirSizes(selectedId.value, paths)
    for (const it of items) {
      dirSizes[it.path] = it.error
        ? {status: 'error', error: it.error}
        : {status: 'done', size: it.size, method: it.method}
    }
    // 后端可能因上限截断，未返回的条目标记为错误而不是永久转圈
    for (const p of paths) {
      if (!dirSizes[p] || dirSizes[p].status === 'loading') {
        dirSizes[p] = {status: 'error', error: '未返回结果（可能超出单次计算上限）'}
      }
    }
  } catch (e) {
    for (const p of paths) {
      if (dirSizes[p] && dirSizes[p].status === 'loading') dirSizes[p] = {status: 'error', error: String(e)}
    }
    ElMessage.error('目录大小计算失败：' + e)
  }
}

async function computeOne(entry) {
  await computeSizes([entry.path])
}

async function computeAllSizes() {
  if (computingAll.value) return
  const targets = files.value.filter(f => f.isDir).map(f => f.path)
  if (!targets.length) {
    ElMessage.info('当前目录没有子目录')
    return
  }
  if (targets.length > 500) {
    ElMessage.warning(`子目录数量较多（${targets.length} 个），将分批计算，可能需要一些时间`)
  }
  computingAll.value = true
  computeDone.value = 0
  computeTotal.value = targets.length
  try {
    // 分成 50 个一批，逐批请求，避免一次请求过长导致界面无反馈
    const BATCH = 50
    for (let i = 0; i < targets.length; i += BATCH) {
      await computeSizes(targets.slice(i, i + BATCH))
      computeDone.value = Math.min(i + BATCH, targets.length)
    }
    const failed = targets.filter(p => dirSizes[p] && dirSizes[p].status === 'error').length
    if (failed) ElMessage.warning(`计算完成，其中 ${failed} 个目录失败（多为权限不足）`)
    else ElMessage.success(`已计算 ${targets.length} 个目录大小`)
  } finally {
    computingAll.value = false
  }
}

function formatSize(size) {
  if (size === null || size === undefined) return '-'
  if (size < 1024) return size + ' B'
  if (size < 1024 * 1024) return (size / 1024).toFixed(1) + ' KB'
  if (size < 1024 * 1024 * 1024) return (size / 1024 / 1024).toFixed(1) + ' MB'
  if (size < 1024 * 1024 * 1024 * 1024) return (size / 1024 / 1024 / 1024).toFixed(2) + ' GB'
  return (size / 1024 / 1024 / 1024 / 1024).toFixed(2) + ' TB'
}

function sizeMethodLabel(method) {
  return method === 'walk' ? 'SFTP 遍历统计' : '远端 du 统计'
}

// ---- 上传 / 下载 ----
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

// ---- 删除 / 新建 / 重命名 / 权限 ----
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

async function createDir() {
  if (!newDirName.value) {
    ElMessage.warning('请输入目录名')
    return
  }
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

function openRename(entry) {
  renameOld.value = entry.path
  renameNew.value = entry.name
  renameDialog.value = true
}

async function doRename() {
  if (!renameNew.value) {
    ElMessage.warning('请输入新名称')
    return
  }
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

function openChmod(entry) {
  chmodTarget.value = entry.path
  // 用远端返回的权限位作为默认值，避免把 755 的脚本默认改成 644
  const m = String(entry.mode || '').slice(-9)
  chmodMode.value = m ? modeToOctal(m) : '644'
  chmodDialog.value = true
}

function modeToOctal(symbolic) {
  let out = ''
  for (let i = 0; i < 9; i += 3) {
    const part = symbolic.slice(i, i + 3)
    let n = 0
    if (part[0] === 'r') n += 4
    if (part[1] === 'w') n += 2
    if (part[2] === 'x' || part[2] === 's' || part[2] === 't') n += 1
    out += String(n)
  }
  return out
}

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

// ---- 在线编辑 ----
async function openEditor(entry) {
  if (entry.isDir) return
  editorFile.value = entry
  editorVisible.value = true
}

async function onEdited() {
  editorVisible.value = false
  await loadDir()
}

function copyPath(entry) {
  const p = entry.path
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(p).then(
      () => ElMessage.success('已复制路径'),
      () => ElMessage.warning('复制失败，请手动选择路径')
    )
  } else {
    ElMessage.warning('当前环境不支持剪贴板')
  }
}

// ---- 进度事件 ----
function onProgress(p) {
  if (p.serverId === selectedId.value && p.progress === 100) {
    loadDir()
  }
}

function onDirSizeEvent(d) {
  if (d.serverId !== selectedId.value) return
  if (!(d.path in dirSizes)) return // 只更新当前目录已发起的行
  if (d.error) {
    dirSizes[d.path] = {status: 'error', error: d.error}
  } else {
    dirSizes[d.path] = {status: 'done', size: d.size, method: d.method}
  }
  if (d.total) {
    computeTotal.value = d.total
    if (d.done > computeDone.value) computeDone.value = d.done
  }
}

watch(selectedId, () => {
  onChange()
})

onMounted(async () => {
  await loadServers()
  await loadPresets()
  await loadDir()
  events.on('file:progress', onProgress)
  events.on('file:dirsize', onDirSizeEvent)
})

onBeforeUnmount(() => {
  events.off('file:progress', onProgress)
  events.off('file:dirsize', onDirSizeEvent)
})
</script>

<template>
  <div class="page-container">
    <!-- 功能条：服务器 / 路径 / 上级 -->
    <div class="top-bar">
      <el-select v-model="selectedId" placeholder="选择服务器" class="server-select">
        <el-option v-for="s in servers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
      </el-select>

      <el-input
        v-model="pathInput"
        class="path-input mono"
        placeholder="输入目录路径后回车跳转，如 /etc 或 ~/logs"
        clearable
        @keyup.enter="commitPath"
        @blur="commitPath"
        @clear="commitPath"
      >
        <template #prefix><el-icon><FolderOpened /></el-icon></template>
      </el-input>

      <el-tooltip content="返回上级目录" placement="bottom">
        <el-button
          class="up-btn"
          :disabled="currentPath === '/'"
          :loading="jumping"
          @click="goUp"
        >
          <el-icon><Top /></el-icon>
        </el-button>
      </el-tooltip>
    </div>

    <!-- 当前位置（面包屑，可直接点击切换） -->
    <div class="crumb-bar">
      <span class="crumb-label">当前位置</span>
      <el-breadcrumb separator="/" class="crumb-path">
        <!-- 根目录本身就以 / 表达，不要再作为独立 item 与分隔符叠加成 // -->
        <el-breadcrumb-item>
          <span class="crumb-root" :class="{current: currentPath === '/'}" @click="jumpTo('/')">/</span>
        </el-breadcrumb-item>
        <el-breadcrumb-item v-for="c in breadcrumb" :key="c.path">
          <span class="crumb-link" @click="jumpTo(c.path)">{{ c.name }}</span>
        </el-breadcrumb-item>
      </el-breadcrumb>
      <span v-if="presetPaths.length" class="crumb-presets">
        <el-button v-for="p in presetPaths.slice(0, 6)" :key="p.id" text size="small" @click="jumpTo(p.path)">
          {{ p.name }}
        </el-button>
      </span>
      <div class="spacer"></div>
      <el-button text size="small" :loading="loading" @click="loadDir">
        <el-icon><Refresh /></el-icon>&nbsp;刷新
      </el-button>
    </div>

    <!-- 搜索结果（仅当前目录） -->
    <FileSearchPanel
      v-if="searchVisible"
      :loading="searchLoading"
      :keyword="searchKeyword"
      :scope="searchScope"
      :items="(searchResult && searchResult.items) || []"
      @jump="onSearchJump"
      @close="exitSearch"
    />

    <!-- 目录列表 -->
    <div class="card">
      <div class="list-head">
        <span class="list-meta">
          共 <b>{{ total }}</b> 项{{ dirCount ? `（${dirCount} 个子目录）` : '' }} · 当前第 {{ currentPage }} 页
        </span>
        <div class="spacer"></div>
        <el-input
          v-model="searchKeyword"
          class="search-input"
          placeholder="在当前目录搜索文件名，回车搜索"
          clearable
          @keyup.enter="runSearch"
          @input="onSearchInput"
          @clear="clearSearch"
        >
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <span v-if="computingAll" class="compute-progress">
          <el-icon class="is-loading"><Loading /></el-icon>
          计算中 {{ computeDone }}/{{ computeTotal }}
        </span>
        <el-button size="small" :disabled="!dirCount || computingAll" :loading="computingAll" @click="computeAllSizes">
          计算全部
        </el-button>
        <el-button size="small" @click="newDirDialog = true">新建目录</el-button>
        <el-button size="small" type="primary" :loading="uploadLoading" @click="pickUpload">上传文件</el-button>
      </div>

      <el-table
        :data="pagedFiles"
        v-loading="loading"
        stripe
        @row-dblclick="openDir"
        @sort-change="onSortChange"
      >
        <el-table-column label="名称" prop="name" min-width="240" sortable="custom">
          <template #default="{row}">
            <span class="file-name" @click="openDir(row)">
              <el-icon style="margin-right: 6px; vertical-align: middle">
                <Folder v-if="row.isDir" /><Document v-else />
              </el-icon>
              {{ row.name }}
            </span>
          </template>
        </el-table-column>

        <el-table-column label="大小" prop="size" width="150" sortable="custom">
          <template #default="{row}">
            <!-- 文件：直接显示大小 -->
            <span v-if="!row.isDir">{{ formatSize(row.size) }}</span>
            <!-- 目录：未计算 → 点击计算；计算中 → 转圈；完成 → 显示大小；失败 → 可重试 -->
            <template v-else>
              <span v-if="!dirSizes[row.path]" class="size-action" @click="computeOne(row)">计算</span>
              <span v-else-if="dirSizes[row.path].status === 'loading'" class="size-action loading">
                <el-icon class="is-loading"><Loading /></el-icon>
                <span>计算中</span>
              </span>
              <el-tooltip
                v-else-if="dirSizes[row.path].status === 'done'"
                :content="`${sizeMethodLabel(dirSizes[row.path].method)} · 点击重新计算`"
                placement="top"
              >
                <span class="size-action done" @click="computeOne(row)">{{ formatSize(dirSizes[row.path].size) }}</span>
              </el-tooltip>
              <el-tooltip v-else :content="dirSizes[row.path].error" placement="top">
                <span class="size-action error" @click="computeOne(row)">计算失败</span>
              </el-tooltip>
            </template>
          </template>
        </el-table-column>

        <el-table-column prop="mode" label="权限" width="110" />
        <el-table-column prop="modTime" label="修改时间" width="165" sortable="custom" />

        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{row}">
            <!-- 目录没有下载/编辑语义，只保留重命名、权限、复制路径、删除 -->
            <template v-if="!row.isDir">
              <el-button size="small" text type="primary" @click="openEditor(row)">编辑</el-button>
              <el-button size="small" text type="primary" @click="download(row)">下载</el-button>
            </template>
            <el-button size="small" text type="primary" @click="openRename(row)">重命名</el-button>
            <el-button size="small" text @click="openChmod(row)">权限</el-button>
            <el-button size="small" text @click="copyPath(row)">复制路径</el-button>
            <el-button size="small" text type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>

        <template #empty>
          <el-empty :description="loading ? '加载中…' : '当前目录为空'" :image-size="70" />
        </template>
      </el-table>

      <div class="pager">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="PAGE_SIZES"
          :total="total"
          layout="total, sizes, prev, pager, next, jumper"
          background
        />
      </div>
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

    <FileEditorDialog
      v-model="editorVisible"
      :server-id="selectedId"
      :file="editorFile"
      @saved="onEdited"
    />
  </div>
</template>

<style scoped>
/* 功能条：服务器 + 路径输入 + 上级图标按钮，单行紧凑排布 */
.top-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}
.server-select { width: 220px; flex: 0 0 220px; }
.path-input { flex: 1; min-width: 240px; }
.up-btn { flex: 0 0 auto; }

.crumb-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  padding: 0 2px;
  font-size: 12px;
  color: var(--ink-2);
  min-height: 24px;
}
.crumb-label { color: var(--ink-3); }
.crumb-presets { display: inline-flex; align-items: center; }
.spacer { flex: 1; }
.crumb-path { display: inline-flex; align-items: center; }
.crumb-root {
  cursor: pointer;
  padding: 0 4px;
  border-radius: 6px;
  background: var(--fill-color, rgba(127, 127, 127, .12));
  transition: color .2s ease;
}
.crumb-root:hover { color: var(--blue); }
/* 根目录时后面没有片段，只有它自身，弱化底纹避免看起来像可点的“上一级” */
.crumb-root.current { background: transparent; padding: 0 2px; }
.crumb-link { cursor: pointer; transition: color .2s ease; }
.crumb-link:hover { color: var(--blue); }

.file-name { cursor: pointer; transition: color .2s ease; }
.file-name:hover { color: var(--blue); }

/* 标题栏：统计 + 搜索 + 操作按钮 */
.list-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.list-meta { font-size: 13px; color: var(--ink-2); }
.list-meta b { color: var(--ink); font-size: 15px; }
.search-input { width: 300px; }
.compute-progress {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--blue);
}

.size-action {
  cursor: pointer;
  color: var(--blue);
  border-bottom: 1px dashed var(--blue);
  transition: opacity .2s ease;
}
.size-action:hover { opacity: .75; }
.size-action.loading,
.size-action.error {
  border-bottom: none;
  cursor: pointer;
}
.size-action.loading {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--ink-3);
  cursor: default;
}
.size-action.done { color: var(--ink); border-bottom-color: var(--line); }
.size-action.error { color: var(--red); }
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 14px;
}
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
</style>