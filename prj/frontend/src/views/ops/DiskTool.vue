<script setup>
import {ref, computed, onMounted, onBeforeUnmount} from 'vue'
import {useRouter} from 'vue-router'
import {ElMessage} from 'element-plus'
import {api, events} from '@/api'
import {writeClipboard, fmtMB, fmtPct} from '@/utils/clipboard'

const props = defineProps({
  serverId: {type: String, required: true}
})

const router = useRouter()

// ---- 选择要检测的盘 ----
const disks = ref([])
const mount = ref('')
const useSudo = ref(false)
const loadingDisks = ref(false)
const disksError = ref('')

async function loadDisks() {
  loadingDisks.value = true
  disksError.value = ''
  try {
    disks.value = await api.listServerDisks(props.serverId)
    // 默认落在"使用率最高的真实盘"上（与后端 pickKeyDisk 的口径一致）
    const real = disks.value.filter(d => !d.isPseudo && !d.isReadOnly)
    const pool = real.length ? real : disks.value.filter(d => !d.isPseudo)
    const best = (pool.length ? pool : disks.value).slice().sort((a, b) => (b.usage || 0) - (a.usage || 0))[0]
    mount.value = best ? best.mount : '/'
  } catch (e) {
    disksError.value = '获取磁盘列表失败：' + e
  } finally {
    loadingDisks.value = false
  }
}

const diskOptions = computed(() => disks.value.map(d => ({
  value: d.mount,
  label: `${d.mount}　${fmtMB(d.usedMB)} / ${fmtMB(d.totalMB)}（${fmtPct(d.usage)}）` +
    (d.isPseudo ? ' · 伪文件系统' : '') + (d.isReadOnly ? ' · 只读' : ''),
  disabled: false
})))

// ---- 检测任务 ----
const taskId = ref('')
const running = ref(false)
const progressText = ref('')
const errorText = ref('')
const noticeText = ref('')
const report = ref(null)
let offs = []

const STAGE_TEXT = {
  connecting: '正在连接服务器…',
  quickscan: '正在检查容量、inode、系统日志与已删除句柄…',
  volume: '正在扫描目录占用（大目录可能需要十几秒）…',
  done: '分析完成'
}

function unsubscribeAll() {
  offs.forEach(f => {
    try { f() } catch (e) { /* 忽略 */ }
  })
  offs = []
}

function onProgress(p) {
  const stage = p && p.stage ? p.stage : ''
  progressText.value = (p && p.message) || STAGE_TEXT[stage] || stage
}

function onReport(rep) {
  // 后端已保证切片非 nil；这里再兜一层，避免任何 null 让整页渲染失败
  report.value = rep ? {
    ...rep,
    findings: rep.findings || [],
    topDirs: rep.topDirs || [],
    bigFiles: rep.bigFiles || [],
    notes: rep.notes || []
  } : null
}

function onDone(info) {
  running.value = false
  taskId.value = ''
  unsubscribeAll()
  if (!info) return
  if (info.ok) {
    progressText.value = ''
    ElMessage.success(`分析完成，用时 ${((info.elapsedMs || 0) / 1000).toFixed(1)}s`)
    return
  }
  if (info.cancelled) {
    progressText.value = ''
    noticeText.value = '已取消目录扫描，下面显示的是已完成的瞬时检查结果。'
    return
  }
  progressText.value = ''
  errorText.value = info.message || '检测失败'
}

function subscribe(id) {
  unsubscribeAll()
  offs.push(events.on('ops:disk:progress:' + id, onProgress))
  offs.push(events.on('ops:disk:report:' + id, onReport))
  offs.push(events.on('ops:disk:done:' + id, onDone))
}

