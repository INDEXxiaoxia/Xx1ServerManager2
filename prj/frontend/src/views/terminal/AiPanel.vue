<script setup>
import {ref, reactive, computed, watch, onMounted, onActivated, nextTick} from 'vue'
import {ElMessage, ElMessageBox} from 'element-plus'
import {api, events} from '@/api'
import {writeClipboard} from '@/utils/clipboard'
import {renderMarkdown} from '@/utils/markdown-lite'

// 终端 AI 对话面板
//
// 三个模式：建议（只给命令）/ 直接执行（跑一条就停）/ 完全接管（连续多轮）
// 命令注入到**用户看得见的那个终端**里执行。
//
// 对话按终端隔离：每个终端标签各有一份对话记录与自己的后端会话（chats[终端key]），
// 切到另一个终端就是一段新对话，切回来原来的记录还在。

const props = defineProps({
  activeTerminal: {type: Object, default: null},
  terminals: {type: Array, default: () => []},
  sync: {type: Boolean, default: false}
})

const emit = defineEmits(['insert', 'update:takeover'])

const MODE_KEY = 'xx1.ai.mode'
const CTX_KEY = 'xx1.ai.ctx'

const mode = ref(localStorage.getItem(MODE_KEY) || 'suggest')
const ctxMode = ref(localStorage.getItem(CTX_KEY) || 'last200')
const input = ref('')
const ctxLines = ref(200)
const running = ref(false)
const busyText = ref('')
const aiReady = ref(false)
const aiHint = ref('')
const previewVisible = ref(false)
const previewText = ref('')
const logRef = ref(null)

const MODE_LABEL = {suggest: '建议', exec: '直接执行', takeover: '完全接管'}
const MODE_TIP = {
  suggest: '只给命令：复制或插入到终端，你自己按回车执行',
  exec: 'AI 的命令会在当前终端执行一条，回读结果后停下',
  takeover: 'AI 连续自动执行，仅在测试服务器上使用 ⚠️'
}
const STATE_LABEL = {
  running: '执行中', paused: '需要你处理', stopped: '已停止',
  done: '已完成', error: '出错', maxrounds: '已达上限'
}
const riskLabel = {low: '低风险', medium: '改系统状态', high: '破坏性操作'}
const riskType = {low: 'success', medium: 'warning', high: 'danger'}

// ---- 按终端隔离的对话 ----
const chats = reactive({})
const termKey = computed(() => (props.activeTerminal && props.activeTerminal.key) || '')

function blankChat() {
  return {messages: [], sessionId: '', takeoverState: null, takeoverFull: false, lastContext: null}
}
function chatOf(k) {
  if (!chats[k]) chats[k] = blankChat()
  return chats[k]
}
const chat = computed(() => chatOf(termKey.value))
const termId = computed(() => (props.activeTerminal && props.activeTerminal.termId) || '')
const canUse = computed(() => !!termId.value)
// 接管中且不在"等你输入"状态时，才收回终端的键盘。
// 建议模式下 AI 并不碰终端，所以不算锁定（否则用户干等模型回复时连终端都不能用）。
const locked = computed(() => {
  const st = chat.value.takeoverState
  if (!chat.value.takeoverFull) return false
  return !(st && st.state === 'paused' && (st.kind === 'input' || st.kind === 'altscreen'))
})

