<script setup>
import {ref, reactive, computed, watch, onMounted, onActivated, onBeforeUnmount, nextTick} from 'vue'
import {ElMessage, ElMessageBox} from 'element-plus'
import {api, events} from '@/api'
import {writeClipboard} from '@/utils/clipboard'
import {renderMarkdown} from '@/utils/markdown-lite'

// 终端 AI 面板（交互参照 ChatGPT / Gemini / DSH 这类 agent 工具）
//
// 交互约定：
//   · 会话默认带上下文——直接点「发送」就是在继续对话；只有「重置对话」才会开新会话
//   · 模式（建议 / 直接执行 / 完全接管）在输入框下方切换，切到危险模式当场弹确认
//   · 输入框上方的功能行只放低频工具：上下文档位、预览上下文、重置对话
//   · 每条命令的 复制 / 插入 / 执行 挤在一行里，不换行
//
// 每个终端标签各有一份对话记录与自己的后端会话（chats[终端key]）。

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
const taRef = ref(null)

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
// 上下文档位：做成下拉，标签尽量短，省下横向空间
const CTX_LABEL = {
  last50: '最近 50 行',
  last200: '最近 200 行',
  sinceLast: '自上次 AI 命令',
  all: '全部 ≤32KB'
}

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
// 接管中且不在"等你输入"状态时，才收回终端的键盘
const locked = computed(() => {
  const st = chat.value.takeoverState
  if (!chat.value.takeoverFull) return false
  return !(st && st.state === 'paused' && (st.kind === 'input' || st.kind === 'altscreen'))
})
const takeoverActive = computed(() => !!(chat.value.takeoverFull && chat.value.takeoverState))
const canSend = computed(() => canUse.value && aiReady.value && !props.sync && !chat.value.takeoverFull && !!input.value.trim())
const ctxSizeText = computed(() => {
  const c = chat.value.lastContext
  if (!c) return ''
  return `${c.lines} 行 / ${(c.bytes / 1024).toFixed(1)} KB`
})

// ---- 消息与滚动 ----
// 用"贴底意图"判断，而不是每次现算位置：状态条出现/消失会让列表高度突变，
// 现算会得出"已经不在底部"于是不滚，结果最后一条被遮住一半。
const stickToBottom = ref(true)
function nearBottom(el) {
  return el.scrollHeight - el.scrollTop - el.clientHeight < 80
}
function onLogScroll() {
  const el = logRef.value
  if (el) stickToBottom.value = nearBottom(el)
}
function scrollToBottom(force) {
  if (force) stickToBottom.value = true
  if (!stickToBottom.value) return
  nextTick(() => {
    const el = logRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}
function push(item) {
  chat.value.messages.push(item)
  scrollToBottom()
}
// 内容一变就贴到底：覆盖"状态条出现/消失、输出折叠展开"这类不经过 push 的高度变化
let logObserver = null
function attachLogObserver() {
  const el = logRef.value
  if (!el || logObserver) return
  logObserver = new MutationObserver(() => scrollToBottom())
  logObserver.observe(el, {childList: true, subtree: true, characterData: true})
}

// 输入框随内容增高（默认 2 行，最多 ~6 行）
function autoGrow() {
  const el = taRef.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 132) + 'px'
}
watch(input, () => nextTick(autoGrow))

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
  if (['done', 'stopped', 'error', 'maxrounds'].includes(st.state)) c.takeoverFull = false
  emitTakeover()
}

function onAIStep(step) {
  if (!step) return
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
    // 把当前模式也带上去：终端页据此在"已选中完全接管但还没开跑"时就套上 ARMED 外框
    mode: mode.value,
    sessionId: c.sessionId,
    round: st ? st.round : 0,
    maxRounds: st ? st.maxRounds : 0,
    message: st ? st.message : ''
  })
}

watch(termKey, () => {
  subscribe(chat.value.sessionId)
  emitTakeover()
  scrollToBottom(true)
}, {immediate: true})

watch(() => (props.terminals || []).map(t => t.key).join('|'), () => {
  const alive = new Set((props.terminals || []).map(t => t.key))
  Object.keys(chats).forEach(k => {
    if (!alive.has(k)) delete chats[k]
  })
})