async function start() {
  if (!mount.value) {
    ElMessage.warning('请先选择要检测的磁盘')
    return
  }
  // 下钻用的是同一个 SSH 连接池之外的一次性连接，等它拿完结果再重开，避免两次扫描互相盖结果
  if (drillTarget.value) {
    ElMessage.warning(`正在统计「${drillTarget.value}」的体积，等它跑完再重新检测`)
    return
  }
  resetResult()
  running.value = true
  progressText.value = STAGE_TEXT.connecting
  // 关键顺序：先生成 taskId 并订阅事件，再发起检测。
  // 反过来的话，检测若极快失败（SSH 连不上），首个事件会在订阅之前就发出去，
  // 界面上就会一直停在"分析中"。
  const id = 'dt-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8)
  taskId.value = id
  subscribe(id)
  try {
    await api.startDiskAnalysis(props.serverId, mount.value, useSudo.value, id)
  } catch (e) {
    running.value = false
    taskId.value = ''
    unsubscribeAll()
    errorText.value = String(e)
  }
}

async function cancel() {
  if (!taskId.value) {
    running.value = false
    return
  }
  try {
    await api.cancelDiskAnalysis(taskId.value)
  } catch (e) {
    ElMessage.error('取消失败：' + e)
  }
}

// ---- 目录下钻 ----
// drillStack 第 0 层是报告给的一级目录，之后每层是点进去的结果
const drillStack = ref([])
const drilling = ref(false)

function resetResult() {
  report.value = null
  errorText.value = ''
  noticeText.value = ''
  drillStack.value = []
}

async function rerunAsRoot() {
  useSudo.value = true
  ElMessage.info('将以 root 权限重新扫描（使用已保存的服务器密码执行 sudo）')
  await start()
}

function openTerminal() {
  router.push({path: '/terminal', query: {serverId: props.serverId}})
}

// ---- 目录下钻 ----
// drillStack：第 0 层是报告给的一级目录，之后每层是点进去的结果
const currentDirs = computed(() => {
  if (drillStack.value.length) {
    const top = drillStack.value[drillStack.value.length - 1]
    return top.items.map(it => ({...it, percent: pctOf(it)}))
  }
  return (report.value && report.value.topDirs) || []
})

const currentPath = computed(() => {
  if (!drillStack.value.length) return report.value ? report.value.mount : ''
  return drillStack.value[drillStack.value.length - 1].path
})

// 当前层是不是"被超时中断的部分结果"
const currentLevel = computed(() => (drillStack.value.length ? drillStack.value[drillStack.value.length - 1] : null))

function pctOf(item) {
  const used = report.value ? report.value.usedMB : 0
  if (!used) return item.percent || 0
  return Number(((item.sizeMB || 0) * 100 / used).toFixed(2))
}

// 下钻时的「局部」加载提示：只盖住目录列表那一块，不遮整页。
// du 扫大目录本来就要十几秒，没有反馈用户会以为点不动了。
const drillTarget = ref('')
const drillElapsed = ref(0)
let drillTimer = null

function startDrillTimer() {
  stopDrillTimer()
  drillElapsed.value = 0
  drillTimer = setInterval(() => { drillElapsed.value += 1 }, 1000)
}
function stopDrillTimer() {
  if (drillTimer) {
    clearInterval(drillTimer)
    drillTimer = null
  }
}

async function drill(item) {
  if (drilling.value || !item.hasChildren) return
  drilling.value = true
  drillTarget.value = item.path
  startDrillTimer()
  try {
    const res = await api.drillDiskDir(props.serverId, mount.value, item.path, useSudo.value)
    const items = (res && res.items) || []
    if (!items.length) {
      ElMessage.info(`「${item.path}」下面没有子项（或没有读权限）`)
      return
    }
    drillStack.value.push({
      path: item.path,
      items,
      partial: !!(res && res.partial),
      note: (res && res.note) || ''
    })
  } catch (e) {
    ElMessage.error('下钻失败：' + e)
  } finally {
    drilling.value = false
    drillTarget.value = ''
    stopDrillTimer()
  }
}

function drillUp() {
  drillStack.value.pop()
}

function drillRoot() {
  drillStack.value = []
}

// 面包屑回退到第 i 层
function drillTo(i) {
  drillStack.value = drillStack.value.slice(0, i + 1)
}

