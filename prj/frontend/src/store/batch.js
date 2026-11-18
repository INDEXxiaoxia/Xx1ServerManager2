import {defineStore} from 'pinia'
import {events} from '@/api'

// 批量操作的共享状态：
// 1) 两个 tab 共用同一份「已选服务器」，切换 tab 不会丢选择；
// 2) 任务进度/输出保存在 store 里，离开页面再回来仍能看到此刻的任务状态。
// 事件监听挂在 store 上（由 Layout 启动一次），因此即使用户切到其他页面，
// 进度事件依然会被记录，回到批量操作页不会出现「进度断档」。
export const useBatchStore = defineStore('batch', {
  state: () => ({
    tab: 'upload',            // upload | execute
    selectedIds: [],          // 已选服务器 id（跨 tab 共享）

    uploadSessionId: '',
    uploadRunning: false,
    uploadTasks: {},          // key -> 上传任务（含 progress/status/errorMsg）

    executeSessionId: '',
    executeRunning: false,
    executeOutputs: {},       // serverId -> 执行结果
    executeCommand: '',       // 命令内容也保留，便于切页返回后继续编辑
    executeTimeout: 30,

    pendingUpload: false,
    earlyUpload: [],
    pendingExecute: false,
    earlyExecute: [],
    bound: false
  }),

  getters: {
    uploadList: (s) => Object.values(s.uploadTasks),
    executeList: (s) => Object.values(s.executeOutputs),
    executeSummary: (s) => {
      const list = Object.values(s.executeOutputs)
      return {
        total: list.length,
        running: list.filter(o => o.status === 'running').length,
        success: list.filter(o => o.status === 'success').length,
        failed: list.filter(o => o.status === 'failed').length
      }
    }
  },

  actions: {
    setTab(tab) {
      this.tab = tab === 'execute' ? 'execute' : 'upload'
    },

    toggleServer(id) {
      const i = this.selectedIds.indexOf(id)
      if (i >= 0) this.selectedIds.splice(i, 1)
      else this.selectedIds.push(id)
    },

    setSelectedIds(ids) {
      this.selectedIds = [...ids]
    },

    // ---- 上传 ----
    beginUpload() { this.uploadRunning = true; this.pendingUpload = true; this.earlyUpload = [] },
    setUploadSession(sessionId) {
      this.uploadSessionId = sessionId
      this.uploadRunning = !!sessionId
      this.uploadTasks = {}
      this.pendingUpload = false
      const buffered = this.earlyUpload; this.earlyUpload = []
      for (const e of buffered) { if (e.output) this.applyUploadProgress(e.output); else this.finishUpload(e.sessionId) }
    },
    applyUploadProgress(data) {
      if (this.pendingUpload) { this.earlyUpload.push({output:data}); return }
      if (this.uploadSessionId && data.sessionId && data.sessionId !== this.uploadSessionId) return
      const key = data.taskId || (data.serverId + '|' + data.localPath)
      this.uploadTasks[key] = {...(this.uploadTasks[key] || {}), ...data}
    },
    finishUpload(sessionId) {
      if (this.pendingUpload) { this.earlyUpload.push({sessionId}); return }
      if (sessionId && this.uploadSessionId && sessionId !== this.uploadSessionId) return
      this.uploadRunning = false
    },

    // ---- 执行 ----
    beginExecute() {
      this.executeRunning = true
      this.pendingExecute = true
      this.earlyExecute = []
    },
    setExecuteSession(sessionId) {
      this.executeSessionId = sessionId
      this.executeRunning = !!sessionId
      this.executeOutputs = {}
      this.pendingExecute = false
      const buffered = this.earlyExecute
      this.earlyExecute = []
      for (const event of buffered) {
        if (event.output) this.applyCommandOutput(event.output)
        else this.finishExecute(event.sessionId)
      }
    },
    applyCommandOutput(data) {
      if (this.pendingExecute) { this.earlyExecute.push({output: data}); return }
      if (this.executeSessionId && data.sessionId && data.sessionId !== this.executeSessionId) return
      this.executeOutputs[data.serverId] = data
    },
    finishExecute(sessionId) {
      if (this.pendingExecute) { this.earlyExecute.push({sessionId}); return }
      if (sessionId && this.executeSessionId && sessionId !== this.executeSessionId) return
      this.executeRunning = false
    },

    // ---- 事件监听（应用级，注册一次）----
    bindEvents() {
      if (this.bound) return
      this.bound = true
      events.on('upload:progress', (d) => this.applyUploadProgress(d))
      events.on('upload:completed', (sid) => this.finishUpload(sid))
      events.on('command:output', (d) => this.applyCommandOutput(d))
      events.on('command:completed', (sid) => this.finishExecute(sid))
    },

    unbindEvents() {
      events.off('upload:progress')
      events.off('upload:completed')
      events.off('command:output')
      events.off('command:completed')
      this.bound = false
    }
  }
})