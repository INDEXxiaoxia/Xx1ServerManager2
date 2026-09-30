<script setup>
import {ref, computed, onMounted, onActivated} from 'vue'
import {useRoute, useRouter} from 'vue-router'
import {ElMessage} from 'element-plus'
import {api} from '@/api'
import DiskTool from './DiskTool.vue'

// 组件名供 Layout 的 <keep-alive :include> 匹配：切到其他页面时本页不销毁，
// 正在跑的磁盘检测（后端 goroutine + 事件订阅）不会中断，切回来即原样恢复。
defineOptions({name: 'OpsPage'})

const route = useRoute()
const router = useRouter()
const servers = ref([])
const serverId = ref('')
// 当前打开的工具；空串 = 工具卡片列表（首页）
const activeTool = ref('')

// 工具清单：以后新增"妙妙小工具"只要往这里加一项 + 挂一个组件即可。
// features 用一句话讲清"这个工具能帮你做什么"，卡片上直接展示。
const tools = [
  {
    key: 'disk',
    name: '磁盘空间检测',
    desc: '磁盘满了，先找出是谁占的——含 du 找不到大头的那几种情况',
    icon: 'DataAnalysis',
    ready: true,
    features: [
      '容量 / inode / 已删除句柄 一次扫完',
      '目录逐层下钻到具体文件',
      '结论与处置命令可一键复制'
    ]
  },
  {
    key: 'mem',
    name: '内存与进程',
    desc: '内存去哪了、谁在吃 CPU，以及有没有泄漏趋势',
    icon: 'Cpu',
    ready: false,
    features: ['按内存/CPU 排序的进程 TOP', '单进程增长趋势', 'OOM 杀手记录']
  },
  {
    key: 'logs',
    name: '日志体积分析',
    desc: '按服务统计日志增长，看看轮转策略是不是失效了',
    icon: 'Document',
    ready: false,
    features: ['逐服务日志体积排行', '增长速率', '轮转配置检查']
  },
  {
    key: 'health',
    name: '服务器体检',
    desc: '一条命令看清这台机器的整体健康度',
    icon: 'FirstAidKit',
    ready: false,
    features: ['负载 / 失败服务 / 重启历史', '时间同步与证书到期', '内核参数与连接数']
  }
]

const LAST_SERVER_KEY = 'xx1.ops.serverId'

const activeToolMeta = computed(() => tools.find(t => t.key === activeTool.value) || {name: '', desc: ''})
const currentServer = computed(() => servers.value.find(s => s.id === serverId.value) || null)

async function loadServers() {
  servers.value = await api.listServers()
  if (!servers.value.length) {
    serverId.value = ''
    return
  }
  if (!servers.value.some(s => s.id === serverId.value)) {
    const last = localStorage.getItem(LAST_SERVER_KEY)
    serverId.value = servers.value.some(s => s.id === last) ? last : servers.value[0].id
  }
}

// 从服务器列表的「监测运维」进来时带上 serverId 与 tool，直接进对应工具
function applyQuery() {
  const sid = route.query.serverId
  if (sid && servers.value.some(s => s.id === sid)) serverId.value = sid
  const tool = route.query.tool
  if (tool && tools.some(t => t.key === tool && t.ready)) activeTool.value = tool
}

// 把"当前在哪个工具"写进地址栏：从服务器列表再次进来、或切页回来时能还原
function syncQuery() {
  if (activeTool.value) {
    router.replace({path: '/ops', query: {tool: activeTool.value, serverId: serverId.value}})
  } else {
    router.replace({path: '/ops'})
  }
}

function openTool(t) {
  if (!t.ready) {
    ElMessage.info(`「${t.name}」还在路上，先把磁盘空间检测用起来`)
    return
  }
  activeTool.value = t.key
  syncQuery()
}

function backHome() {
  activeTool.value = ''
  syncQuery()
}

function onServerChange(v) {
  if (v) localStorage.setItem(LAST_SERVER_KEY, v)
  syncQuery()
}