// ---- 消息与滚动 ----
function scrollToBottom() {
  nextTick(() => {
    const el = logRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}
function push(item) {
  chat.value.messages.push(item)
  scrollToBottom()
}

function newSid() {
  return 'ai-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8)
}

let offs = []
function unsubscribeAll() {
  offs.forEach(f => {
    try { f() } catch (e) { /* 忽略 */ }
  })
  offs = []
}
// 先订阅再发起：AI 失败往往很快（Key 错、模型名错），
// 等 IPC 返回再订阅会漏掉首个状态事件，界面会一直停在"执行中"
function subscribe(sid) {
  unsubscribeAll()
  if (!sid) return
  offs.push(events.on('ai:state:' + sid, onAIState))
  offs.push(events.on('ai:step:' + sid, onAIStep))
}

function onAIState(st) {
  if (!st) return
  const c = chat.value
  c.takeoverState = st
  if (st.message && ['paused', 'error', 'maxrounds', 'done', 'stopped'].includes(st.state)) {
    push({kind: 'note', state: st.state, text: st.message})
  }
  if (['done', 'stopped', 'error', 'maxrounds'].includes(st.state)) {
    c.takeoverFull = false
  }
  emitTakeover()
}

function onAIStep(step) {
  if (!step) return
  // 后端每轮推两条：先"准备执行"（无 result），执行完再推带 result 的
  if (!step.result) {
    push({kind: 'cmd', command: step.command, risk: step.risk, thought: step.thought, round: step.round})
    return
  }
  push({kind: 'res', ...step.result})
}

function emitTakeover() {
  const c = chat.value
  const st = c.takeoverState
  emit('update:takeover', {
    active: c.takeoverFull || running.value,
    full: !!c.takeoverFull,
    paused: !!(st && st.state === 'paused'),
    kind: (st && st.kind) || '',
    locked: locked.value,
    sessionId: c.sessionId,
    round: st ? st.round : 0,
    maxRounds: st ? st.maxRounds : 0,
    message: st ? st.message : ''
  })
}

// 切换终端：换一段对话；若那段对话的会话还在跑，重新挂上事件订阅
watch(termKey, () => {
  const c = chat.value
  subscribe(c.sessionId)
  emitTakeover()
  scrollToBottom()
}, {immediate: true})

// 终端被关掉时清掉它的对话记录
watch(() => (props.terminals || []).map(t => t.key).join('|'), () => {
  const alive = new Set((props.terminals || []).map(t => t.key))
  Object.keys(chats).forEach(k => {
    if (!alive.has(k)) delete chats[k]
  })
})

function setMode(v) {
  if (chat.value.takeoverFull) {
    ElMessage.warning('AI 接管进行中，无法切换模式（先点停止）')
    return
  }
  mode.value = v
  localStorage.setItem(MODE_KEY, v)
}
function setCtx(v) {
  ctxMode.value = v
  localStorage.setItem(CTX_KEY, v)
}

// ---- 发送 ----
function buildReq(text, m, sid) {
  return {
    sessionId: sid,
    termId: termId.value,
    serverId: props.activeTerminal ? props.activeTerminal.serverId : '',
    text,
    mode: m,
    context: ctxMode.value,
    lines: ctxLines.value,
    selection: '',
    maxRounds: 0,
    continueSession: false
  }
}

function guard() {
  if (!canUse.value) { ElMessage.warning('请先打开一个终端'); return false }
  if (!aiReady.value) { ElMessage.warning('还没有配置 AI：设置 → AI 设置'); return false }
  if (props.sync) { ElMessage.warning('并行（同步）模式下不支持 AI：请先关闭「同步操作」'); return false }
  if (chat.value.takeoverFull) { ElMessage.warning('AI 正在接管，请先停止或等它跑完'); return false }
  return true
}

async function send() {
  const text = input.value.trim()
  if (!text || !guard()) return
  if (mode.value === 'takeover' && !(await confirmDanger())) return

  push({kind: 'user', text})
  input.value = ''
  running.value = true
  busyText.value = mode.value === 'takeover' ? 'AI 正在规划第一步…' : 'AI 正在思考…'
  emitTakeover()

  try {
    if (mode.value === 'takeover') {
      const sid = newSid()
      chat.value.sessionId = sid
      subscribe(sid)
      chat.value.takeoverFull = true
      await api.aiStartTakeover(buildReq(text, 'takeover', sid))
      chat.value.takeoverState = {state: 'running', kind: '', round: 0, maxRounds: 0, message: 'AI 接管中'}
    } else {
      const resp = await api.aiChat(buildReq(text, mode.value, newSid()))
      chat.value.lastContext = resp.context || null
      push({kind: 'ai', text: resp.text || '（模型没有给出文字说明）'})
      const cmds = resp.commands || []
      if (!cmds.length) push({kind: 'note', state: 'info', text: '这次没有给出可执行的命令'})
      for (const c of cmds) push({kind: 'cmd', command: c.command, risk: c.risk, reason: c.reason})
      if (mode.value === 'exec' && cmds.length) await execCommand(cmds[0].command)
    }
  } catch (e) {
    // 抛异常 = 任务根本没起来：必须把接管状态收回来，否则终端会一直停在锁定态
    push({kind: 'err', text: 'AI 调用失败：' + e})
    chat.value.takeoverFull = false
    chat.value.takeoverState = null
  } finally {
    running.value = false
    busyText.value = ''
    emitTakeover()
  }
}

// 完全接管的危险确认：必须把"不能回退"讲清楚
async function confirmDanger() {
  try {
    await ElMessageBox.confirm(
      '完全接管模式下，AI 会在服务器上自动连续执行命令，包括安装、删除、重启等有风险的操作，' +
      '且不会逐条征求你的同意。\n\n' +
      '⚠️ 请仅在测试服务器上使用，不要在任何业务服务器上使用；出问题本软件无法回退。\n\n' +
      '执行期间终端会显示"AI 接管中"。遇到需要输入密码时终端会自动解锁并提示你。',
      '⚠️ 完全接管模式（实验功能）',
      {type: 'error', confirmButtonText: '我已知悉风险，仅用于测试环境', cancelButtonText: '取消'}
    )
    return true
  } catch (e) {
    return false
  }
}

async function execCommand(command) {
  if (!termId.value) { ElMessage.warning('请先打开一个终端'); return }
  const sid = chat.value.sessionId || (chat.value.sessionId = newSid())
  subscribe(sid)
  chat.value.takeoverFull = true
  chat.value.takeoverState = {state: 'running', kind: '', round: 1, maxRounds: 1, message: '正在执行一条命令'}
  emitTakeover()
  try {
    const res = await api.aiExecOne(termId.value, sid, command)
    push({kind: 'res', ...res})
  } catch (e) {
    push({kind: 'err', text: '执行失败：' + e})
  } finally {
    chat.value.takeoverFull = false
    chat.value.takeoverState = null
    emitTakeover()
  }
}

async function control(action) {
  if (!chat.value.sessionId) return
  try {
    await api.aiTakeoverControl(chat.value.sessionId, action)
    if (action === 'continue') push({kind: 'note', state: 'info', text: '已继续，AI 接着执行'})
    if (action === 'ctrlc') push({kind: 'note', state: 'info', text: '已发送 Ctrl+C 中断这一步'})
  } catch (e) {
    ElMessage.error('操作失败：' + e)
  }
}

async function stopTakeover() {
  if (!chat.value.sessionId) return
  try {
    await api.aiStopTakeover(chat.value.sessionId)
  } catch (e) {
    ElMessage.error('停止失败：' + e)
  }
}

// 任务停下后追加新要求：复用同一段上下文继续跑
async function appendTask() {
  const text = input.value.trim()
  if (!text) { ElMessage.warning('请先在输入框里写要追加的要求'); return }
  if (props.sync) { ElMessage.warning('并行（同步）模式下不支持 AI'); return }
  if (!(await confirmDanger())) return
  push({kind: 'user', text: '（追加）' + text})
  input.value = ''
  running.value = true
  try {
    const sid = chat.value.sessionId || (chat.value.sessionId = newSid())
    subscribe(sid)
    chat.value.takeoverFull = true
    const req = buildReq(text, 'takeover', sid)
    req.continueSession = true
    await api.aiStartTakeover(req)
  } catch (e) {
    push({kind: 'err', text: '追加任务失败：' + e})
    chat.value.takeoverFull = false
    chat.value.takeoverState = null
  } finally {
    running.value = false
    emitTakeover()
  }
}

async function clearChat() {
  if (chat.value.takeoverFull) { ElMessage.warning('AI 正在接管，先停止再清空'); return }
  try {
    await api.aiReset(termId.value || '')
  } catch (e) { /* 忽略 */ }
  unsubscribeAll()
  const keep = termKey.value
  chats[keep] = blankChat()
  emitTakeover()
  ElMessage.success('已新建对话')
}

// ---- 命令卡动作 ----
async function copyCmd(text) {
  if (await writeClipboard(text)) ElMessage.success('命令已复制')
  else ElMessage.error('复制失败')
}
function insertCmd(text) {
  if (!canUse.value) { ElMessage.warning('请先打开一个终端'); return }
  emit('insert', text)
}
async function previewContext() {
  if (!termId.value) { ElMessage.warning('请先打开一个终端'); return }
  try {
    const ctx = await api.aiTerminalSnapshot(termId.value, ctxMode.value, ctxLines.value, '')
    previewText.value = ctx.text || '（当前没有可读的终端内容）'
    previewVisible.value = true
  } catch (e) {
    ElMessage.error('读取终端内容失败：' + e)
  }
}

async function loadAI() {
  try {
    const s = await api.getAISettings()
    ctxLines.value = s.contextLines || 200
    aiReady.value = !!(s.baseURL && s.model)
    aiHint.value = aiReady.value ? '' : '未配置 AI：请到「设置 → AI 设置」填写接口地址、API Key 与模型'
  } catch (e) {
    aiReady.value = false
    aiHint.value = '读取 AI 设置失败：' + e
  }
}

onMounted(loadAI)
onActivated(() => {
  loadAI()
  emitTakeover()
  scrollToBottom()
})
</script>

<template>
  <div class="ai-panel">
    <!-- 模式：紧凑分段控件 -->
    <div class="ai-modes">
      <button
        v-for="m in ['suggest', 'exec', 'takeover']"
        :key="m"
        class="ai-mode"
        :class="{active: mode === m, danger: m === 'takeover'}"
        :title="MODE_TIP[m]"
        :disabled="chat.takeoverFull"
        @click="setMode(m)"
      >{{ MODE_LABEL[m] }}</button>
    </div>

    <div v-if="aiHint" class="ai-line warn">{{ aiHint }}</div>
    <div v-else-if="sync" class="ai-line warn">并行（同步）模式下 AI 不可用，请先关闭「同步操作」</div>
    <div v-else-if="!canUse" class="ai-line warn">请先打开一个终端</div>

    <!-- 接管 / 需要你处理的提示条（紧凑，不含多余元素） -->
    <div v-if="chat.takeoverState && chat.takeoverFull" class="ai-status" :class="chat.takeoverState.state">
      <span class="ai-status-title">{{ STATE_LABEL[chat.takeoverState.state] || chat.takeoverState.state }}</span>
      <span v-if="chat.takeoverState.maxRounds && chat.takeoverState.maxRounds > 1" class="ai-status-round">
        {{ chat.takeoverState.round }}/{{ chat.takeoverState.maxRounds }}
      </span>
      <span class="ai-status-msg">{{ chat.takeoverState.message }}</span>
      <div class="ai-status-actions">
        <template v-if="chat.takeoverState.state === 'paused'">
          <el-button v-if="locked" size="small" @click="control('continue')">继续等待</el-button>
          <el-button v-else size="small" type="primary" @click="control('continue')">输入完成，继续</el-button>
          <el-button size="small" @click="control('ctrlc')">发送 Ctrl+C</el-button>
        </template>
        <el-button size="small" type="danger" plain @click="stopTakeover">停止</el-button>
      </div>
    </div>

    <!-- 对话流 -->
    <div ref="logRef" class="ai-log">
      <div v-if="!chat.messages.length" class="ai-empty">
        <div class="ai-empty-item" @click="input = '怎么查 XXX.jar 的进程？'">· 怎么查 XXX.jar 的进程？</div>
        <div class="ai-empty-item" @click="input = '现在的终端内容发生了什么？爆的什么错？'">· 现在的终端内容发生了什么？</div>
        <div class="ai-empty-item" @click="input = '磁盘快满了，帮我看看哪里占得最多'">· 磁盘快满了，帮我看看哪里占得最多</div>
      </div>

      <template v-for="(m, i) in chat.messages" :key="i">
        <div v-if="m.kind === 'user'" class="ai-msg user">{{ m.text }}</div>

        <div v-else-if="m.kind === 'ai'" class="ai-msg ai" v-html="renderMarkdown(m.text)" />

        <div v-else-if="m.kind === 'err'" class="ai-msg err">{{ m.text }}</div>

        <div v-else-if="m.kind === 'note'" class="ai-note" :class="m.state">{{ m.text }}</div>

        <div v-else-if="m.kind === 'cmd'" class="ai-card cmd">
          <div class="ai-card-head">
            <el-tag v-if="m.risk" size="small" :type="riskType[m.risk] || 'info'" effect="plain" class="ai-risk">
              {{ riskLabel[m.risk] || m.risk }}
            </el-tag>
            <span v-if="m.round && chat.takeoverState && chat.takeoverState.maxRounds > 1" class="ai-round">第 {{ m.round }} 轮</span>
            <span class="ai-card-actions">
              <el-button text size="small" @click="copyCmd(m.command)">复制</el-button>
              <el-button text size="small" @click="insertCmd(m.command)">插入</el-button>
              <el-button text size="small" type="primary" :disabled="locked" @click="execCommand(m.command)">执行</el-button>
            </span>
          </div>
          <div v-if="m.thought" class="ai-thought">{{ m.thought }}</div>
          <pre class="ai-cmd mono">{{ m.command }}</pre>
          <div v-if="m.reason" class="ai-reason">{{ m.reason }}</div>
        </div>

        <div v-else-if="m.kind === 'res'" class="ai-card res" :class="m.status">
          <div class="ai-card-head">
            <span class="ai-res-state">
              <span v-if="m.status === 'done' && m.exitCode === 0" class="ai-ok">✓ 成功</span>
              <span v-else-if="m.status === 'done'" class="ai-bad">退出码 {{ m.exitCode }}</span>
              <span v-else class="ai-bad">{{ STATE_LABEL[m.status] || m.status }}</span>
            </span>
            <span class="ai-round">{{ (m.elapsedMs / 1000).toFixed(1) }}s</span>
          </div>
          <div v-if="m.message" class="ai-reason">{{ m.message }}</div>
          <details v-if="m.output" class="ai-output" open>
            <summary>{{ m.truncated ? '输出（已截断）' : '输出' }}</summary>
            <pre class="mono">{{ m.output }}</pre>
          </details>
        </div>
      </template>

      <div v-if="running && !chat.takeoverFull" class="ai-busy">
        <el-icon class="ai-spin"><Loading /></el-icon>
        <span>{{ busyText }}</span>
      </div>
    </div>

    <!-- 输入区：紧凑两行起，自动增高 -->
    <div class="ai-input-wrap">
      <el-input
        v-model="input"
        type="textarea"
        :autosize="{minRows: 2, maxRows: 6}"
        resize="none"
        placeholder="描述你的问题，或粘贴一段看不懂的输出（Enter 发送）"
        @keydown.enter.exact.prevent="send"
      />
      <div class="ai-input-foot">
        <el-select v-model="ctxMode" size="small" class="ai-ctx-select" :disabled="chat.takeoverFull" @change="setCtx">
          <el-option label="最近 50 行" value="last50" />
          <el-option label="最近 200 行" value="last200" />
          <el-option label="自上次 AI 命令以来" value="sinceLast" />
          <el-option label="全部（≤32KB）" value="all" />
        </el-select>
        <el-tooltip content="查看这次会把哪段终端内容发给 AI" placement="top">
          <el-button text size="small" :disabled="!canUse" @click="previewContext">预览上下文</el-button>
        </el-tooltip>
        <span class="ai-foot-spacer" />
        <el-tooltip content="清空当前终端的对话，并开始新对话" placement="top">
          <el-button text size="small" :disabled="locked || !chat.messages.length" @click="clearChat">新建对话</el-button>
        </el-tooltip>
        <el-button
          v-if="chat.takeoverState && !chat.takeoverFull && ['stopped','done','maxrounds','error'].includes(chat.takeoverState.state)"
          size="small"
          @click="appendTask"
        >追加任务</el-button>
        <el-button type="primary" size="small" :loading="running" :disabled="chat.takeoverFull || !canUse" @click="send">
          发送
        </el-button>
      </div>
      <div v-if="chat.lastContext" class="ai-ctx-info text-muted">
        上次附带：{{ chat.lastContext.lines }} 行 / {{ (chat.lastContext.bytes / 1024).toFixed(1) }} KB
        <span v-if="chat.lastContext.truncated">· 已截断</span>
        <span v-if="chat.lastContext.elided">· 重复行已折叠</span>
        <span v-if="chat.lastContext.dropped">· 起点已被覆盖</span>
      </div>
    </div>

    <el-dialog v-model="previewVisible" title="将发送给 AI 的终端内容" width="720px" append-to-body>
      <pre class="mono ai-preview">{{ previewText }}</pre>
      <template #footer>
        <el-button @click="previewVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.ai-panel {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
}

/* 模式选择：紧凑分段 */
.ai-modes {
  display: flex;
  gap: 3px;
  padding: 6px 8px 4px;
  flex: 0 0 auto;
}
.ai-mode {
  flex: 1;
  height: 24px;
  line-height: 22px;
  border-radius: 7px;
  border: 1px solid var(--line);
  background: var(--card);
  color: var(--ink-2);
  font-family: inherit;
  font-size: 11.5px;
  cursor: pointer;
  transition: all .18s ease;
  padding: 0;
}
.ai-mode:hover:not(:disabled) {
  color: var(--blue-deep);
  border-color: var(--blue);
}
.ai-mode.active {
  background: var(--blue-soft);
  border-color: var(--blue);
  color: var(--blue-deep);
  font-weight: 600;
}
.ai-mode.danger.active {
  background: var(--red-soft);
  border-color: var(--red);
  color: var(--red);
}
.ai-mode:disabled {
  opacity: .5;
  cursor: not-allowed;
}
.ai-line {
  margin: 0 8px 4px;
  font-size: 11px;
  line-height: 1.5;
}
.ai-line.warn {
  color: var(--yellow);
}

/* 状态条：一行搞定，按钮右对齐 */
.ai-status {
  margin: 0 8px 4px;
  padding: 5px 7px;
  border-radius: 8px;
  background: var(--red-soft);
  border: 1px solid var(--red);
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  flex: 0 0 auto;
}
.ai-status.paused {
  background: var(--yellow-soft);
  border-color: var(--yellow);
}
.ai-status.done {
  background: var(--green-soft);
  border-color: var(--green);
}
.ai-status-title {
  font-size: 11.5px;
  font-weight: 700;
  color: var(--ink);
  flex: 0 0 auto;
}
.ai-status-round {
  font-size: 11px;
  color: var(--ink-2);
  flex: 0 0 auto;
}
.ai-status-msg {
  flex: 1 1 120px;
  min-width: 0;
  font-size: 11px;
  line-height: 1.55;
  color: var(--ink-2);
}
.ai-status-actions {
  flex: 0 0 auto;
  display: flex;
  gap: 3px;
  margin-left: auto;
}

/* 对话流 */
.ai-log {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 2px 8px 6px;
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.ai-empty {
  padding: 4px 0;
}
.ai-empty-item {
  padding: 3px 4px;
  font-size: 11.5px;
  color: var(--blue-deep);
  cursor: pointer;
  border-radius: 6px;
  line-height: 1.7;
}
.ai-empty-item:hover {
  background: var(--hover-bg);
}
.ai-msg {
  font-size: 12px;
  line-height: 1.65;
  padding: 6px 8px;
  border-radius: 9px;
  word-break: break-word;
}
.ai-msg.user {
  background: var(--blue-soft);
  color: var(--ink);
  align-self: flex-end;
  max-width: 94%;
}
.ai-msg.ai {
  background: var(--panel-2);
  border: 1px solid var(--line);
  color: var(--ink);
}
.ai-msg.err {
  background: var(--red-soft);
  color: var(--red);
}
.ai-note {
  font-size: 11px;
  padding: 4px 7px;
  border-radius: 7px;
  background: var(--hover-bg);
  color: var(--ink-2);
  line-height: 1.6;
}
.ai-note.error { background: var(--red-soft); color: var(--red); }
.ai-note.paused { background: var(--yellow-soft); color: var(--yellow); }
.ai-note.done { background: var(--green-soft); color: var(--green); }
.ai-note.maxrounds { background: var(--yellow-soft); color: var(--yellow); }

/* 命令卡 / 结果卡 */
.ai-card {
  border: 1px solid var(--line);
  border-radius: 9px;
  padding: 6px 8px;
  background: var(--card);
}
.ai-card.cmd { border-left: 3px solid var(--blue); }
.ai-card.res.done { border-left: 3px solid var(--green); }
.ai-card.res.blocked,
.ai-card.res.timeout,
.ai-card.res.interrupted { border-left: 3px solid var(--yellow); }
.ai-card-head {
  display: flex;
  align-items: center;
  gap: 5px;
  min-height: 20px;
}
.ai-risk { height: 18px; line-height: 16px; }
.ai-round {
  margin-left: auto;
  font-size: 10.5px;
  color: var(--ink-3);
}
.ai-card-actions {
  margin-left: auto;
  display: flex;
  gap: 0;
}
.ai-res-state { font-size: 11.5px; }
.ai-ok { color: var(--green); font-weight: 600; }
.ai-bad { color: var(--yellow); font-weight: 600; }
.ai-thought {
  font-size: 11.5px;
  color: var(--ink-2);
  margin-top: 3px;
  line-height: 1.55;
}
.ai-cmd {
  margin: 4px 0 0;
  padding: 5px 7px;
  background: var(--terminal-bg);
  color: var(--terminal-fg);
  border-radius: 7px;
  font-size: 11px;
  overflow-x: auto;
  white-space: pre;
}
.ai-reason {
  font-size: 11px;
  color: var(--ink-3);
  line-height: 1.55;
  margin-top: 3px;
}
.ai-output { margin-top: 4px; }
.ai-output summary {
  font-size: 11px;
  color: var(--ink-3);
  cursor: pointer;
}
.ai-output pre {
  margin: 3px 0 0;
  padding: 5px 7px;
  background: var(--terminal-bg);
  color: var(--terminal-fg);
  border-radius: 7px;
  max-height: 200px;
  overflow: auto;
  font-size: 10.5px;
}
.ai-busy {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  color: var(--blue-deep);
}
.ai-spin { animation: ai-rotate 1.1s linear infinite; }
@keyframes ai-rotate { to { transform: rotate(360deg); } }

/* 输入区 */
.ai-input-wrap {
  flex: 0 0 auto;
  padding: 6px 8px 8px;
  border-top: 1px solid var(--line);
}
.ai-input-wrap :deep(.el-textarea__inner) {
  font-size: 12px;
  padding: 5px 8px;
}
.ai-input-foot {
  display: flex;
  align-items: center;
  gap: 2px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.ai-ctx-select { width: 132px; }
.ai-foot-spacer { flex: 1 1 auto; }
.ai-ctx-info {
  font-size: 10.5px;
  margin-top: 3px;
  line-height: 1.5;
}
.ai-preview {
  max-height: 420px;
  overflow: auto;
  padding: 8px;
  background: var(--terminal-bg);
  color: var(--terminal-fg);
  border-radius: 8px;
  font-size: 11.5px;
}

/* AI 回复里的 markdown 元素（v-html 生成，需要 :deep） */
.ai-msg :deep(.md-p) { margin: 0 0 4px; }
.ai-msg :deep(.md-p:last-child) { margin-bottom: 0; }
.ai-msg :deep(.md-h) { font-weight: 600; margin: 4px 0; }
.ai-msg :deep(.md-h1) { font-size: 14px; }
.ai-msg :deep(.md-h2) { font-size: 13px; }
.ai-msg :deep(.md-h3), .ai-msg :deep(.md-h4) { font-size: 12.5px; }
.ai-msg :deep(.md-ul) { margin: 2px 0; padding-left: 16px; }
.ai-msg :deep(code) {
  background: var(--hover-bg);
  border-radius: 4px;
  padding: 0 3px;
  font-family: ui-monospace, Consolas, monospace;
  font-size: 11.5px;
}
.ai-msg :deep(.md-code) {
  margin: 4px 0;
  padding: 6px 8px;
  background: var(--terminal-bg);
  color: var(--terminal-fg);
  border-radius: 7px;
  overflow-x: auto;
  font-size: 11px;
  white-space: pre;
}
</style>