// ---- 展示辅助 ----
const LEVEL_META = {
  critical: {label: '严重', color: 'var(--red)', bg: 'var(--red-soft)'},
  warning: {label: '注意', color: 'var(--yellow)', bg: 'var(--yellow-soft)'},
  info: {label: '提示', color: 'var(--blue)', bg: 'var(--blue-soft)'},
  unavailable: {label: '不可用', color: 'var(--ink-3)', bg: 'var(--hover-bg)'}
}
function levelMeta(level) {
  return LEVEL_META[level] || LEVEL_META.info
}

const KIND_LABEL = {
  usage: '容量', inode: 'inode', 'deleted-open': '已删除句柄', gap: '差额',
  journal: '系统日志', docker: 'Docker', bigfile: '大文件', permission: '权限',
  pseudo: '文件系统', reserved: '预留块'
}
function kindLabel(kind) {
  return KIND_LABEL[kind] || kind
}

function usageColor(pct) {
  if (pct >= 90) return 'var(--red)'
  if (pct >= 80) return 'var(--yellow)'
  return 'var(--green)'
}

const criticalCount = computed(() => (report.value ? report.value.findings.filter(f => f.level === 'critical').length : 0))
const warningCount = computed(() => (report.value ? report.value.findings.filter(f => f.level === 'warning').length : 0))
const headline = computed(() => {
  if (!report.value) return ''
  const n = criticalCount.value + warningCount.value
  if (!n) return '没有发现明显问题'
  const parts = []
  if (criticalCount.value) parts.push(`${criticalCount.value} 项严重`)
  if (warningCount.value) parts.push(`${warningCount.value} 项需注意`)
  const reclaim = report.value.reclaimableMB ? `，涉及约 ${fmtMB(report.value.reclaimableMB)}` : ''
  return `发现 ${parts.join('、')}${reclaim}`
})

async function copyText(text, tip) {
  if (await writeClipboard(text)) ElMessage.success(tip || '已复制')
  else ElMessage.error('复制失败，请手动选中复制')
}

// 路径拆成「文件名」+「所在目录」两行展示：
// 超长路径若整条塞进一行，即使用省略号也会让用户看不到到底是哪个文件
function baseName(p) {
  const s = String(p || '')
  const i = s.lastIndexOf('/')
  return i >= 0 ? s.slice(i + 1) || s : s
}
function dirName(p) {
  const s = String(p || '')
  const i = s.lastIndexOf('/')
  return i > 0 ? s.slice(0, i) : ''
}

function copyCommand(f) {
  copyText(f.command, '命令已复制（本页不会执行它，请到 SSH 终端里粘贴运行）')
}

// 把整份报告导成 Markdown，方便贴到工单/群里
function buildReportMarkdown() {
  const r = report.value
  if (!r) return ''
  const lines = []
  lines.push('# 磁盘空间检测报告')
  lines.push('')
  lines.push(`- 目标盘：${r.mount}（${r.device || '未知设备'}${r.fstype ? ', ' + r.fstype : ''}）`)
  lines.push(`- 容量：已用 ${fmtMB(r.usedMB)} / ${fmtMB(r.totalMB)}（${fmtPct(r.usage)}），可用 ${fmtMB(r.availMB)}`)
  if (r.inodeTotal > 0) lines.push(`- inode：${fmtPct(r.inodeUsage)}（${r.inodeUsed} / ${r.inodeTotal}）`)
  lines.push(`- 一级目录扫描合计：${fmtMB(r.scannedMB)}，未解释差额：${fmtMB(r.gapMB)}`)
  lines.push(`- 检测时间：${r.scannedAt}，耗时 ${((r.elapsedMs || 0) / 1000).toFixed(1)}s${r.partial ? '（结果不完整）' : ''}`)
  lines.push('')
  lines.push('## 结论')
  if (!r.findings.length) lines.push('没有发现明显问题。')
  r.findings.forEach((f, i) => {
    lines.push('')
    lines.push(`### ${i + 1}. [${levelMeta(f.level).label}] ${f.title}`)
    lines.push(f.detail || '')
    if (f.command) {
      lines.push('', '```bash', f.command, '```')
      if (f.commandNote) lines.push('', `> ${f.commandNote}`)
    }
    if (f.evidence) lines.push('', '原始输出：', '```', f.evidence, '```')
  })
  if (r.topDirs && r.topDirs.length) {
    lines.push('', '## 目录占用 TOP')
    r.topDirs.forEach(d => lines.push(`- ${d.path} — ${fmtMB(d.sizeMB)}（${fmtPct(pctOf(d))}）`))
  }
  if (r.bigFiles && r.bigFiles.length) {
    lines.push('', '## 大文件')
    r.bigFiles.forEach(d => lines.push(`- ${fmtMB(d.sizeMB)}  ${d.path}`))
  }
  if (r.notes && r.notes.length) {
    lines.push('', '## 备注')
    r.notes.forEach(n => lines.push(`- ${n}`))
  }
  lines.push('', '> 由 Xx1ServerManager2「运维工具 · 磁盘空间检测」生成（仅分析，未做任何修改）')
  return lines.join('\n')
}

