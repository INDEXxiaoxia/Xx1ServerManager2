import {createRouter, createWebHashHistory} from 'vue-router'
import Layout from '@/views/Layout.vue'

const routes = [
  {
    path: '/',
    component: Layout,
    redirect: '/dashboard',
    children: [
      {path: 'dashboard', name: 'Dashboard', component: () => import('@/views/dashboard/Index.vue'), meta: {title: '总览'}},
      {path: 'servers', name: 'ServerList', component: () => import('@/views/servers/List.vue'), meta: {title: '服务器列表'}},
      {path: 'monitor', name: 'Monitor', component: () => import('@/views/servers/Monitor.vue'), meta: {title: '实时监控'}},
      {path: 'files', name: 'FileManager', component: () => import('@/views/files/Manager.vue'), meta: {title: '文件管理'}},
      {path: 'upload', name: 'FileUpload', component: () => import('@/views/files/Upload.vue'), meta: {title: '批量上传'}},
      {path: 'execute', name: 'CommandExecute', component: () => import('@/views/commands/Execute.vue'), meta: {title: '批量执行'}},
      {path: 'services', name: 'ServiceManage', component: () => import('@/views/services/Index.vue'), meta: {title: '服务管理'}},
      {path: 'terminal', name: 'Terminal', component: () => import('@/views/terminal/Index.vue'), meta: {title: 'SSH 终端'}},
      {path: 'groups', name: 'GroupManage', component: () => import('@/views/groups/Index.vue'), meta: {title: '分组管理'}},
      {path: 'presets', name: 'PresetManage', component: () => import('@/views/presets/Index.vue'), meta: {title: '预设管理'}},
      {path: 'settings', name: 'Settings', component: () => import('@/views/settings/Index.vue'), meta: {title: '设置'}}
    ]
  }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

export default router