onMounted(async () => {
  await loadServers()
  applyQuery()
  if (serverId.value) localStorage.setItem(LAST_SERVER_KEY, serverId.value)
})

// 从别处切回来：服务器列表可能变了，query 也可能带了新的服务器/工具
onActivated(async () => {
  await loadServers()
  applyQuery()
})
</script>

<template>
  <div class="page-container ops-page">
    <!-- ① 工具卡片列表 -->
    <template v-if="!activeTool">
      <div class="ops-hero">
        <h2 class="ops-hero-title">运维工具</h2>
        <p class="ops-hero-sub">
          选一个工具开始排查。<b>本模块只做分析</b>，不会修改或删除服务器上的任何文件。
        </p>
      </div>

      <div class="ops-grid">
        <div
          v-for="t in tools"
          :key="t.key"
          class="tool-card"
          :class="{'is-ready': t.ready}"
          @click="openTool(t)"
        >
          <i class="tc-glow" />
          <div class="tc-head">
            <div class="tc-icon"><el-icon><component :is="t.icon" /></el-icon></div>
            <span class="tc-pill" :class="{on: t.ready}">{{ t.ready ? '可用' : '即将上线' }}</span>
          </div>
          <div class="tc-name">{{ t.name }}</div>
          <div class="tc-desc">{{ t.desc }}</div>
          <ul class="tc-feats">
            <li v-for="f in t.features" :key="f"><i class="tc-dot" />{{ f }}</li>
          </ul>
          <div class="tc-foot">
            <span v-if="t.ready" class="tc-go">开始使用<el-icon><Right /></el-icon></span>
            <span v-else class="tc-go muted">开发中</span>
          </div>
        </div>
      </div>

      <div v-if="!servers.length" class="card mt-12">
        <el-empty description="还没有服务器，请先在「服务器列表」里添加" :image-size="70" />
      </div>
    </template>

    <!-- ② 单个工具 -->
    <template v-else>
      <div class="card mb-12 ops-toolbar">
        <el-button text class="ops-back" @click="backHome">
          <el-icon><ArrowLeft /></el-icon>&nbsp;返回工具列表
        </el-button>
        <i class="ops-divider" />
        <span class="ops-tool-title">{{ activeToolMeta.name }}</span>
        <span class="text-muted ops-tool-sub">{{ activeToolMeta.desc }}</span>
        <div class="ops-toolbar-right">
          <span class="ops-label">服务器</span>
          <el-select
            v-model="serverId"
            placeholder="选择要排查的服务器"
            style="width: 240px"
            filterable
            size="default"
            @change="onServerChange"
          >
            <el-option v-for="s in servers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
          </el-select>
          <span v-if="currentServer" class="text-muted mono ops-host">{{ currentServer.username }}@{{ currentServer.host }}</span>
        </div>
      </div>

      <DiskTool v-if="activeTool === 'disk' && serverId" :key="serverId" :server-id="serverId" />
      <div v-else class="card">
        <el-empty description="请先选择一台服务器" :image-size="80" />
      </div>
    </template>
  </div>
</template>

<style scoped>
.ops-page {
  min-width: 0;
}

/* ---------- 首页：标题 ---------- */
.ops-hero {
  padding: 4px 2px 14px;
}
.ops-hero-title {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  color: var(--ink);
  letter-spacing: .5px;
}
.ops-hero-sub {
  margin: 6px 0 0;
  font-size: 12.5px;
  color: var(--ink-3);
  line-height: 1.6;
}
.ops-hero-sub b {
  color: var(--blue-deep);
  font-weight: 600;
}

