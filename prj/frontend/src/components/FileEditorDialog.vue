<script setup>
import {ref, computed, watch, nextTick} from 'vue'
import {ElMessage, ElMessageBox} from 'element-plus'
import {api} from '@/api'

const props = defineProps({
  modelValue: {type: Boolean, default: false},
  serverId: {type: String, default: ''},
  file: {type: Object, default: null}
})
const emit = defineEmits(['update:modelValue', 'saved'])

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const loading = ref(false)
const saving = ref(false)
const meta = ref(null)
const text = ref('')
const original = ref('')
const encoding = ref('utf-8')
const keepBOM = ref(false)
const removeBOM = ref(false) // keepBOM 的界面反向开关：打开表示“保存时移除 BOM”
const eol = ref('lf')
const textareaRef = ref(null)

const ENCODINGS = [
  {label: 'UTF-8', value: 'utf-8'},
  {label: 'GB18030（兼容 GBK/GB2312）', value: 'gb18030'},
  {label: 'GBK', value: 'gbk'},
  {label: 'GB2312', value: 'gb2312'}
]

const encodingLabel = computed(() => (ENCODINGS.find(e => e.value === encoding.value) || {}).label || encoding.value)
const eolLabel = computed(() => ({lf: 'LF', crlf: 'CRLF', cr: 'CR'}[eol.value] || eol.value))
const dirty = computed(() => text.value !== original.value || (meta.value && removeBOM.value !== meta.value.hasBOM))
const lines = computed(() => text.value.split('\n'))
const lineNumbers = computed(() => {
  const n = lines.value.length
  if (n > 4000) return ''
  return Array.from({length: n}, (_, i) => i + 1).join('\n')
})
const byteHint = computed(() => {
  const n = new TextEncoder().encode(text.value).length
  return n < 1024 ? `${n} B` : `${(n / 1024).toFixed(1)} KB`
})

// 按 base64 解码，避免非 ASCII 编码内容在传输中被替换
function decodeBase64(b64, enc) {
  if (!b64) return ''
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  const label = enc === 'gbk' || enc === 'gb2312' ? 'gbk' : enc
  try {
    return new TextDecoder(label, {fatal: false}).decode(bytes)
  } catch (e) {
    return new TextDecoder('utf-8', {fatal: false}).decode(bytes)
  }
}

watch(visible, async (v) => {
  if (v) await load()
})

async function load() {
  if (!props.serverId || !props.file) return
  loading.value = true
  meta.value = null
  text.value = ''
  original.value = ''
  try {
    const info = await api.detectRemoteEncoding(props.serverId, props.file.path)
    meta.value = info
    encoding.value = info.encoding
    keepBOM.value = !!info.hasBOM
    removeBOM.value = false
    eol.value = info.eol || 'lf'
    if (!info.editable) {
      ElMessage.warning(info.message || '该文件不支持在线编辑')
      visible.value = false
      return
    }
    const content = await api.readRemoteTextFile(props.serverId, props.file.path, info.encoding)
    const decoded = decodeBase64(content.base64, content.encoding || info.encoding)
    // 统一为 LF 展示（保存时按原换行风格写回）
    text.value = decoded.replace(/\r\n/g, '\n').replace(/\r/g, '\n')
    original.value = text.value
    encoding.value = content.encoding || info.encoding
    keepBOM.value = !!content.hasBOM
    removeBOM.value = false
    eol.value = content.eol || info.eol || 'lf'
    await nextTick()
  } catch (e) {
    ElMessage.error('打开文件失败：' + e)
    visible.value = false
  } finally {
    loading.value = false
  }
}

async function changeEncoding(v) {
  if (!meta.value || v === meta.value.encoding) return
  try {
    await ElMessageBox.confirm(
      `编码将从「${meta.value.encoding}」改为「${v}」，保存时会按新编码重写文件字节。确定继续吗？`,
      '编码变更', {type: 'warning'}
    )
  } catch (e) {
    encoding.value = meta.value.encoding
  }
}

async function save() {
  if (saving.value) return
  saving.value = true
  // removeBOM 为真时强制不写 BOM；否则保持原状
  const wantBOM = removeBOM.value ? false : keepBOM.value
  try {
    await api.writeRemoteTextFile(props.serverId, props.file.path, {
      text: text.value,
      encoding: encoding.value,
      keepBOM: wantBOM,
      eol: eol.value
    })
    original.value = text.value
    keepBOM.value = wantBOM
    if (meta.value) meta.value.hasBOM = wantBOM
    removeBOM.value = false
    ElMessage.success('保存成功')
    emit('saved', props.file.path)
  } catch (e) {
    ElMessage.error('保存失败：' + e)
  } finally {
    saving.value = false
  }
}

async function close() {
  if (dirty.value) {
    try {
      await ElMessageBox.confirm('当前修改尚未保存，确定关闭吗？', '提示', {type: 'warning'})
    } catch (e) {
      return
    }
  }
  visible.value = false
}

