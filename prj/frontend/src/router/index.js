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
        // 批量上传与批量执行合并为一个菜单，内部用 tab 切换
        path: 'batch',
        component: () => import('@/views/batch/Index.vue'),
        redirect: '/batch/upload',
        meta: {title: '批量操作'},
        children: [
          {
            path: 'upload',
            name: 'BatchUpload',
            component: () => import('@/views/files/Upload.vue')
            // 不覆盖 meta.title，标题栏统一显示父级「批量操作」
          },
          {
            path: 'execute',
            name: 'BatchExecute',
            component: () => import('@/views/commands/Execute.vue')
          }
        ]
      },
      // 兼容旧地址（含书签与批量页面之间的跳转）
      {path: 'upload', redirect: '/batch/upload'},
      {path: 'execute', redirect: '/batch/execute'},
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