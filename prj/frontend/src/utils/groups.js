// 分组规则的唯一出处：单属约束 + 统一的展示顺序。
//
// 单属约束：一个服务器只能属于一个分组，groupIds 里至多一个 id（首项生效，空即未分组）。
// 顺序规则（下拉、总览分区、分组管理共用）：
//   有服务器的分组在前（台数降序，同数按中文名升序）→ 无服务器的分组（按名）→ 未分组永远最后。

export const UNGROUPED = '__ungrouped__'

const UNGROUPED_COLOR = 'var(--ink-3)'

// groupIdOf 取生效分组 id（空串表示未分组）
export function groupIdOf(server) {
  const ids = (server && server.groupIds) || []
  for (const id of ids) {
    if (id) return id
  }
  return ''
}

// groupIdsFor 把生效分组 id 组装成后端契约用的数组形态（长度 0 或 1）
export function groupIdsFor(groupId) {
  return groupId ? [groupId] : []
}

// matchServer 按名称/主机做大小写不敏感的模糊匹配
export function matchServer(server, keyword) {
  const k = String(keyword || '').trim().toLowerCase()
  if (!k) return true
  return String(server.name || '').toLowerCase().includes(k) ||
    String(server.host || '').toLowerCase().includes(k)
}

function byName(a, b) {
  return String(a.name || '').localeCompare(String(b.name || ''), 'zh')
}

// orderedGroups 分组 + 台数，按统一顺序排列（未分组永远最后）
export function orderedGroups(servers, groups) {
  const list = servers || []
  const counts = new Map()
  let ungroupedCount = 0
  for (const s of list) {
    const id = groupIdOf(s)
    if (id) counts.set(id, (counts.get(id) || 0) + 1)
    else ungroupedCount++
  }
  const items = (groups || []).map(g => ({
    id: g.id,
    name: g.name,
    color: g.color || 'var(--blue)',
    count: counts.get(g.id) || 0
  }))
  const withServers = items.filter(i => i.count > 0).sort((a, b) => b.count - a.count || byName(a, b))
  const empty = items.filter(i => i.count === 0).sort(byName)
  return [
    ...withServers,
    ...empty,
    {id: UNGROUPED, name: '未分组', color: UNGROUPED_COLOR, count: ungroupedCount, isUngrouped: true}
  ]
}

// groupedSections 总览分区：按统一顺序把服务器分到各区，空区不出现
export function groupedSections(servers, groups) {
  return orderedGroups(servers, groups)
    .map(g => ({
      ...g,
      servers: g.isUngrouped
        ? (servers || []).filter(s => !groupIdOf(s))
        : (servers || []).filter(s => groupIdOf(s) === g.id)
    }))
    .filter(sec => sec.servers.length > 0)
}

// groupNameOf 分组名（未知 id 或未分组时返回"未分组"）
export function groupNameOf(groups, groupId) {
  if (!groupId) return '未分组'
  const g = (groups || []).find(x => x.id === groupId)
  return g ? g.name : '未分组'
}

// groupColorOf 分组色（未知 id 或未分组时用弱化色）
export function groupColorOf(groups, groupId) {
  if (!groupId) return UNGROUPED_COLOR
  const g = (groups || []).find(x => x.id === groupId)
  return (g && g.color) || 'var(--blue)'
}