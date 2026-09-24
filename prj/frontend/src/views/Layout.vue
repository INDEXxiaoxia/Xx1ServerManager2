<script setup>
import {computed, ref, onMounted, onBeforeUnmount} from 'vue'
import {useRoute} from 'vue-router'
import {ElMessage} from 'element-plus'
import {useBatchStore} from '@/store/batch'

const route = useRoute()
const collapsed = ref(false)
const batchStore = useBatchStore()

// 需要保活的页面（按组件名匹配，名字由页面里的 defineOptions 声明）
// 只有 SSH 终端页需要：切走后会话与窗口状态继续保留，切回来即原样恢复。
const KEEP_ALIVE = ['TerminalPage']

// 批量任务的进度事件在这里统一监听：即使用户切到其他页面，
// 上传/执行进度也会持续记录，回到批量操作页不会出现状态断档
onMounted(() => {
  batchStore.bindEvents()
})
onBeforeUnmount(() => {
  batchStore.unbindEvents()
})

const menus = [
  {path: '/dashboard', title: '总览', icon: 'Odometer'},
  {path: '/servers', title: '服务器列表', icon: 'Connection'},
  {path: '/monitor', title: '实时监控', icon: 'DataLine'},
  {path: '/terminal', title: 'SSH 终端', icon: 'Monitor'},
  {path: '/files', title: '文件管理', icon: 'Folder'},
  {path: '/batch', title: '批量操作', icon: 'Upload'},
  {path: '/services', title: '服务管理', icon: 'Setting'},
  {path: '/groups', title: '分组管理', icon: 'FolderOpened'},
  {path: '/presets', title: '预设管理', icon: 'Collection'},
  {path: '/settings', title: '设置', icon: 'Tools'}
]

// 子路由（如 /batch/upload）也要高亮对应的父级菜单
const activeMenu = computed(() => route.path)
const currentTitle = computed(() => route.meta.title || 'Xx1ServerManager2')
const asideWidth = computed(() => (collapsed.value ? '64px' : '200px'))

function toggleCollapse() {
  collapsed.value = !collapsed.value
}

function lock() {
  ElMessage.info('锁定功能将在后续版本开放')
}
</script>

