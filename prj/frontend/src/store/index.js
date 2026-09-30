import {defineStore} from 'pinia'
import {api} from '@/api'

export const useAppStore = defineStore('app', {
  state: () => ({
    // ready：getAppInfo 是否已返回。未返回前不渲染主界面，
    // 否则锁定态启动时会先闪一下应用外壳再跳到解锁页。
    ready: false,
    locked: false,
    unlocked: false,
    masterPasswordSet: false,
    settings: {maxConcurrent: 5, uploadTimeout: 300, commandTimeout: 30, theme: 'light'}
  }),
  actions: {
    async init() {
      try {
        const info = await api.getAppInfo()
        this.masterPasswordSet = !!info.masterPasswordSet
        this.locked = !!info.locked
        this.unlocked = !!info.unlocked
        if (info.settings) this.settings = info.settings
      } catch (e) {
        console.error('初始化失败', e)
      } finally {
        this.ready = true
      }
    },
    // lock 锁定：清掉内存密钥并进入锁定态（未保存的密码不会丢，密文仍在库里）
    async lock() {
      await api.lock()
      this.locked = true
      this.unlocked = false
    },
    // unlock 用主密码解锁
    async unlock(password) {
      const ok = await api.verifyMasterPassword(password)
      if (ok) {
        this.unlocked = true
        this.locked = false
      }
      return ok
    },
    // unlockWithoutPassword 未设置主密码时的一键解锁
    async unlockWithoutPassword() {
      await api.unlockWithoutPassword()
      this.unlocked = true
      this.locked = false
    },
    async setPassword(password) {
      await api.setMasterPassword(password)
      this.masterPasswordSet = true
      this.unlocked = true
      this.locked = false
    },
    async changePassword(oldPassword, newPassword) {
      const res = await api.changeMasterPassword(oldPassword, newPassword)
      this.masterPasswordSet = true
      this.unlocked = true
      this.locked = false
      return res
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