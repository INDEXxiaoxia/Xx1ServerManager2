import {defineStore} from 'pinia'
import {events} from '@/api'
export const useTransfers = defineStore('transfers', {
  state: () => ({tasks: [], visible: false, off: null}),
  actions: {
    bind() { if (!this.off) this.off = events.on('transfer:progress', p => {
      const t = this.tasks.find(t => t.id === p.id)
      if (t && t.status === 'running') Object.assign(t, p)
    }) },
    async addPaths(servers, paths, directory) {
      const entries = await window.go.main.App.ListUploadEntries(paths)
      if (!entries.length) throw new Error('没有可上传的文件或目录')
      const occupied = new Set(this.tasks.filter(t => ['running','queued','cancelling'].includes(t.status))
        .map(t => JSON.stringify([t.serverId,t.remotePath])))
      const pending = []
      for (const server of servers) {
        for (const entry of entries) {
          const remotePath = directory.replace(/\/+$/, '') + '/' + entry.relative
          const key = JSON.stringify([server.id,remotePath])
          if (occupied.has(key)) throw new Error('目标重复：' + remotePath)
          occupied.add(key)
          pending.push({id:crypto.randomUUID(), serverId:server.id, serverName:server.name,
            localPath:entry.local, remotePath, directory:entry.directory, status:'queued',
            transferred:0, total:0, speed:0, error:''})
        }
      }
      this.tasks = this.tasks.concat(pending)
      this.pump()
    },
    pump() {
      if (this.tasks.some(t => ['running','cancelling'].includes(t.status))) return
      const t = this.tasks.find(t => t.status === 'queued')
      if (t) this.run(t)
    },
    async run(t) {
      t.status = 'running'; t.transferred = 0; t.total = 0; t.speed = 0; t.error = ''
      try { if (t.directory) await window.go.main.App.MakeRemoteDir(t.serverId,t.remotePath); else await window.go.main.App.UploadFileTask(t.id,t.serverId,t.localPath,t.remotePath); t.status = t.status === 'cancelling' ? 'cancelled' : 'success' }
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