<template>
  <el-container class="layout">
    <el-aside :width="asideWidth" class="aside">
      <div class="logo">
        <h2 v-show="!collapsed">Xx1ServerManager2</h2>
        <span v-show="collapsed" class="logo-mini" title="Xx1ServerManager2">X2</span>
      </div>
      <el-menu :default-active="activeMenu" router class="menu" :collapse="collapsed" :collapse-transition="false">
        <el-menu-item v-for="m in menus" :key="m.path" :index="m.path" :title="m.title">
          <el-icon><component :is="m.icon" /></el-icon>
          <span>{{ m.title }}</span>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="header">
        <div class="header-left">
          <el-button text class="collapse-btn" @click="toggleCollapse" :title="collapsed ? '展开菜单' : '折叠菜单'">
            <el-icon><component :is="collapsed ? 'Expand' : 'Fold'" /></el-icon>
          </el-button>
          <div class="header-title">{{ currentTitle }}</div>
        </div>
        <div class="header-right">
          <el-button text @click="lock"><el-icon><Lock /></el-icon> 锁定</el-button>
        </div>
      </el-header>
      <el-main class="main">
        <!-- 常驻外壳：给页面根元素提供高度上下文（页面根用 height:100% 撑满可视区），
             它本身不参与过渡，也永远不携带 transform。 -->
        <div class="page-shell">
          <!-- keep-alive 只保活 SSH 终端页：切到其他页面时终端组件不销毁，
               会话、标签、输出、侧栏状态都留在原处，切回来即原样恢复。
               它必须放在 router-view 动态组件的外层、且自身常驻 —— 一旦让它跟着路由
               一起重建（例如塞进带 :key 的包裹 div 里），缓存会随之被销毁，页面照样被卸载。
               过渡只做 opacity：transform（哪怕 translateY(0)）会让元素成为后代
               position:fixed 元素的包含块，弹窗遮罩与终端右键菜单就会按"页面"而不是
               "视口"定位 —— 这正是右键菜单跑到窗口外面的成因，所以这里不再用位移动画。 -->
          <router-view v-slot="{ Component }">
            <transition name="page" mode="out-in">
              <keep-alive :include="KEEP_ALIVE">
                <component :is="Component" />
              </keep-alive>
            </transition>
          </router-view>
        </div>
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.layout {
  height: 100vh;
  width: 100vw;
  overflow: hidden;
}
.aside {
  background: var(--aside-bg);
  border-right: 1px solid var(--line);
  display: flex;
  flex-direction: column;
  transition: width .25s ease;
  overflow: hidden;
}
.logo {
  height: 50px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.logo h2 {
  font-size: 15px;
  font-weight: 700;
  letter-spacing: .6px;
  color: var(--ink);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.logo-mini {
  font-size: 15px;
  font-weight: 700;
  color: var(--blue-deep);
  letter-spacing: .5px;
}
.menu {
  border: none;
  flex: 1;
  overflow-y: auto;
  background: transparent;
  padding: 4px 0;
}
.menu :deep(.el-menu-item) {
  position: relative;
  margin: 3px 8px;
  padding-left: 16px !important;
  border-radius: 10px;
  color: var(--ink-2);
  transition: background-color .25s ease, color .25s ease;
}
.menu :deep(.el-menu-item:hover) {
  background: var(--hover-bg);
  color: var(--ink);
}
.menu :deep(.el-menu-item.is-active) {
  background: var(--blue-soft);
  color: var(--blue-deep);
  font-weight: 600;
}
.menu :deep(.el-menu-item.is-active::before) {
  content: '';
  position: absolute;
  left: 5px;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 16px;
  border-radius: 999px;
  background: var(--blue);
}
/* 折叠态：菜单项居中显示图标 */
.menu :deep(.el-menu--collapse .el-menu-item) {
  margin: 3px 8px;
  padding: 0 !important;
  justify-content: center;
}
.header {
  height: 50px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  background: var(--header-bg);
  border-bottom: 1px solid var(--line);
}
@supports (backdrop-filter: blur(12px)) {
  .header {
    background: var(--header-bg);
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
  }
}
.header-left {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.collapse-btn {
  width: 32px;
  height: 32px;
  padding: 0;
  border-radius: 8px;
}
.collapse-btn :deep(.el-icon) {
  font-size: 17px;
}
.collapse-btn:hover {
  background: var(--pink-soft);
}
.header-title {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 14px;
  font-weight: 600;
  color: var(--ink);
  letter-spacing: .3px;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}
.header-title::before {
  content: '';
  width: 3px;
  height: 14px;
  border-radius: 999px;
  background: linear-gradient(180deg, var(--blue), var(--pink));
}
.header-right {
  display: flex;
  align-items: center;
  flex-shrink: 0;
}
.header-right :deep(.el-button:hover) {
  background: var(--pink-soft);
}
.main {
  background: transparent;
  padding: 0;
  overflow-y: auto;
  /* 固定滚动条槽位：页面切换时内容宽度不跳动 */
  scrollbar-gutter: stable;
}

/* 页面过渡：如微风拂过 */
/* 只用 opacity：transform / filter 之类会让元素成为后代 position:fixed 元素的包含块，
   页面内的弹窗遮罩、下拉浮层、终端右键菜单都会改按"页面"定位而错位；
   opacity 只创建层叠上下文，不会改变 fixed 的定位基准，是这里唯一安全的选择。 */
.page-shell {
  /* 让页面根元素（height: 100%）仍能撑满可视区，不因多包一层而塌陷 */
  height: 100%;
}
.page-enter-active,
.page-leave-active {
  transition: opacity .3s ease;
}
.page-enter-from,
.page-leave-to {
  opacity: 0;
}
</style>