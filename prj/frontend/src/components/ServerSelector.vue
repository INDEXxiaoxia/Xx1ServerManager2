<template>
  <div class="server-selector">
    <div class="selector-header">
      <span class="selector-title">选择服务器</span>
      <el-button size="small" text type="primary" :disabled="!selectedCount" @click="clearSelection">清空选择</el-button>
    </div>
    <div class="selection-info">
      <el-tag type="success" size="small">已选择 {{ selectedCount }} 台</el-tag>
    </div>
    <el-input v-model="filterText" placeholder="搜索服务器..." clearable size="small" class="search-input">
      <template #prefix><el-icon><Search /></el-icon></template>
    </el-input>
    <el-tree
      ref="treeRef"
      :data="treeData"
      :props="treeProps"
      :filter-node-method="filterNode"
      node-key="id"
      show-checkbox
      default-expand-all
      :expand-on-click-node="false"
      class="server-tree"
      @check="handleCheck"
    >
      <template #default="{data}">
        <span class="tree-node">
          <el-icon v-if="data.type === 'group'" class="node-icon"><Folder /></el-icon>
          <el-icon v-else class="node-icon"><Monitor /></el-icon>
          <span class="node-label">{{ data.label }}</span>
          <span v-if="data.type === 'server'" class="server-info">{{ data.host }}</span>
        </span>
      </template>
    </el-tree>
  </div>
</template>

<script setup>
import {ref, computed, watch, onMounted} from 'vue'
import {api} from '@/api'

const emit = defineEmits(['update:selectedServers'])
const filterText = ref('')
const treeRef = ref(null)
const groups = ref([])
const servers = ref([])
const checkedNodes = ref([])

const treeProps = {children: 'children', label: 'label'}

const treeData = computed(() => {
  const data = []
  const svs = servers.value || []
  const grps = groups.value || []
  const ungrouped = svs.filter(s => !s.groupIds || !s.groupIds.length)
  if (ungrouped.length) {
    data.push({id: 'ungrouped', label: '未分组', type: 'group', children: ungrouped.map(toNode)})
  }
  grps.forEach(g => {
    const gs = svs.filter(s => (s.groupIds || []).includes(g.id))
    if (gs.length) data.push({id: g.id, label: `${g.name} (${gs.length})`, type: 'group', children: gs.map(toNode)})
  })
  return data
})

function toNode(s) {
  return {id: s.id, label: s.name, type: 'server', host: s.host, server: s}
}

const selectedCount = computed(() => checkedNodes.value.filter(n => n.type === 'server').length)

watch(filterText, val => treeRef.value?.filter(val))

onMounted(load)

async function load() {
  try {
    servers.value = await api.listServers()
    groups.value = await api.listGroups()
  } catch (e) {
    // 忽略加载失败，页面会自行提示
  }
}

function handleCheck() {
  checkedNodes.value = treeRef.value.getCheckedNodes()
  emit('update:selectedServers', checkedNodes.value.filter(n => n.type === 'server').map(n => n.server))
}

function clearSelection() {
  treeRef.value?.setCheckedKeys([])
  checkedNodes.value = []
  emit('update:selectedServers', [])
}

function filterNode(value, data) {
  if (!value) return true
  const s = value.toLowerCase()
  if (data.type === 'server') {
    return (data.label || '').toLowerCase().includes(s) || (data.host || '').toLowerCase().includes(s)
  }
  return (data.label || '').toLowerCase().includes(s)
}
</script>

<style scoped>
.server-selector {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.selector-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}
.selector-title { font-weight: 600; font-size: 14px; }
.selection-info { margin-bottom: 8px; }
.search-input { margin-bottom: 8px; }
.server-tree {
  flex: 1;
  overflow-y: auto;
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 8px;
  background: var(--panel-2);
}
.tree-node {
  display: flex;
  align-items: center;
  flex: 1;
  min-width: 0;
  padding: 2px 4px;
  border-radius: 8px;
  transition: background-color .2s ease;
}
.tree-node:hover {
  background: var(--fill-color);
}
.node-icon { margin-right: 4px; color: var(--ink-3); }
.node-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.server-info {
  margin-left: 6px;
  font-size: 12px;
  color: var(--ink-3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>