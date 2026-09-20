// 主题机制：三种主题（光明 / 黑暗 / 赛博朋克）
// 应用方式：document.documentElement.dataset.theme = <name>（CSS 通过 :root[data-theme='...'] 切换）
// 持久化：localStorage（主，可在解锁前生效）；数据库 settings.theme 在设置页保存时同步
import {reactive} from 'vue'

export const THEMES = [
  {name: 'light', label: '光明模式'},
  {name: 'dark', label: '黑暗模式'},
  {name: 'cyber', label: '赛博朋克'}
]

const STORAGE_KEY = 'sm2_theme'
const VALID = new Set(THEMES.map(t => t.name))

// 响应式主题状态，供组件 watch（图表 / 终端联动）
export const themeState = reactive({name: 'light'})

export function getStoredTheme() {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    return VALID.has(v) ? v : 'light'
  } catch (e) {
    return 'light'
  }
}

export function applyTheme(name) {
  const n = VALID.has(name) ? name : 'light'
  themeState.name = n
  try {
    localStorage.setItem(STORAGE_KEY, n)
  } catch (e) {
    /* 忽略存储异常 */
  }
  document.documentElement.dataset.theme = n
}

// 各主题 init 背景（防首屏闪白时底色跳变）
export const THEME_BG = {
  light: '#F5F4F1',
  dark: '#1D1E24',
  cyber: '#05070D'
}

// ---------- ECharts 主题配色 ----------
export function getChartTheme(name) {
  if (name === 'cyber') {
    return {
      axis: '#7B85A3',
      line: '#2A3550',
      split: 'rgba(33,230,255,.12)',
      tooltipBg: 'rgba(10,14,24,.92)',
      tooltipBorder: 'rgba(33,230,255,.4)',
      tooltipText: '#E8EDFF',
      series: {cpu: '#21E6FF', mem: '#00FFA3', disk: '#FFB020', net: '#FF3EA5'}
    }
  }
  if (name === 'dark') {
    return {
      axis: '#A2A5B0',
      line: '#4A4B55',
      split: 'rgba(255,255,255,.10)',
      tooltipBg: '#26272E',
      tooltipBorder: '#3A3B44',
      tooltipText: '#E6E7EC',
      series: {cpu: '#6C9FD6', mem: '#7FBA96', disk: '#D0A85C', net: '#D294A3'}
    }
  }
  return {
    axis: '#6E7078',
    line: '#C9C6BF',
    split: 'rgba(70,72,79,.12)',
    tooltipBg: '#FFFFFF',
    tooltipBorder: '#E7E4DE',
    tooltipText: '#46484F',
    series: {cpu: '#5780A9', mem: '#67947A', disk: '#C79E4F', net: '#C58A98'}
  }
}

// ---------- 磁盘调色板 ----------
// 每块磁盘固定一色：'/' 恒用第 1 色（与"磁盘"语义色一致），其余按挂载点字母序取后续颜色，
// 保证同一挂载点在监控页列表、折线图、总览迷你条里颜色一致。
const DISK_PALETTE = {
  light: ['#C79E4F', '#5780A9', '#67947A', '#C58A98', '#9B7FB8', '#8F919B'],
  dark: ['#D0A85C', '#6C9FD6', '#7FBA96', '#D294A3', '#A88FD0', '#8B8F9A'],
  cyber: ['#FFB020', '#21E6FF', '#00FFA3', '#FF3EA5', '#A88FD0', '#7DEAFF']
}

// getDiskPalette 取指定主题的磁盘调色板（未知主题回退光明模式）
export function getDiskPalette(name) {
  return DISK_PALETTE[name] || DISK_PALETTE.light
}

// diskColorOf 按挂载点取稳定配色：mounts 为当前已知挂载点列表（/ 之外的按字母序排）
export function diskColorOf(name, mount, mounts) {
  const palette = getDiskPalette(name)
  if (mount === '/') return palette[0]
  const rest = (mounts || []).filter(m => m !== '/').slice().sort()
  const i = rest.indexOf(mount)
  return palette[(i < 0 ? 0 : i + 1) % palette.length]
}

// usageColor 使用率语义色：绿 → 黄 → 红（走主题变量，迷你条与进度条共用）
export function usageColor(pct) {
  const v = Number(pct) || 0
  if (v >= 85) return 'var(--red)'
  if (v >= 70) return 'var(--yellow)'
  return 'var(--green)'
}

// ---------- xterm 终端主题配色（其余配色键由 xterm 默认补齐） ----------
export function getTerminalTheme(name) {
  if (name === 'cyber') {
    return {
      background: '#070B13',
      foreground: '#C7F9FF',
      cursor: '#21E6FF',
      cursorAccent: '#070B13',
      selectionBackground: 'rgba(33,230,255,.30)',
      black: '#0A0E18',
      red: '#FF4D6D',
      green: '#00FFA3',
      yellow: '#FFB020',
      blue: '#21E6FF',
      magenta: '#FF3EA5',
      cyan: '#00D4FF',
      white: '#DCE6FF',
      brightBlack: '#3A4258',
      brightRed: '#FF7088',
      brightGreen: '#4DFFC0',
      brightYellow: '#FFC84D',
      brightBlue: '#7DEAFF',
      brightMagenta: '#FF78C0',
      brightCyan: '#70E6FF',
      brightWhite: '#FFFFFF'
    }
  }
  if (name === 'dark') {
    return {
      background: '#16171B',
      foreground: '#D6D8DD',
      cursor: '#A2A5B0',
      selectionBackground: 'rgba(120,130,160,.35)'
    }
  }
  return {background: '#1e1e1e', foreground: '#d4d4d4'}
}