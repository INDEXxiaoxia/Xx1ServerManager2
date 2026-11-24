<script setup>
import {ref, computed, watch, onMounted, onBeforeUnmount} from 'vue'
const props = defineProps({session: String, x: Number, y: Number})
const emit = defineEmits(['close','choose'])
const query = ref(''), names = ref([]), index = ref(0), more = ref(false), busy = ref(true), message = ref('')
const input = ref(null)
const start = computed(() => Math.max(0, index.value - 6))
let generation = 0, timer
async function load(token) {
  busy.value = true
  try {
    const offset = names.value.length
    const limit = Math.min(21, 70 - offset)
    const rows = await window.go.main.App.TerminalDirectoryCandidates(props.session, query.value, offset)
    if (token !== generation) return
    names.value.push(...rows.slice(0, limit))
    more.value = rows.length > limit
    if (more.value && names.value.length === 70) message.value = '目录太多了，请使用命令方式进行切换'
  } catch (error) {
    if (token === generation) message.value = String(error)
  } finally { if (token === generation) busy.value = false }
}
watch(query, () => {
  clearTimeout(timer)
  const token = ++generation
  names.value = []; index.value = 0; more.value = false; message.value = ''; busy.value = true
  timer = setTimeout(() => load(token), 150)
}, {flush:'sync'})
function move(step) {
  index.value = Math.max(0, Math.min(names.value.length - 1, index.value + step))
  if (step > 0 && index.value === names.value.length - 1 && more.value && !busy.value && names.value.length < 70) load(generation)
}
function keydown(e) {
  e.stopPropagation()
  if (e.isComposing || e.keyCode === 229) return
  if (e.key === 'Escape') emit('close')
  else if (e.key === 'Enter') { if (names.value[index.value]) emit('choose', names.value[index.value]) }
  else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') move(e.key === 'ArrowDown' ? 1 : -1)
  else if (e.key !== 'Tab') return
  e.preventDefault()
}
onMounted(() => { input.value.focus(); load(generation) })
onBeforeUnmount(() => { generation++; clearTimeout(timer) })
</script>

<template>
  <Teleport to="body">
    <div class="directory-picker" :style="{left:x+'px',top:y+'px'}" @mousedown.stop @click.stop>
      <input ref="input" v-model="query" aria-label="按目录名前缀搜索" placeholder="输入目录名前缀" @keydown="keydown" @blur="emit('close', false)" />
      <div v-for="(name,i) in names.slice(start,start+7)" :key="start+i" class="candidate" :class="{selected:start+i===index}" :title="name" @mousedown.prevent @click="emit('choose',name)">▱　{{ name }}/</div>
      <small v-if="message || busy || !names.length">{{ message || (busy ? '正在读取目录…' : '没有匹配的目录') }}</small>
    </div>
  </Teleport>
</template>

<style scoped>
.directory-picker {position:fixed;z-index:2400;width:min(248px,95vw);box-sizing:border-box;padding:5px;background:var(--card);color:var(--ink);border:1px solid var(--line);border-radius:7px;box-shadow:0 6px 20px #0003;font-size:13px}
input {width:100%;box-sizing:border-box;background:var(--panel-2);color:inherit;border:1px solid var(--line);padding:4px 6px}
.candidate {line-height:24px;padding-inline:6px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;cursor:pointer}
.selected {background:var(--blue);color:white;border-radius:3px}
small {display:block;padding:4px}
</style>
