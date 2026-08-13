import {defineStore} from 'pinia'
import {api} from '@/api'

export const useAppStore = defineStore('app', {
  state: () => ({
    unlocked: false,
    masterPasswordSet: false,
    settings: {maxConcurrent: 5, uploadTimeout: 300, commandTimeout: 30, theme: 'light'}
  }),
  actions: {
    async init() {
      try {
        const info = await api.getAppInfo()
        this.masterPasswordSet = info.masterPasswordSet
        this.unlocked = info.unlocked
        if (info.settings) this.settings = info.settings
      } catch (e) {
        console.error('初始化失败', e)
      }
    },
    async unlock(password) {
      const ok = await api.verifyMasterPassword(password)
      if (ok) this.unlocked = true
      return ok
    },
    async setPassword(password) {
      await api.setMasterPassword(password)
      this.masterPasswordSet = true
      this.unlocked = true
    },
    async saveSettings() {
      await api.saveSettings(this.settings)
    }
  }
})

export const useServerStore = defineStore('server', {
  state: () => ({
    servers: [],
    groups: []
  }),
  actions: {
    async loadServers() {
      this.servers = await api.listServers()
      return this.servers
    },
    async loadGroups() {
      this.groups = await api.listGroups()
      return this.groups
    },
    async init() {
      await Promise.all([this.loadServers(), this.loadGroups()])
    }
  }
})