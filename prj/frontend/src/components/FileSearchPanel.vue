<script setup>
import {ref, watch, computed} from 'vue'
import {ElMessage} from 'element-plus'
import {api} from '@/api'

const props = defineProps({
  modelValue: {type: Boolean, default: false},
  serverId: {type: String, default: ''},
  currentPath: {type: String, default: '/'}
})
const emit = defineEmits(['update:modelValue', 'jump'])

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const keyword = ref('')
const recursive = ref(false) // 仅当前目录(默认) / 整个目录树
const loading = ref(false)
const result = ref(null)
const searching = ref(false)

let timer = null
watch(keyword, () => {
  if (timer) clearTimeout(timer)
  const kw = keyword.value.trim()
  if (!kw) {
    result.value = null
    return
  }
  // 防抖：避免每敲一个字符就打一次远端
  timer = setTimeout(() => run(), 400)
})

// 服务器或当前目录变化时清空旧结果，避免串目录
watch([() => props.serverId, () => props.currentPath], () => {
  if (!keyword.value) result.value = null
})

watch(visible, (v) => {
  if (!v) {
    keyword.value = ''
    result.value = null
  }
})

// 整站搜索：以根目录为起点递归（后端有深度/条数/耗时上限保护）
const searchRoot = computed(() => (recursive.value ? '/' : props.currentPath || '/'))

async function run() {
  const kw = keyword.value.trim()
  if (!kw) {
    ElMessage.warning('请输入搜索关键字')
    return
  }
  if (!props.serverId) {
    ElMessage.warning('请先选择服务器')
    return
  }
  searching.value = true
  try {
    result.value = await api.searchRemoteDir(props.serverId, searchRoot.value, kw, recursive.value)
  } catch (e) {
    ElMessage.error('搜索失败：' + e)
    result.value = null
  } finally {
    searching.value = false
  }
}

function parentOf(p) {
  if (!p) return '/'
  const idx = p.lastIndexOf('/')
  if (idx <= 0) return '/'
  return p.substring(0, idx)
}

function dirOf(item) {
  return parentOf(item.path)
}

function jump(item) {
  const dir = item.isDir ? item.path : dirOf(item)
  emit('jump', dir)
}

function formatSize(size) {
  if (size === null || size === undefined) return '-'
  if (size < 1024) return size + ' B'
  if (size < 1024 * 1024) return (size / 1024).toFixed(1) + ' KB'
  if (size < 1024 * 1024 * 1024) return (size / 1024 / 1024).toFixed(1) + ' MB'
  return (size / 1024 / 1024 / 1024).toFixed(2) + ' GB'
}

// 关键字高亮：先做 HTML 转义再插入 <mark>，避免文件名中的特殊字符被当成标签渲染
function escapeHtml(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function highlight(name) {
  const safe = escapeHtml(name)
  const kw = keyword.value.trim()
  if (!kw) return safe
  // 通配符/字符类不是字面量，只高亮其中的纯文本部分
  const plain = kw.replace(/[*?[\]]/g, '')
  if (!plain) return safe
  const idx = name.toLowerCase().indexOf(plain.toLowerCase())
  if (idx < 0) return safe
  return (
    safe.slice(0, idx) +
    '<mark>' + safe.slice(idx, idx + plain.length) + '</mark>' +
    safe.slice(idx + plain.length)
  )
}
</script>

<template>
  <div class="card mb-12 search-panel">
    <div class="search-head">
      <h3 class="card-title">文件搜索</h3>
      <el-button text size="small" @click="visible = false">收起</el-button>
    </div>
    <div class="search-controls">
      <el-input
        v-model="keyword"
        placeholder="在当前目录下模糊搜索文件名，支持 * 与 ? 通配符"
        clearable
        style="max-width: 420px"
        @keyup.enter="run"
      >
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <el-select v-model="recursive" style="width: 210px">
        <el-option :label="`当前目录及子目录（${currentPath}）`" :value="false" />
        <el-option label="整站搜索（从 / 递归）" :value="true" />
      </el-select>
      <el-button type="primary" :loading="searching" @click="run">搜索</el-button>
      <span class="text-muted search-tip">搜索范围：{{ searchRoot }}</span>
    </div>

    <div v-if="searching" class="search-loading">
      <el-icon class="is-loading"><Loading /></el-icon>
      <span>正在扫描 {{ searchRoot }} …</span>
    </div>

    <template v-else-if="result">
      <el-alert
        v-if="result.truncated"
        type="warning"
        show-icon
        :closable="false"
        class="mt-12"
        title="结果过多已截断"
        description="仅显示前 500 条匹配项，请缩小搜索范围或使用更精确的关键字。"
      />
      <div class="search-summary">
        在 <span class="mono">{{ result.root }}</span> 下扫描 {{ result.scanned }} 个条目，
        匹配到 <b>{{ (result.items || []).length }}</b> 个（{{ result.elapsedMs }} ms）
      </div>

      <el-empty v-if="!result.items || !result.items.length" description="未找到匹配项" :image-size="60" />

      <el-table v-else :data="result.items" stripe size="small" max-height="360">
        <el-table-column label="名称" min-width="240">
          <template #default="{row}">
            <span class="result-name">
              <el-icon class="result-icon"><Folder v-if="row.isDir" /><Document v-else /></el-icon>
              <span v-html="highlight(row.name)"></span>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="所在目录" min-width="240" show-overflow-tooltip>
          <template #default="{row}">
            <span class="mono text-muted">{{ dirOf(row) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="大小" width="110">
          <template #default="{row}">
            <span v-if="row.isDir" class="text-muted">目录</span>
            <span v-else>{{ formatSize(row.size) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="modTime" label="修改时间" width="160" />
        <el-table-column label="操作" width="92" fixed="right">
          <template #default="{row}">
            <el-button size="small" type="primary" plain @click="jump(row)">跳转</el-button>
          </template>
        </el-table-column>
      </el-table>
    </template>
  </div>
</template>

<style scoped>
.search-panel { transition: box-shadow .2s ease; }
.search-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.search-controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-top: 12px;
}
.search-tip { font-size: 12px; }
.search-loading {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
  font-size: 13px;
  color: var(--ink-2);
}
.search-summary {
  margin: 12px 0 8px;
  font-size: 12px;
  color: var(--ink-2);
}
.result-name { display: inline-flex; align-items: center; gap: 6px; }
.result-icon { color: var(--ink-3); }
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
</style>