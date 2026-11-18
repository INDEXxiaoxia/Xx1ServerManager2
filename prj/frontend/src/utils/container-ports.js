// Keep every bind address in the details, while grouping IPv4/IPv6 duplicates in the table.
export function containerPorts(value) {
  if (!value) return []
  const entries = Array.isArray(value) ? value.map(p => typeof p === 'string' ? p : `${p.host_ip || '0.0.0.0'}:${p.host_port}->${p.container_port}/${p.protocol || 'tcp'}`) : String(value).split(',')
  const result = new Map()
  for (const entry of entries) {
    const raw = entry.trim()
    if (!raw) continue
    const match = raw.match(/^(?:(.*):(\d+(?:-\d+)?)->)?(\d+(?:-\d+)?)\/(tcp|udp|sctp)$/i)
    if (!match) { result.set(raw, {key:raw, raw, addresses:[]}); continue }
    const [,host, published, target, proto] = match
    const protocol = proto.toUpperCase()
    const key = `${published || ''}:${target}/${protocol}`
    if (!result.has(key)) result.set(key,{key,published,target,protocol,addresses:[]})
    if (host && !result.get(key).addresses.includes(host)) result.get(key).addresses.push(host)
  }
  return [...result.values()]
}
export function portDescription(port) {
  if (port.raw) return port.raw
  if (!port.published) return `容器端口 ${port.target}/${port.protocol}，未发布到宿主机`
  return `${port.addresses.join('、')} · 宿主机 ${port.published} → 容器 ${port.target}/${port.protocol}`
}
