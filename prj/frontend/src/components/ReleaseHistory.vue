<script setup>
import {computed, ref, watch} from 'vue'
import MarkdownIt from 'markdown-it'
import {BrowserOpenURL} from '../../wailsjs/runtime/runtime'

const props = defineProps({records: Array, version: String})
const expanded = ref('')
watch(() => props.records, records => { expanded.value = records[0]?.Tag || '' }, {immediate:true})

const markdown = new MarkdownIt({html:false, linkify:true})
markdown.disable('image')
const selected = computed(() => props.records.find(item => item.Tag === expanded.value))
const body = computed(() => markdown.render(selected.value?.Body?.trim() || ''))
function openLink(event) {
  const anchor = event.target.closest('a')
  if (!anchor) return
  event.preventDefault()
  const base = 'https://github.com/INDEXxiaoxia/Xx1ServerManager2/'
  try {
    const url = new URL(anchor.getAttribute('href'), base)
    if (['https:', 'http:'].includes(url.protocol)) BrowserOpenURL(url.href)
  } catch {}
}
</script>

<template>
  <section class="history">
    <h3>更新记录</h3>
    <el-collapse v-model="expanded" accordion>
      <el-collapse-item v-for="record in records" :key="record.Tag" :name="record.Tag">
        <template #title>
          <strong>{{ record.Tag }}</strong>
          <el-tag v-if="record.Tag.replace(/^v/i,'') === version" size="small">当前版本</el-tag>
          <time>{{ record.Date.slice(0,10) }}</time>
        </template>
        <template v-if="expanded === record.Tag">
          <article v-if="body" class="notes" tabindex="0" :aria-label="record.Tag + ' 更新记录'" @click="openLink" v-html="body" />
          <p v-else>此版本未附更新说明。</p>
        </template>
      </el-collapse-item>
    </el-collapse>
  </section>
</template>

<style scoped>
.history { margin-top: 16px; }
h3 { font-size: 15px; margin: 0 0 10px; }
.history :deep(.el-collapse-item__header) { gap: 10px; background: transparent; }
.history :deep(.el-collapse-item__wrap) { background: transparent; }
time { margin-left: auto; color: var(--ink-3); font-variant-numeric: tabular-nums; }
.notes { max-height: min(42vh, 420px); overflow: auto; scrollbar-gutter: stable; padding: 12px 18px; border: 1px solid var(--line); border-radius: 8px; color: var(--ink); line-height: 1.7; overflow-wrap: anywhere; }
.notes :deep(:first-child) { margin-top: 0; }
.notes :deep(h1), .notes :deep(h2), .notes :deep(h3) { font-size: 1.1rem; margin-bottom: .4em; }
.notes :deep(ul), .notes :deep(ol) { padding-left: 1.6em; }
.notes :deep(pre) { overflow-x: auto; padding: .7em; background: var(--blue-soft); }
.notes :deep(a) { color: var(--blue-deep); text-decoration: underline; }
.notes :deep(th), .notes :deep(td) { padding: .25em .6em; border: 1px solid var(--line); }
.notes :deep(table) { border-collapse: collapse; }
</style>
