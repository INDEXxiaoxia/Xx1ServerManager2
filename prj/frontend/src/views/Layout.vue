<script setup>
import {computed} from 'vue'
import {useRoute} from 'vue-router'
import {ElMessage} from 'element-plus'

const route = useRoute()

const menus = [
  {path: '/dashboard', title: '总览', icon: 'Odometer'},
  {path: '/servers', title: '服务器列表', icon: 'Connection'},
  {path: '/monitor', title: '实时监控', icon: 'DataLine'},
  {path: '/files', title: '文件管理', icon: 'Folder'},
  {path: '/upload', title: '批量上传', icon: 'Upload'},
  {path: '/execute', title: '批量执行', icon: 'ScaleToOriginal'},
  {path: '/services', title: '服务管理', icon: 'Setting'},
  {path: '/terminal', title: 'SSH 终端', icon: 'Monitor'},
  {path: '/groups', title: '分组管理', icon: 'FolderOpened'},
  {path: '/presets', title: '预设管理', icon: 'Collection'},
  {path: '/settings', title: '设置', icon: 'Tools'}
]

const activeMenu = computed(() => route.path)
const currentTitle = computed(() => route.meta.title || 'ServerManager2')

function lock() {
  ElMessage.info('锁定功能将在后续版本开放')
}
</script>

<template>
  <el-container class="layout">
    <el-aside width="220px" class="aside">
      <div class="logo">
        <h2>ServerManager2</h2>
      </div>
      <el-menu :default-active="activeMenu" router class="menu">
        <el-menu-item v-for="m in menus" :key="m.path" :index="m.path">
          <el-icon><component :is="m.icon" /></el-icon>
          <span>{{ m.title }}</span>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="header">
        <div class="header-title">{{ currentTitle }}</div>
        <div class="header-right">
          <el-button text @click="lock"><el-icon><Lock /></el-icon> 锁定</el-button>
        </div>
      </el-header>
      <el-main class="main">
        <router-view />
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
  background: #fff;
  border-right: 1px solid #ebeef5;
  display: flex;
  flex-direction: column;
}
.logo {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea, #764ba2);
}
.logo h2 {
  color: #fff;
  font-size: 18px;
  letter-spacing: 1px;
}
.menu {
  border: none;
  flex: 1;
  overflow-y: auto;
}
.menu :deep(.el-menu-item) {
  margin: 4px 8px;
  border-radius: 8px;
}
.menu :deep(.el-menu-item.is-active) {
  background: linear-gradient(135deg, #667eea, #764ba2);
  color: #fff;
}
.header {
  background: #fff;
  border-bottom: 1px solid #ebeef5;
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 60px;
}
.header-title {
  font-size: 18px;
  font-weight: 600;
  color: #4a4a4a;
}
.header-right {
  display: flex;
  align-items: center;
}
.main {
  background: #f5f7fa;
  padding: 0;
}
</style>