<script setup>
import {ref, computed, onMounted} from 'vue'
import {api} from '@/api'
import {useBatchStore} from '@/store/batch'
import ServerSelector from '@/components/ServerSelector.vue'
// 注意：不能直接叫 Upload / Execute —— 这两个名字与全局注册的 Element Plus 图标
// （@element-plus/icons-vue 的 Upload / Execute）重名，模板标签会解析到图标组件上，
// 导致整个右侧内容渲染不出来。这里统一加 View 后缀避免冲突。
import BatchUpload from '@/views/files/Upload.vue'
import BatchExecute from '@/views/commands/Execute.vue'

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
  store.setSelectedIds((list || []).map(s => s.id))
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
          <!-- 自绘分段控件：不依赖 el-radio-button，样式完全可控 -->
          <div class="seg" role="tablist">
            <button
              v-for="t in TABS"
              :key="t.name"
              type="button"
              class="seg-item"
              :class="{active: store.tab === t.name}"
              role="tab"
              :aria-selected="store.tab === t.name"
              @click="onChangeTab(t.name)"
            >
              <el-icon><component :is="t.icon" /></el-icon>
              <span>{{ t.label }}</span>
            </button>
          </div>
          <span class="head-tip">已选 <b>{{ selectedServers.length }}</b> 台服务器</span>
          <el-tag v-if="store.uploadRunning || store.executeRunning" type="warning" size="small">
            有任务执行中
          </el-tag>
        </div>

        <!-- 两个组件同时保留：切换 tab 不卸载，任务进度与已填内容都不会丢 -->
        <div v-show="store.tab === 'upload'">
          <BatchUpload :selected-servers="selectedServers" />
        </div>
        <div v-show="store.tab === 'execute'">
          <BatchExecute :selected-servers="selectedServers" />
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
  min-height: 40px;
}
/* 分段控件（与预设管理页的 tab 风格一致） */
.seg {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 2px;
  padding: 3px;
  background: var(--fill-color);
  border-radius: 999px;
}
.seg-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: none;
  outline: none;
  cursor: pointer;
  background: transparent;
  padding: 6px 16px;
  border-radius: 999px;
  font-family: inherit;
  font-size: 13px;
  line-height: 1.4;
  color: var(--ink-2);
  white-space: nowrap;
  transition: background-color .25s ease, color .25s ease, box-shadow .25s ease;
}
.seg-item:hover { color: var(--ink); }
.seg-item.active {
  background: var(--card);
  color: var(--blue-deep);
  font-weight: 600;
  box-shadow: var(--card-shadow);
}
.head-tip {
  font-size: 13px;
  color: var(--ink-2);
  white-space: nowrap;
}
.head-tip b { color: var(--ink); }
</style>