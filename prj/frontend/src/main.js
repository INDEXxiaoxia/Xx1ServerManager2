import {createApp} from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import {createPinia} from 'pinia'
import App from './App.vue'
import router from './router'
import './style.css'

// 全局错误可见化：任何页面渲染出错时，在页面顶部显示红色错误条，
// 方便在没有 DevTools 的 Wails 生产环境直接看到错误文本
function formatError(err, info) {
  const msg = err && err.message ? err.message : String(err)
  const stack = err && err.stack ? String(err.stack).split('\n').slice(0, 3).join('\n') : ''
  return `${msg}\n${info ? '[' + info + '] ' : ''}${stack}`.trim()
}

function showErrorBar(text) {
  const id = 'app-error-bar'
  let bar = document.getElementById(id)
  if (!bar) {
    bar = document.createElement('div')
    bar.id = id
    bar.style.cssText =
      'position:fixed;top:0;left:0;right:0;z-index:99999;background:#f56c6c;' +
      'color:#fff;font:12px/1.5 Consolas,Menlo,monospace;padding:8px 12px;' +
      'white-space:pre-wrap;word-break:break-all;cursor:pointer;'
    bar.addEventListener('click', () => bar.remove())
    document.body.appendChild(bar)
  }
  bar.textContent = '点击关闭 | ' + text
  console.error('[App Error]', text)
}

const app = createApp(App)

app.config.errorHandler = (err, instance, info) => {
  formatError(err, info)
  showErrorBar(formatError(err, info))
}

window.addEventListener('error', (e) => {
  showErrorBar(formatError(e.error || e.message, 'window.onerror'))
})
window.addEventListener('unhandledrejection', (e) => {
  showErrorBar(formatError(e.reason, 'unhandledrejection'))
})

for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(ElementPlus, {locale: zhCn})
app.use(createPinia())
app.use(router)
app.mount('#app')