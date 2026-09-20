<script setup>
import {ref, watch, onMounted} from 'vue'
import {useRoute, useRouter} from 'vue-router'
import Upload from '@/views/files/Upload.vue'
import Execute from '@/views/commands/Execute.vue'

const route = useRoute()
const router = useRouter()

const TABS = [
  {name: 'upload', label: '批量上传', icon: 'Upload'},
  {name: 'execute', label: '批量执行', icon: 'ScaleToOriginal'}
]

const activeTab = ref('upload')
// 首次访问的 tab 才挂载，之后保持挂载以保留页面内的选择与进度状态
const mounted = ref({upload: false, execute: false})

function resolveTab() {
  return route.path.endsWith('/execute') ? 'execute' : 'upload'
}

watch(
  () => route.path,
  () => {
    activeTab.value = resolveTab()
  }
)

watch(activeTab, (v) => {
  mounted.value[v] = true
  const target = `/batch/${v}`
  if (route.path !== target) router.replace(target)
})

onMounted(() => {
  activeTab.value = resolveTab()
  mounted.value[activeTab.value] = true
})
</script>

<template>
  <div class="batch-page">
    <el-tabs v-model="activeTab" class="batch-tabs">
      <el-tab-pane v-for="t in TABS" :key="t.name" :name="t.name">
        <template #label>
          <span class="tab-label">
            <el-icon><component :is="t.icon" /></el-icon>
            {{ t.label }}
          </span>
        </template>
        <!-- 切换 tab 不销毁内容，选中服务器、任务进度等状态保持原样 -->
        <Upload v-if="t.name === 'upload' && mounted.upload" />
        <Execute v-else-if="t.name === 'execute' && mounted.execute" />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped>
/* 内层页面自带 .page-container 内边距，这里不再叠加 */
.batch-page { min-height: 100%; }
.batch-tabs { min-height: 100%; }
.batch-tabs :deep(.el-tabs__header) {
  margin: 0 0 8px;
  padding: 0 16px;
  border-bottom: 1px solid var(--line);
}
.batch-tabs :deep(.el-tabs__nav-wrap::after) { display: none; }
.batch-tabs :deep(.el-tabs__item) {
  height: 42px;
  line-height: 42px;
  font-size: 14px;
  color: var(--ink-2);
}
.batch-tabs :deep(.el-tabs__item.is-active) {
  color: var(--blue-deep);
  font-weight: 600;
}
.tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
</style>