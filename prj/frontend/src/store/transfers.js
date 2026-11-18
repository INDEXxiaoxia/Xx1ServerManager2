import {defineStore} from 'pinia'
import {events} from '@/api'
export const useTransfers = defineStore('transfers', {
  state: () => ({tasks: [], visible: false, off: null}),
  actions: {
    bind() { if (!this.off) this.off = events.on('transfer:progress', p => {
      const t = this.tasks.find(t => t.id === p.id)
      if (t && t.status === 'running') Object.assign(t, p)
    }) },
    add(serverId, serverName, localPath, remotePath) {
      if (this.tasks.some(t => ['running','queued','cancelling'].includes(t.status) && t.serverId === serverId && t.remotePath === remotePath)) throw new Error('该目标文件已有上传任务')
      const t = {id: crypto.randomUUID(), serverId, serverName, localPath, remotePath, status: 'queued', transferred: 0, total: 0, speed: 0, error: ''}
      this.tasks.push(t); this.visible = true; this.pump()
    },
    pump() {
      if (this.tasks.some(t => ['running','cancelling'].includes(t.status))) return
      const t = this.tasks.find(t => t.status === 'queued')
      if (t) this.run(t)
    },
    async run(t) {
      t.status = 'running'; t.transferred = 0; t.total = 0; t.speed = 0; t.error = ''
      try { await window.go.main.App.UploadFileTask(t.id,t.serverId,t.localPath,t.remotePath); t.status = t.status === 'cancelling' ? 'cancelled' : 'success' }
      catch(e) { t.error = String(e); t.status = t.status === 'cancelling' ? 'cancelled' : 'failed' }
      finally { t.speed = 0; this.pump() }
    },
    async cancel(t) {
      if (t.status === 'queued') { t.status = 'cancelled'; return }
      t.status = 'cancelling'
      try { await window.go.main.App.CancelFileTransfer(t.id) } catch(e) { t.error = String(e) }
    },
    retry(t) { if (!['failed','cancelled'].includes(t.status)) return; t.id = crypto.randomUUID(); t.status = 'queued'; this.pump() }
  }
})