onMounted(loadDisks)

onBeforeUnmount(() => {
  unsubscribeAll()
  stopDrillTimer()
  // 切走/换服务器时把还在跑的任务停掉，避免留下没人看的后台扫描
  if (running.value && taskId.value) api.cancelDiskAnalysis(taskId.value).catch(() => {})
})
</script>

<template>
  <div class="disk-tool">
    <!-- 操作条 -->
    <div class="card mb-12">
      <div class="dt-bar">
        <span class="dt-label">磁盘</span>
        <el-select v-model="mount" :loading="loadingDisks" style="width: 320px" placeholder="选择要检测的磁盘">
          <el-option v-for="d in diskOptions" :key="d.value" :label="d.label" :value="d.value" />
        </el-select>
        <el-checkbox v-model="useSudo" class="dt-sudo">
          以 root 权限扫描
        </el-checkbox>
        <el-tooltip content="普通用户读不到的目录不会被 du 统计，勾选后用服务器密码执行 sudo 重扫" placement="top">
          <el-icon class="dt-help"><InfoFilled /></el-icon>
        </el-tooltip>
        <div class="dt-actions">
          <el-button v-if="running" @click="cancel">
            <el-icon><Close /></el-icon>&nbsp;取消
          </el-button>
          <el-button v-else type="primary" :disabled="loadingDisks || !!disksError || !!drillTarget" @click="start">
            <el-icon><VideoPlay /></el-icon>&nbsp;开始检测
          </el-button>
          <el-button text @click="loadDisks" :loading="loadingDisks">
            <el-icon><Refresh /></el-icon>&nbsp;刷新磁盘
          </el-button>
          <el-button text @click="openTerminal">
            <el-icon><Monitor /></el-icon>&nbsp;打开终端
          </el-button>
        </div>
      </div>
      <div v-if="disksError" class="dt-error">{{ disksError }}</div>
      <div v-else-if="running" class="dt-progress">
        <el-icon class="dt-spin"><Loading /></el-icon>
        {{ progressText || '分析中…' }}
      </div>
      <div v-else-if="progressText" class="text-muted dt-progress-plain">{{ progressText }}</div>
    </div>

    <div v-if="errorText" class="card mb-12 dt-error-card">
      <el-icon><WarningFilled /></el-icon>
      <div>
        <div class="dt-error-title">检测失败</div>
        <div class="mono dt-error-body">{{ errorText }}</div>
      </div>
    </div>

    <div v-if="!report && !errorText && !running" class="card">
      <el-empty description="点「开始检测」后，3 秒内先给出容量、inode、日志与已删除句柄的结论，目录占用排行随后到达" :image-size="80" />
    </div>

    <template v-if="report">
      <!-- 结论摘要 -->
      <div class="card mb-12">
        <div class="dt-sum-head">
          <div>
            <div class="dt-sum-title">{{ headline }}</div>
            <div class="text-muted dt-sum-sub">
              {{ report.mount }} · {{ report.device || '未知设备' }}{{ report.fstype ? ' · ' + report.fstype : '' }} ·
              检测于 {{ report.scannedAt }}（{{ ((report.elapsedMs || 0) / 1000).toFixed(1) }}s）
              <span v-if="report.partial" class="dt-partial">· 结果不完整</span>
            </div>
          </div>
          <div class="dt-sum-actions">
            <el-button text size="small" @click="copyText(buildReportMarkdown(), '报告已复制为 Markdown')">
              <el-icon><CopyDocument /></el-icon>&nbsp;复制报告
            </el-button>
          </div>
        </div>

        <div class="dt-metrics">
          <div class="dt-metric">
            <div class="dt-metric-label">空间使用</div>
            <div class="dt-metric-value" :style="{color: usageColor(report.usage)}">{{ fmtPct(report.usage) }}</div>
            <div class="dt-metric-sub">已用 {{ fmtMB(report.usedMB) }} / {{ fmtMB(report.totalMB) }}，可用 {{ fmtMB(report.availMB) }}</div>
          </div>
          <div class="dt-metric">
            <div class="dt-metric-label">inode 使用</div>
            <div class="dt-metric-value" :style="{color: report.inodeTotal ? usageColor(report.inodeUsage) : 'var(--ink-3)'}">
              {{ report.inodeTotal ? fmtPct(report.inodeUsage) : '不可用' }}
            </div>
            <div class="dt-metric-sub">
              {{ report.inodeTotal ? `${report.inodeUsed} / ${report.inodeTotal}` : '该文件系统不统计 inode' }}
            </div>
          </div>
          <div class="dt-metric">
            <div class="dt-metric-label">目录扫描到</div>
            <div class="dt-metric-value">{{ fmtMB(report.scannedMB) }}</div>
            <div class="dt-metric-sub">未解释差额 {{ fmtMB(report.gapMB) }}</div>
          </div>
          <div class="dt-metric">
            <div class="dt-metric-label">涉及体积合计</div>
            <div class="dt-metric-value" :style="{color: report.reclaimableMB ? 'var(--red)' : 'var(--green)'}">
              {{ fmtMB(report.reclaimableMB) }}
            </div>
            <div class="dt-metric-sub">报告中标注可处理的体积</div>
          </div>
        </div>

        <div v-if="noticeText" class="dt-notice">{{ noticeText }}</div>
        <div v-if="report.notes && report.notes.length" class="dt-notes">
          <div v-for="(n, i) in report.notes" :key="i" class="text-muted">· {{ n }}</div>
        </div>
      </div>

      <!-- 发现清单 -->
      <div class="card mb-12">
        <div class="dt-section-title">发现（{{ report.findings.length }}）</div>
        <div v-if="!report.findings.length" class="text-muted dt-empty-line">没有发现明显问题。</div>
        <div v-for="(f, i) in report.findings" :key="i" class="dt-finding" :style="{'--lv': levelMeta(f.level).color, '--lv-bg': levelMeta(f.level).bg}">
          <div class="dt-finding-head">
            <span class="dt-lv">{{ levelMeta(f.level).label }}</span>
            <span class="dt-kind">{{ kindLabel(f.kind) }}</span>
            <span class="dt-finding-title">{{ f.title }}</span>
            <span v-if="f.sizeMB" class="dt-finding-size">{{ fmtMB(f.sizeMB) }}</span>
          </div>
          <div class="dt-finding-detail">{{ f.detail }}</div>
          <div class="dt-finding-foot">
            <details v-if="f.evidence" class="dt-evidence">
              <summary>查看原始输出</summary>
              <pre class="mono">{{ f.evidence }}</pre>
            </details>
            <div v-if="f.command" class="dt-cmd">
              <div class="dt-cmd-row">
                <code class="mono">{{ f.command }}</code>
                <el-button text size="small" @click="copyCommand(f)">
                  <el-icon><CopyDocument /></el-icon>&nbsp;复制命令
                </el-button>
              </div>
              <!-- 命令只复制不执行，所以必须讲清它做什么、跑完会怎样 -->
              <div v-if="f.commandNote" class="dt-cmd-note">
                <el-icon class="dt-cmd-note-icon"><InfoFilled /></el-icon>
                <span>{{ f.commandNote }}</span>
              </div>
            </div>
            <el-button
              v-if="f.kind === 'permission' && !useSudo"
              text
              size="small"
              type="primary"
              @click="rerunAsRoot"
            >用 root 重跑</el-button>
          </div>
        </div>
      </div>

      <!-- 目录占用 + 下钻 -->
      <div class="card mb-12">
        <div class="dt-section-title">
          目录占用
          <span class="text-muted dt-section-sub">
            {{ currentPath }}
            <template v-if="!drillStack.length">（一级目录）</template>
          </span>
        </div>
        <div class="dt-crumb">
          <el-button text size="small" @click="drillRoot">磁盘根目录</el-button>
          <template v-for="(lv, i) in drillStack" :key="lv.path">
            <span class="dt-crumb-sep">/</span>
            <el-button text size="small" :disabled="i === drillStack.length - 1" @click="drillTo(i)">
              {{ lv.path.split('/').filter(Boolean).pop() || lv.path }}
            </el-button>
          </template>
          <el-button v-if="drillStack.length" text size="small" class="dt-crumb-up" @click="drillUp">
            <el-icon><ArrowLeft /></el-icon>&nbsp;上一层
          </el-button>
        </div>
        <!-- du 超时中断时，列表只是"统计到的部分"，必须明说，否则会被当成完整结果 -->
        <div v-if="currentLevel && currentLevel.partial" class="dt-partial-note">
          <el-icon><WarningFilled /></el-icon>
          <span>{{ currentLevel.note }}</span>
        </div>
        <div v-if="!currentDirs.length" class="text-muted dt-empty-line">
          没有子项（或该目录没有读权限）。可以勾选「以 root 权限扫描」后重试。
        </div>
        <!-- 局部加载：包住列表本身，遮罩只在这块区域内，页面其余部分照常可操作 -->
        <div v-else class="dt-dirs-wrap" :class="{busy: !!drillTarget}">
          <div class="dt-dirs">
            <div v-for="d in currentDirs" :key="d.path" class="dt-dir" @click="drill(d)">
              <div class="dt-dir-main">
                <div class="dt-dir-path" :title="d.path">{{ d.path }}</div>
                <div class="dt-dir-bar">
                  <i :style="{width: Math.min(100, d.percent) + '%', background: d.percent >= 30 ? 'var(--red)' : 'var(--blue)'}" />
                </div>
              </div>
              <div class="dt-dir-size">
                {{ fmtMB(d.sizeMB) }}
                <span class="text-muted dt-dir-pct">{{ fmtPct(d.percent) }}</span>
              </div>
              <el-icon class="dt-dir-go" :class="{busy: !!drillTarget}"><ArrowRight /></el-icon>
            </div>
          </div>
          <div v-if="drillTarget" class="dt-dirs-loading">
            <el-icon class="dt-spin dt-dirs-loading-icon"><Loading /></el-icon>
            <div class="dt-dirs-loading-title">正在统计该目录的体积…</div>
            <div class="dt-dirs-loading-path mono" :title="drillTarget">{{ drillTarget }}</div>
            <div class="dt-dirs-loading-sub">
              已用 {{ drillElapsed }}s · 目录越大越慢，通常十几秒内返回
            </div>
          </div>
        </div>
        <div class="text-muted dt-hint">点任意一行进入下一层；每层的体积都是该目录的合计值</div>
      </div>

      <!-- 大文件 -->
      <div v-if="report.bigFiles && report.bigFiles.length" class="card mb-12">
        <div class="dt-section-title">大文件（{{ report.bigFiles.length }}）</div>
        <div v-for="f in report.bigFiles" :key="f.path" class="dt-file">
          <span class="dt-file-size">{{ fmtMB(f.sizeMB) }}</span>
          <div class="dt-file-main" :title="f.path">
            <!-- 文件名单独一行、目录另起一行并省略：超长路径不会再顶出可视区 -->
            <div class="dt-file-name">{{ baseName(f.path) }}</div>
            <div class="dt-file-dir mono">{{ dirName(f.path) }}</div>
          </div>
          <el-button text size="small" class="dt-file-copy" @click="copyText(f.path, '完整路径已复制')">
            <el-icon><CopyDocument /></el-icon>
          </el-button>
        </div>
        <div class="text-muted dt-hint">
          本页只分析不处置。日志类文件建议用 <code class="mono">truncate -s 0 文件</code> 清空（不要直接 rm，
          rm 之后写入进程仍持有句柄、空间不会释放）；其他文件请先确认用途再处理。
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.disk-tool {
  min-width: 0;
}
.dt-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.dt-label {
  font-size: 13px;
  color: var(--ink-2);
}
.dt-sudo {
  margin-left: 4px;
}
.dt-help {
  color: var(--ink-3);
  cursor: help;
}
.dt-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
}
.dt-progress,
.dt-progress-plain {
  margin-top: 10px;
  font-size: 12px;
  display: flex;
  align-items: center;
  gap: 6px;
}
.dt-progress {
  color: var(--blue-deep);
}
.dt-spin {
  animation: dt-rotate 1.1s linear infinite;
}
@keyframes dt-rotate {
  to { transform: rotate(360deg); }
}
.dt-error {
  margin-top: 8px;
  font-size: 12px;
  color: var(--red);
}
.dt-error-card {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  color: var(--red);
}
.dt-error-title {
  font-size: 13px;
  font-weight: 600;
}
.dt-error-body {
  margin-top: 4px;
  font-size: 12px;
}
.dt-sum-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.dt-sum-title {
  font-size: 17px;
  font-weight: 700;
  color: var(--ink);
}
.dt-sum-sub {
  font-size: 12px;
  margin-top: 4px;
}
.dt-partial {
  color: var(--yellow);
}
.dt-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  margin-top: 12px;
}
.dt-metric {
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 10px 12px;
  min-width: 0;
}
.dt-metric-label {
  font-size: 11px;
  color: var(--ink-3);
}
.dt-metric-value {
  font-size: 18px;
  font-weight: 700;
  margin: 2px 0;
}
.dt-metric-sub {
  font-size: 11px;
  color: var(--ink-3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dt-notice {
  margin-top: 10px;
  font-size: 12px;
  color: var(--yellow);
}
.dt-notes {
  margin-top: 8px;
  font-size: 12px;
  line-height: 1.6;
}
.dt-section-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--ink);
  margin-bottom: 8px;
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.dt-section-sub {
  font-size: 12px;
  font-weight: 400;
}
.dt-empty-line {
  font-size: 12px;
  padding: 8px 0;
}
.dt-finding {
  border-left: 3px solid var(--lv);
  background: var(--lv-bg);
  border-radius: 10px;
  padding: 10px 12px;
  margin-bottom: 8px;
}
.dt-finding-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.dt-lv {
  font-size: 11px;
  font-weight: 700;
  color: #fff;
  background: var(--lv);
  border-radius: 6px;
  padding: 1px 7px;
  flex: 0 0 auto;
}
.dt-kind {
  font-size: 11px;
  color: var(--ink-2);
  border: 1px solid var(--line);
  border-radius: 999px;
  padding: 0 7px;
  flex: 0 0 auto;
}
/* 标题里可能带长路径：必须允许换行 + 任意位置断行，否则会把卡片顶出可视区
   （flex-wrap 只在子项之间换行，不会在子项内部断开） */
.dt-finding-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink);
  min-width: 0;
  flex: 1 1 auto;
  overflow-wrap: anywhere;
  word-break: break-word;
}
.dt-finding-size {
  flex: 0 0 auto;
  font-size: 12px;
  color: var(--ink-2);
  font-weight: 600;
}
.dt-finding-detail {
  font-size: 12px;
  color: var(--ink-2);
  line-height: 1.7;
  margin-top: 6px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  word-break: break-word;
}
.dt-finding-foot {
  margin-top: 8px;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.dt-evidence summary {
  font-size: 12px;
  color: var(--ink-3);
  cursor: pointer;
}
.dt-evidence pre {
  margin: 6px 0 0;
  padding: 8px;
  background: var(--terminal-bg);
  color: var(--terminal-fg);
  border-radius: 8px;
  max-height: 220px;
  overflow: auto;
  font-size: 11px;
}
.dt-cmd {
  display: flex;
  flex-direction: column;
  gap: 4px;
  flex: 1 1 100%;
  min-width: 0;
}
.dt-cmd-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.dt-cmd-row code {
  flex: 1 1 auto;
  min-width: 0;
  background: var(--card);
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 4px 8px;
  overflow-x: auto;
  white-space: pre;
  font-size: 11.5px;
}
/* 命令说明：这段是给"看不懂命令的人"看的，不能省略，允许换行 */
.dt-cmd-note {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 11.5px;
  line-height: 1.75;
  color: var(--ink-2);
  background: var(--card);
  border: 1px dashed var(--line);
  border-radius: 8px;
  padding: 6px 8px;
}
.dt-cmd-note-icon {
  flex: 0 0 auto;
  margin-top: 3px;
  color: var(--blue);
}
.dt-crumb {
  display: flex;
  align-items: center;
  gap: 2px;
  flex-wrap: wrap;
  margin-bottom: 6px;
}
.dt-crumb-sep {
  color: var(--ink-3);
}
.dt-crumb-up {
  margin-left: 6px;
}
.dt-partial-note {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin-bottom: 6px;
  padding: 6px 8px;
  font-size: 11.5px;
  line-height: 1.7;
  color: var(--yellow);
  background: var(--yellow-soft);
  border-radius: 8px;
}
.dt-dirs-wrap {
  position: relative;
  min-height: 64px;
}
/* 扫描中：旧的一层留在原地并压暗，给用户"我在这一层往下钻"的上下文 */
.dt-dirs-wrap.busy .dt-dirs {
  opacity: .42;
  pointer-events: none;
  transition: opacity .2s ease;
}
/* 局部遮罩：只覆盖目录列表区域（absolute 相对 .dt-dirs-wrap），不遮整页 */
.dt-dirs-loading {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 5px;
  padding: 10px;
  text-align: center;
  border-radius: 10px;
  background: var(--card);
  opacity: .94;
}
.dt-dirs-loading-icon {
  font-size: 22px;
  color: var(--blue-deep);
}
.dt-dirs-loading-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink);
}
.dt-dirs-loading-path {
  max-width: 100%;
  font-size: 11.5px;
  color: var(--ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dt-dirs-loading-sub {
  font-size: 11px;
  color: var(--ink-3);
}
.dt-dirs {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.dt-dir {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 8px;
  border-radius: 8px;
  cursor: pointer;
  transition: background-color .15s ease;
}
.dt-dir:hover {
  background: var(--hover-bg);
}
.dt-dir-main {
  flex: 1 1 auto;
  min-width: 0;
}
.dt-dir-path {
  font-size: 12px;
  color: var(--ink);
  font-family: 'Cascadia Code', Consolas, monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dt-dir-bar {
  height: 4px;
  background: var(--line);
  border-radius: 999px;
  margin-top: 4px;
  overflow: hidden;
}
.dt-dir-bar i {
  display: block;
  height: 100%;
  border-radius: 999px;
  transition: width .3s ease;
}
.dt-dir-size {
  flex: 0 0 auto;
  font-size: 12px;
  font-weight: 600;
  color: var(--ink);
  text-align: right;
}
.dt-dir-pct {
  font-weight: 400;
  margin-left: 4px;
}
.dt-dir-go {
  flex: 0 0 auto;
  color: var(--ink-3);
}
.dt-dir-go.busy {
  opacity: .4;
}
.dt-file {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 8px;
  border-radius: 8px;
}
.dt-file:hover {
  background: var(--hover-bg);
}
.dt-file-size {
  flex: 0 0 auto;
  font-size: 12px;
  font-weight: 600;
  color: var(--red);
  min-width: 74px;
}
.dt-file-main {
  flex: 1 1 auto;
  min-width: 0;
}
/* 文件名整行显示（可断行），目录单行省略——长路径也不会把复制按钮挤出屏幕 */
.dt-file-name {
  font-size: 12px;
  color: var(--ink);
  overflow-wrap: anywhere;
  word-break: break-word;
  line-height: 1.5;
}
.dt-file-dir {
  font-size: 11px;
  color: var(--ink-3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.5;
}
.dt-file-copy {
  flex: 0 0 auto;
}
.dt-hint {
  font-size: 11px;
  line-height: 1.7;
  margin-top: 6px;
}
@media (max-width: 1200px) {
  .dt-metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>