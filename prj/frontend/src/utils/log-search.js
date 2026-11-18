export function findLogMatches(text, query) {
  if (!query) return []
  const escaped = query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const pattern = new RegExp(escaped, 'gi')
  return Array.from(text.matchAll(pattern), match => ({start: match.index, end: match.index + match[0].length}))
}
export function nextLogMatch(current, count, direction) {
  if (!count) return -1
  return current < 0 ? count - 1 : (current + direction + count) % count
}
