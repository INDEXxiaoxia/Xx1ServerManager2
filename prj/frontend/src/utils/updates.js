import config from '../../../wails.json'

export const repositoryURL = 'https://github.com/INDEXxiaoxia/Xx1ServerManager2'
export const currentVersion = config.info.productVersion
export function isNewerVersion(tag) {
  return /^v?\d+\.\d+\.\d+$/i.test(tag) &&
    tag.replace(/^v/i, '').localeCompare(currentVersion, 'en', {numeric:true}) > 0
}

const interval = 12 * 60 * 60 * 1000
const lastCheckKey = 'xx1.update.lastAutomaticCheck'
export function scheduleUpdateChecks(onUpdate) {
  let stopped = false
  let timer
  async function check() {
    const now = Date.now()
    const last = Number(localStorage.getItem(lastCheckKey))
    const remaining = last > 0 && last <= now ? interval - (now - last) : 0
    if (remaining > 0) {
      timer = setTimeout(check, remaining)
      return
    }
    localStorage.setItem(lastCheckKey, String(now))
    try {
      const tag = await window.go.main.App.LatestVersion()
      if (!stopped && isNewerVersion(tag)) onUpdate(tag)
    } catch {
      // Manual checks display network errors. Background checks retry at the next interval.
    } finally {
      if (!stopped) timer = setTimeout(check, Math.max(0, now + interval - Date.now()))
    }
  }
  check()
  return () => { stopped = true; clearTimeout(timer) }
}