// ---- 模式切换：切到有风险的模式当场确认，而不是等到发送时 ----
async function setMode(v) {
  if (v === mode.value) return
  if (chat.value.takeoverFull) {
    ElMessage.warning('AI 接管进行中，无法切换模式（先点停止）')
    return
  }
  if (v === 'takeover') {
    if (!(await confirmDanger())) return
  } else if (v === 'exec') {
    try {
      await ElMessageBox.confirm(
        '直接执行模式下，AI 给出的命令会**在当前终端里自动执行**（每次一条，执行完停下并把结果回读给 AI）。\n\n' +
        '命令可能修改或删除服务器上的内容，建议先在测试服务器上确认效果。',
        '切换到「直接执行」',
        {type: 'warning', confirmButtonText: '我知道了', cancelButtonText: '取消'}
      )
    } catch (e) {
      return
    }
  }
  mode.value = v
  localStorage.setItem(MODE_KEY, v)
  emitTakeover()
  if (v === 'takeover') push({kind: 'note', state: 'paused', text: '已切换到完全接管模式：终端已进入待命状态，AI 将连续自动执行命令，可随时点「停止」'})
}
function setCtx(v) {
  ctxMode.value = v
  localStorage.setItem(CTX_KEY, v)
}

// 完全接管的危险确认
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

