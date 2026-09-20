<script setup>
import {ref, computed, onMounted} from 'vue'
import {api} from '@/api'
import {useBatchStore} from '@/store/batch'
import ServerSelector from '@/components/ServerSelector.vue'
import Upload from '@/views/files/Upload.vue'
import Execute from '@/views/commands/Execute.vue'

const store = useBatchStore()

const servers = ref([])
const loading = ref(false)

async function loadServers() {
  loading.value = true
  try {
    servers.value = await api.listServers()
  } finally {
    loading.value = false
  }
}

// 右侧两个组件共用同一份已选服务器（保存在 store 中，切换 tab / 切页都不丢）
const selectedServers = computed(() =>
  servers.value.filter(s => store.selectedIds.includes(s.id))
)

const TABS = [
  {name: 'upload', label: '批量上传', icon: 'Upload'},
  {name: 'execute', label: '批量执行', icon: 'ScaleToOriginal'}
]

function onChangeTab(name) {
  store.setTab(name)
}

function onServerSelection(list) {
  store.setSelectedIds(list.map(s => s.id))
}

onMounted(async () => {
  await loadServers()
  // 服务器列表可能在别处被修改，选中的 id 若已失效则清理掉
  const valid = store.selectedIds.filter(id => servers.value.some(s => s.id === id))
  if (valid.length !== store.selectedIds.length) store.setSelectedIds(valid)
})
</script>

<template>
  <div class="page-container batch-page">
    <div class="layout-container">
      <!-- 左：服务器选择（两个 tab 共用） -->
      <div class="left-panel">
        <div class="card selector-card">
          <ServerSelector
            :selected-server-ids="store.selectedIds"
            @update:selected-servers="onServerSelection"
          />
        </div>
      </div>

      <!-- 右：批量上传 | 批量执行 -->
      <div class="right-panel">
        <div class="batch-head">
          <el-radio-group :model-value="store.tab" @change="onChangeTab">
            <el-radio-button value="upload">
              <el-icon><Upload /></el-icon>&nbsp;批量上传
            </el-radio-button>
            <el-radio-button value="execute">
              <el-icon><ScaleToOriginal /></el-icon>&nbsp;批量执行
            </el-radio-button>
          </el-radio-group>
          <span class="head-tip">
            已选 <b>{{ selectedServers.length }}</b> 台服务器
            <el-tag v-if="store.uploadRunning || store.executeRunning" type="warning" size="small" class="running-tag">
              有任务执行中
            </el-tag>
          </span>
        </div>

        <!-- 两个组件同时保留：切换 tab 不卸载，任务进度与已填内容都不会丢 -->
        <div v-show="store.tab === 'upload'">
          <Upload :selected-servers="selectedServers" />
        </div>
        <div v-show="store.tab === 'execute'">
          <Execute :selected-servers="selectedServers" />
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
.batch-head {
  display: flex;
  align-items: center;
  gap: 14px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.head-tip {
  font-size: 13px;
  color: var(--ink-2);
}
.head-tip b { color: var(--ink); }
.running-tag { margin-left: 6px; }
</style>