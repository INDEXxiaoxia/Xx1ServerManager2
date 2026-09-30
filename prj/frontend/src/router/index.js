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
      {
        // 批量上传与批量执行合并为一个菜单：左侧服务器选择 + 右侧 tab 切换
        // tab 是页面内状态（不切路由），这样切换 tab 不会重建组件、任务状态不丢
        path: 'batch',
        name: 'Batch',
        component: () => import('@/views/batch/Index.vue'),
        meta: {title: '批量操作'}
      },
      // 兼容旧地址
      {path: 'upload', redirect: '/batch'},
      {path: 'execute', redirect: '/batch'},
      {
        // 运维工具：磁盘空间检测等排查小工具（只分析、不处置）
        path: 'ops',
        name: 'OpsTools',
        component: () => import('@/views/ops/Index.vue'),
        meta: {title: '运维工具'}
      },
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