// ---- 发送：默认就是在继续当前会话 ----
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
    continueSession: true
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
  push({kind: 'user', text})
  input.value = ''
  nextTick(autoGrow)
  running.value = true
  busyText.value = mode.value === 'takeover' ? 'AI 正在规划下一步…' : 'AI 正在思考…'
  // 会话默认带上下文：沿用同一段会话（没有才新建）；只有「重置对话」会换新的
  const sid = chat.value.sessionId || (chat.value.sessionId = newSid())
  subscribe(sid)
  emitTakeover()

  try {
    if (mode.value === 'takeover') {
      chat.value.takeoverFull = true
      await api.aiStartTakeover(buildReq(text, 'takeover', sid))
      chat.value.takeoverState = {state: 'running', kind: '', round: 0, maxRounds: 0, message: 'AI 接管中'}
    } else {
      const resp = await api.aiChat(buildReq(text, mode.value, sid))
      chat.value.sessionId = resp.sessionId || sid
      chat.value.lastContext = resp.context || null
      push({kind: 'ai', text: resp.text || '（模型没有给出文字说明）'})
      const cmds = resp.commands || []
      if (!cmds.length) push({kind: 'note', state: 'info', text: '这次没有给出可执行的命令'})
      for (const c of cmds) push({kind: 'cmd', command: c.command, risk: c.risk, reason: c.reason})
      if (mode.value === 'exec' && cmds.length) await execCommand(cmds[0].command)
    }
  } catch (e) {
    push({kind: 'err', text: 'AI 调用失败：' + e})
    chat.value.takeoverFull = false
    chat.value.takeoverState = null
  } finally {
    running.value = false
    busyText.value = ''
    emitTakeover()
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

// 重置对话 = 开一段全新会话（后端上下文一起丢掉）
async function resetChat() {
  if (chat.value.takeoverFull) { ElMessage.warning('AI 正在接管，先停止再重置'); return }
  try {
    await api.aiReset(termId.value || '')
  } catch (e) { /* 忽略 */ }
  unsubscribeAll()
  chats[termKey.value] = blankChat()
  emitTakeover()
  ElMessage.success('已开始新对话')
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

onMounted(() => {
  loadAI()
  nextTick(() => {
    attachLogObserver()
    autoGrow()
  })
})
onBeforeUnmount(() => {
  if (logObserver) {
    logObserver.disconnect()
    logObserver = null
  }
})
onActivated(() => {
  loadAI()
  emitTakeover()
  scrollToBottom(true)
})
</script>

<template>
  <div class="ai-panel">
    <!-- 状态条：只在接管中 / 需要你处理时出现，一行搞定 -->
    <div v-if="takeoverActive" class="ai-status" :class="chat.takeoverState.state">
      <i v-if="chat.takeoverState.state === 'running'" class="ai-dot" />
      <span class="ai-status-title">{{ STATE_LABEL[chat.takeoverState.state] || chat.takeoverState.state }}</span>
      <span v-if="chat.takeoverState.maxRounds > 1" class="ai-status-round">
        {{ chat.takeoverState.round }}/{{ chat.takeoverState.maxRounds }}
      </span>
      <span class="ai-status-msg">{{ chat.takeoverState.message }}</span>
      <span class="ai-status-actions">
        <!-- 暂停时只需要一个动作：告诉 AI"我处理完了，继续"。
             Ctrl+C 与右侧「停止」功能重叠，已去掉。 -->
        <el-button
          v-if="chat.takeoverState.state === 'paused'"
          size="small"
          type="primary"
          @click="control('continue')"
        >{{ locked ? '继续等待' : '输入完成，继续' }}</el-button>
        <el-button size="small" type="danger" plain @click="stopTakeover">停止</el-button>
      </span>
    </div>

    <div v-if="aiHint" class="ai-line warn">{{ aiHint }}</div>
    <div v-else-if="sync" class="ai-line warn">并行（同步）模式下 AI 不可用，请先关闭「同步操作」</div>
    <div v-else-if="!canUse" class="ai-line warn">请先打开一个终端</div>

    <!-- 对话流 -->
    <div ref="logRef" class="ai-log" @scroll="onLogScroll">
      <div v-if="!chat.messages.length" class="ai-empty">
        <div class="ai-empty-item" @click="input = '怎么查 XXX.jar 的进程？'">怎么查 XXX.jar 的进程？</div>
        <div class="ai-empty-item" @click="input = '现在的终端内容发生了什么？爆的什么错？'">现在的终端内容发生了什么？</div>
        <div class="ai-empty-item" @click="input = '磁盘快满了，帮我看看哪里占得最多'">磁盘快满了，帮我看看哪里占得最多</div>
      </div>

      <template v-for="(m, i) in chat.messages" :key="i">
        <div v-if="m.kind === 'user'" class="ai-msg user">{{ m.text }}</div>

        <div v-else-if="m.kind === 'ai'" class="ai-msg ai" v-html="renderMarkdown(m.text)" />

        <div v-else-if="m.kind === 'err'" class="ai-msg err">{{ m.text }}</div>

        <div v-else-if="m.kind === 'note'" class="ai-note" :class="m.state">{{ m.text }}</div>

        <!-- 命令卡：徽标 + 轮次 + 动作挤在一行，窄窗口也不换行 -->
        <div v-else-if="m.kind === 'cmd'" class="ai-card cmd">
          <div class="ai-card-head">
            <span v-if="m.risk" class="ai-chip" :class="m.risk">{{ riskLabel[m.risk] || m.risk }}</span>
            <span v-if="m.round && chat.takeoverState && chat.takeoverState.maxRounds > 1" class="ai-round">第 {{ m.round }} 轮</span>
            <span class="ai-acts">
              <button class="ai-act" @click="copyCmd(m.command)">复制</button>
              <button class="ai-act" @click="insertCmd(m.command)">插入</button>
              <button class="ai-act primary" :disabled="locked" @click="execCommand(m.command)">执行</button>
            </span>
          </div>
          <div v-if="m.thought" class="ai-thought">{{ m.thought }}</div>
          <pre class="ai-cmd mono">{{ m.command }}</pre>
          <div v-if="m.reason" class="ai-reason">{{ m.reason }}</div>
        </div>

        <div v-else-if="m.kind === 'res'" class="ai-card res" :class="m.status">
          <div class="ai-card-head">
            <span v-if="m.status === 'done' && m.exitCode === 0" class="ai-ok">✓ 成功</span>
            <span v-else-if="m.status === 'done'" class="ai-bad">退出码 {{ m.exitCode }}</span>
            <span v-else class="ai-bad">{{ STATE_LABEL[m.status] || m.status }}</span>
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
      <div v-else-if="chat.takeoverFull && chat.takeoverState && chat.takeoverState.state === 'running'" class="ai-busy">
        <el-icon class="ai-spin"><Loading /></el-icon>
        <span>AI 接管中，命令在终端里执行…</span>
      </div>
    </div>

    <!-- 输入区：功能行 → 输入框（内嵌发送）→ 模式切换 -->
    <div class="ai-composer-wrap">
      <div class="ai-tools">
        <!-- 上下文档位：做成纯文字下拉，不带边框与内边距，尽量不占地方 -->
        <el-dropdown trigger="click" placement="top-start" :disabled="chat.takeoverFull" @command="setCtx">
          <button class="ai-tool" :disabled="chat.takeoverFull">
            上下文：{{ CTX_LABEL[ctxMode] }}<el-icon class="ai-caret"><ArrowDown /></el-icon>
          </button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-for="(label, val) in CTX_LABEL" :key="val" :command="val">{{ label }}</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <span v-if="ctxSizeText" class="ai-tool-info">{{ ctxSizeText }}</span>
        <span class="ai-tool-spacer" />
        <button class="ai-tool" :disabled="!canUse" @click="previewContext">预览</button>
        <button class="ai-tool" :disabled="locked || !chat.messages.length" @click="resetChat">重置对话</button>
      </div>

      <!-- 输入框：内部右下角是「权限（模式）下拉 + 发送」，与 DSH 一致 -->
      <div class="ai-composer" :class="{disabled: chat.takeoverFull}">
        <textarea
          ref="taRef"
          v-model="input"
          rows="2"
          class="ai-textarea"
          placeholder="给 AI 发消息（Enter 发送，Shift+Enter 换行）"
          @keydown.enter.exact.prevent="send"
          @input="autoGrow"
        />
        <div class="ai-composer-foot">
          <el-dropdown trigger="click" placement="top-end" :disabled="chat.takeoverFull" @command="setMode">
            <button
              class="ai-perm"
              :class="mode"
              :disabled="chat.takeoverFull"
              :title="MODE_TIP[mode]"
            >
              <i class="ai-perm-dot" />{{ MODE_LABEL[mode] }}<el-icon class="ai-caret"><ArrowUp /></el-icon>
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-for="m in ['suggest', 'exec', 'takeover']" :key="m" :command="m">
                  <span class="ai-perm-item" :class="m">
                    <i class="ai-perm-dot" />{{ MODE_LABEL[m] }}
                    <em>{{ MODE_TIP[m] }}</em>
                  </span>
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <button class="ai-send" :disabled="!canSend" :title="chat.takeoverFull ? 'AI 接管中' : '发送'" @click="send">
            <el-icon v-if="running"><Loading /></el-icon>
            <el-icon v-else><Promotion /></el-icon>
          </button>
        </div>
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

/* 状态条：一行，按钮右对齐 */
.ai-status {
  flex: 0 0 auto;
  margin: 6px 8px 0;
  padding: 5px 7px;
  border-radius: 8px;
  background: var(--red-soft);
  border: 1px solid var(--red);
  display: flex;
  align-items: center;
  gap: 5px;
}
.ai-status.paused {
  background: var(--yellow-soft);
  border-color: var(--yellow);
}
.ai-status.done {
  background: var(--green-soft);
  border-color: var(--green);
}
.ai-dot {
  flex: 0 0 auto;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--red);
  animation: breathe 1.6s ease-in-out infinite;
}
.ai-status-title {
  flex: 0 0 auto;
  font-size: 11.5px;
  font-weight: 700;
  color: var(--ink);
}
.ai-status-round {
  flex: 0 0 auto;
  font-size: 11px;
  color: var(--ink-2);
}
.ai-status-msg {
  flex: 1 1 100px;
  min-width: 0;
  font-size: 11px;
  line-height: 1.5;
  color: var(--ink-2);
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}
.ai-status-actions {
  flex: 0 0 auto;
  margin-left: auto;
  display: flex;
  gap: 2px;
}
.ai-line {
  flex: 0 0 auto;
  margin: 4px 8px 0;
  font-size: 11px;
  line-height: 1.5;
}
.ai-line.warn { color: var(--yellow); }

/* 对话流 */
.ai-log {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 6px 8px;
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.ai-empty {
  padding: 2px 0;
}
.ai-empty-item {
  padding: 3px 4px;
  font-size: 11.5px;
  color: var(--blue-deep);
  cursor: pointer;
  border-radius: 6px;
  line-height: 1.75;
}
.ai-empty-item:hover { background: var(--hover-bg); }
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
.ai-msg.err { background: var(--red-soft); color: var(--red); }
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
/* 关键：整行 nowrap + 动作区不压缩，窄窗口下也只在"徽标"与"动作"之间留白，不会换行 */
.ai-card-head {
  display: flex;
  align-items: center;
  gap: 4px;
  min-height: 18px;
  white-space: nowrap;
  overflow: hidden;
}
.ai-chip {
  flex: 0 0 auto;
  font-size: 10px;
  line-height: 15px;
  padding: 0 5px;
  border-radius: 4px;
  border: 1px solid var(--ink-3);
  color: var(--ink-3);
}
.ai-chip.low { border-color: var(--green); color: var(--green); }
.ai-chip.medium { border-color: var(--yellow); color: var(--yellow); }
.ai-chip.high { border-color: var(--red); color: var(--red); }
.ai-round {
  flex: 0 0 auto;
  font-size: 10.5px;
  color: var(--ink-3);
}
.ai-acts {
  margin-left: auto;
  flex: 0 0 auto;
  display: flex;
  align-items: center;
}
/* 微动作按钮：自己写而不是用 el-button —— Element 的按钮自带外边距，
   四个动作挤在一行时就会被迫换行 */
.ai-act {
  border: none;
  background: transparent;
  color: var(--ink-3);
  font-family: inherit;
  font-size: 11px;
  line-height: 18px;
  padding: 0 5px;
  border-radius: 5px;
  cursor: pointer;
  transition: background-color .15s ease, color .15s ease;
}
.ai-act:hover:not(:disabled) {
  background: var(--hover-bg);
  color: var(--blue-deep);
}
.ai-act.primary {
  color: var(--blue-deep);
  font-weight: 600;
}
.ai-act:disabled {
  opacity: .45;
  cursor: not-allowed;
}
.ai-ok { flex: 0 0 auto; color: var(--green); font-weight: 600; font-size: 11.5px; }
.ai-bad { flex: 0 0 auto; color: var(--yellow); font-weight: 600; font-size: 11.5px; }
.ai-thought {
  font-size: 11.5px;
  color: var(--ink-2);
  margin-top: 2px;
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

/* ---- 输入区 ---- */
.ai-composer-wrap {
  flex: 0 0 auto;
  padding: 6px 8px 8px;
  border-top: 1px solid var(--line);
}
/* 功能行：低频工具放在输入框上方（GPT / Gemini 的做法）。
   这里只留纯文字按钮，不加边框与内边距，尽量省横向空间。 */
.ai-tools {
  display: flex;
  align-items: center;
  gap: 2px;
  margin-bottom: 5px;
}
.ai-tool-info {
  font-size: 10.5px;
  color: var(--ink-3);
  white-space: nowrap;
  margin-left: 2px;
}
.ai-tool-spacer { flex: 1 1 auto; }
.ai-tool {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  border: none;
  background: transparent;
  color: var(--ink-3);
  font-family: inherit;
  font-size: 11px;
  line-height: 18px;
  padding: 0 4px;
  border-radius: 5px;
  cursor: pointer;
  transition: background-color .15s ease, color .15s ease;
}
.ai-tool:hover:not(:disabled) {
  background: var(--hover-bg);
  color: var(--blue-deep);
}
.ai-tool:disabled { opacity: .45; cursor: not-allowed; }
.ai-caret {
  font-size: 10px;
  opacity: .7;
}

/* 输入框：上部文本区 + 下部（右下角）权限下拉与发送按钮 */
.ai-composer {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px 6px 4px 10px;
  border-radius: 14px;
  border: 1px solid var(--line);
  background: var(--card);
  transition: border-color .2s ease, box-shadow .2s ease;
}
.ai-composer:focus-within {
  border-color: var(--blue);
  box-shadow: 0 0 0 3px var(--blue-focus);
}
.ai-composer.disabled {
  opacity: .7;
}
.ai-textarea {
  flex: 1 1 auto;
  min-width: 0;
  /* 默认就是 2 行高（12.5px × 1.6 ≈ 20px/行） */
  min-height: 40px;
  max-height: 132px;
  border: none;
  outline: none;
  background: transparent;
  color: var(--ink);
  font-family: inherit;
  font-size: 12.5px;
  line-height: 1.6;
  padding: 2px 0;
  resize: none;
  overflow-y: auto;
}
.ai-textarea::placeholder { color: var(--ink-3); }
.ai-composer-foot {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
}
/* 权限（模式）下拉：与发送按钮同在右下角，颜色随风险等级变化 */
.ai-perm {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 22px;
  padding: 0 6px;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: transparent;
  color: var(--ink-2);
  font-family: inherit;
  font-size: 11px;
  cursor: pointer;
  transition: all .18s ease;
}
.ai-perm:hover:not(:disabled) {
  border-color: var(--blue);
  color: var(--blue-deep);
}
.ai-perm.takeover {
  border-color: var(--red);
  color: var(--red);
  background: var(--red-soft);
}
.ai-perm.exec {
  border-color: var(--yellow);
  color: var(--yellow);
}
.ai-perm:disabled { opacity: .5; cursor: not-allowed; }
.ai-perm-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--green);
}
.ai-perm.exec .ai-perm-dot { background: var(--yellow); }
.ai-perm.takeover .ai-perm-dot { background: var(--red); }
/* 下拉项：名称 + 一句说明 */
.ai-perm-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
}
.ai-perm-item.exec .ai-perm-dot { background: var(--yellow); }
.ai-perm-item.takeover .ai-perm-dot { background: var(--red); }
.ai-perm-item em {
  font-style: normal;
  font-size: 11px;
  color: var(--ink-3);
}
.ai-send {
  flex: 0 0 auto;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  border: none;
  background: var(--blue);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  font-size: 14px;
  transition: background-color .2s ease, opacity .2s ease;
}
.ai-send:hover:not(:disabled) { background: var(--blue-deep); }
.ai-send:disabled {
  background: var(--line);
  color: var(--ink-3);
  cursor: not-allowed;
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

/* AI 回复里的 markdown（v-html 生成，需要 :deep） */
.ai-msg :deep(.md-p) { margin: 0 0 4px; }
.ai-msg :deep(.md-p:last-child) { margin-bottom: 0; }
.ai-msg :deep(.md-h) { font-weight: 600; margin: 4px 0; font-size: 12.5px; }
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