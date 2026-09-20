<script setup>
import {computed} from 'vue'

const props = defineProps({
  loading: {type: Boolean, default: false},
  keyword: {type: String, default: ''},
  scope: {type: String, default: '/'},
  items: {type: Array, default: () => []}
})
const emit = defineEmits(['jump', 'close'])

const count = computed(() => props.items.length)

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
  const kw = (props.keyword || '').trim()
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
  <div class="card mb-12">
    <div class="search-head">
      <span class="search-title">
        搜索结果
        <span class="text-muted">· 范围 {{ scope }} · 关键字「{{ keyword }}」</span>
      </span>
      <el-tag v-if="!loading" size="small" type="info">匹配 {{ count }} 项</el-tag>
      <div class="spacer"></div>
      <el-button text size="small" @click="emit('close')">返回目录</el-button>
    </div>

    <div v-if="loading" class="search-loading">
      <el-icon class="is-loading"><Loading /></el-icon>
      <span>正在搜索 {{ scope }} …</span>
    </div>

    <el-empty v-else-if="!count" description="当前目录下没有匹配的文件名" :image-size="60" />

    <el-table v-else :data="items" stripe size="small" max-height="360">
      <el-table-column label="名称" min-width="260">
        <template #default="{row}">
          <span class="result-name">
            <el-icon class="result-icon"><Folder v-if="row.isDir" /><Document v-else /></el-icon>
            <span v-html="highlight(row.name)"></span>
          </span>
        </template>
      </el-table-column>
      <el-table-column label="类型" width="90">
        <template #default="{row}">
          <el-tag size="small" :type="row.isDir ? 'warning' : 'info'" effect="plain">
            {{ row.isDir ? '文件夹' : '文件' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="大小" width="110">
        <template #default="{row}">
          <span v-if="row.isDir" class="text-muted">—</span>
          <span v-else>{{ formatSize(row.size) }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="modTime" label="修改时间" width="165" />
      <el-table-column label="操作" width="92" fixed="right">
        <template #default="{row}">
          <el-tooltip :content="row.isDir ? '进入该文件夹' : '跳到该文件所在目录'" placement="top">
            <el-button size="small" type="primary" plain @click="emit('jump', row)">跳转</el-button>
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<style scoped>
.search-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}
.search-title { font-weight: 600; font-size: 14px; }
.search-title .text-muted { font-weight: 400; font-size: 12px; }
.spacer { flex: 1; }
.search-loading {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--ink-2);
}
.result-name { display: inline-flex; align-items: center; gap: 6px; }
.result-icon { color: var(--ink-3); }
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
</style>