async function reset() {
  try {
    await ElMessageBox.confirm('放弃当前修改并恢复为文件原始内容？', '提示', {type: 'warning'})
  } catch (e) {
    return
  }
  text.value = original.value
}

function onKeydown(e) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
    e.preventDefault()
    save()
  }
}

// 滚动行号栏与文本区同步
function onScroll(e) {
  const el = document.querySelector('.editor-gutter')
  if (el) el.scrollTop = e.target.scrollTop
}
</script>

<template>
  <el-dialog
    v-model="visible"
    title="编辑远程文件"
    width="900px"
    top="5vh"
    :close-on-click-modal="false"
    class="file-editor-dialog"
    @close="() => {}"
  >
    <div v-loading="loading" class="editor-body">
      <div class="editor-path mono">{{ file?.path }}</div>

      <el-alert
        v-if="meta && meta.hasBOM"
        type="warning"
        show-icon
        :closable="false"
        class="mb-12"
        title="该文件带有 UTF-8 BOM（Windows 记事本常会添加）"
        description="默认保存时保留 BOM；如需生成标准 Unix 文件，请打开下方「保存时移除 BOM」。"
      />
      <el-alert
        v-if="meta && meta.eolMixed"
        type="info"
        show-icon
        :closable="false"
        class="mb-12"
        title="该文件混用了多种换行符"
        description="保存时会统一为下方选择的换行风格。"
      />

      <div class="editor-toolbar">
        <div class="tool-item">
          <span class="tool-label">编码</span>
          <el-select v-model="encoding" size="small" style="width: 220px" @change="changeEncoding">
            <el-option v-for="e in ENCODINGS" :key="e.value" :label="e.label" :value="e.value" />
          </el-select>
        </div>
        <div class="tool-item">
          <span class="tool-label">换行</span>
          <el-select v-model="eol" size="small" style="width: 110px">
            <el-option label="LF (Unix)" value="lf" />
            <el-option label="CRLF (Windows)" value="crlf" />
            <el-option label="CR (旧 Mac)" value="cr" />
          </el-select>
        </div>
        <div class="tool-item">
          <span class="tool-label">保存时移除 BOM</span>
          <el-switch v-model="removeBOM" size="small" />
        </div>
      </div>

      <div class="editor-wrap">
        <pre class="editor-gutter mono">{{ lineNumbers }}</pre>
        <textarea
          ref="textareaRef"
          v-model="text"
          class="editor-area mono"
          spellcheck="false"
          wrap="off"
          @keydown="onKeydown"
          @scroll="onScroll"
        ></textarea>
      </div>

      <div class="editor-status">
        <span>{{ lines.length }} 行</span>
        <span>{{ text.length }} 字符 · {{ byteHint }}</span>
        <span>编码 {{ encodingLabel }}</span>
        <span>换行 {{ eolLabel }}</span>
        <span v-if="dirty" class="status-dirty">● 未保存</span>
        <span v-else class="status-saved">已与远端一致</span>
      </div>
    </div>

    <template #footer>
      <div class="editor-footer">
        <el-button text @click="reset" :disabled="loading || !dirty">还原</el-button>
        <div>
          <el-button @click="close">取消</el-button>
          <el-button type="primary" :loading="saving" :disabled="loading" @click="save">
            保存 (Ctrl+S)
          </el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.editor-body { min-height: 320px; }
.editor-path {
  font-size: 12px;
  color: var(--ink-3);
  margin-bottom: 10px;
  word-break: break-all;
}
.editor-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 18px;
  margin-bottom: 10px;
}
.tool-item { display: flex; align-items: center; gap: 8px; }
.tool-label { font-size: 12px; color: var(--ink-2); }
.editor-wrap {
  display: flex;
  border: 1px solid var(--line);
  border-radius: 12px;
  overflow: hidden;
  background: var(--panel-2);
}
.editor-gutter {
  margin: 0;
  padding: 10px 8px;
  width: 52px;
  flex: 0 0 52px;
  text-align: right;
  font-size: 12px;
  line-height: 1.55;
  color: var(--ink-3);
  background: var(--paper-deep);
  overflow: hidden;
  user-select: none;
}
.editor-area {
  flex: 1;
  min-width: 0;
  height: 420px;
  padding: 10px 12px;
  border: none;
  outline: none;
  resize: none;
  font-size: 13px;
  line-height: 1.55;
  background: transparent;
  color: var(--ink);
  white-space: pre;
  overflow: auto;
}
.editor-status {
  display: flex;
  gap: 16px;
  align-items: center;
  margin-top: 8px;
  font-size: 12px;
  color: var(--ink-3);
}
.status-dirty { color: var(--yellow); }
.status-saved { color: var(--green); }
.editor-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
</style>