/* ---------- 工具卡片 ---------- */
.ops-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(276px, 1fr));
  gap: 14px;
}
.tool-card {
  position: relative;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  min-height: 208px;
  padding: 16px 16px 8px;
  border-radius: 16px;
  background: var(--card);
  border: 1px solid var(--card-border);
  box-shadow: var(--card-shadow);
  transition: transform .28s cubic-bezier(.16, 1, .3, 1),
              box-shadow .28s ease,
              border-color .28s ease;
}
/* 顶部渐变高光条：与终端标签的选中条同一套视觉语言 */
.tool-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 2px;
  background: linear-gradient(90deg, var(--blue), var(--pink) 55%, transparent);
  opacity: 0;
  transition: opacity .28s ease;
}
/* 右上角光斑：用主题内的 halo 令牌，浅/深/赛博三套自动适配 */
.tc-glow {
  position: absolute;
  top: -46px;
  right: -46px;
  width: 150px;
  height: 150px;
  border-radius: 50%;
  background: radial-gradient(circle, var(--halo-1), transparent 68%);
  pointer-events: none;
}
.tool-card.is-ready {
  cursor: pointer;
}
.tool-card.is-ready:hover {
  transform: translateY(-3px);
  box-shadow: var(--card-shadow-hover);
  border-color: var(--blue);
}
.tool-card.is-ready:hover::before {
  opacity: 1;
}
.tool-card:not(.is-ready) {
  opacity: .74;
}
.tc-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.tc-icon {
  width: 42px;
  height: 42px;
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  border: 1px solid var(--line);
  background: linear-gradient(135deg, var(--blue-soft), var(--pink-soft));
  color: var(--blue-deep);
  font-size: 21px;
  transition: box-shadow .28s ease;
}
.tool-card.is-ready:hover .tc-icon {
  box-shadow: 0 0 0 3px var(--blue-focus);
}
.tc-pill {
  margin-left: auto;
  font-size: 10.5px;
  line-height: 18px;
  padding: 0 8px;
  border-radius: 999px;
  border: 1px solid var(--line);
  color: var(--ink-3);
}
.tc-pill.on {
  border-color: var(--green);
  color: var(--green);
  background: var(--green-soft);
}
.tc-name {
  margin-top: 12px;
  font-size: 15px;
  font-weight: 600;
  color: var(--ink);
}
.tool-card.is-ready:hover .tc-name {
  color: var(--blue-deep);
}
.tc-desc {
  margin-top: 5px;
  font-size: 12px;
  color: var(--ink-3);
  line-height: 1.65;
}
.tc-feats {
  flex: 1 1 auto;
  margin: 10px 0 0;
  padding: 0;
  list-style: none;
}
.tc-feats li {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  font-size: 11.5px;
  color: var(--ink-2);
  line-height: 1.9;
}
.tc-dot {
  flex: 0 0 auto;
  width: 4px;
  height: 4px;
  margin-top: 8px;
  border-radius: 50%;
  background: var(--blue);
}
.tc-foot {
  flex: 0 0 auto;
  display: flex;
  justify-content: flex-end;
  align-items: center;
  height: 30px;
  border-top: 1px solid var(--line);
  margin-top: 8px;
}
.tc-go {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 12px;
  font-weight: 600;
  color: var(--blue-deep);
}
.tc-go.muted {
  color: var(--ink-3);
  font-weight: 400;
}
.tc-go .el-icon {
  transition: transform .25s ease;
}
.tool-card.is-ready:hover .tc-go .el-icon {
  transform: translateX(3px);
}

/* ---------- 工具页顶栏 ---------- */
.ops-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.ops-back {
  flex: 0 0 auto;
}
.ops-divider {
  width: 1px;
  height: 16px;
  background: var(--line);
}
.ops-tool-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--ink);
}
.ops-tool-sub {
  font-size: 12px;
}
.ops-toolbar-right {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
}
.ops-label {
  font-size: 13px;
  color: var(--ink-2);
}
.ops-host {
  font-size: 11.5px;
}

/* 卡片动画在"减少动效"环境下全部关掉 */
@media (prefers-reduced-motion: reduce) {
  .tool-card,
  .tc-go .el-icon,
  .tc-icon {
    transition: none;
  }
  .tool-card.is-ready:hover {
    transform: none;
  }
  .tool-card.is-ready:hover .tc-go .el-icon {
    transform: none;
  }
}
@media (max-width: 720px) {
  .ops-grid {
    grid-template-columns: 1fr;
  }
  .ops-toolbar-right {
    margin-left: 0;
    width: 100%;
  }
}
</style>