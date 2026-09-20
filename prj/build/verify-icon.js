// 从 Windows PE 的 .rsrc 中提取 RT_GROUP_ICON（类型 14）与 RT_ICON（类型 3），
// 用于校验 exe 内嵌图标是否已被替换（纯 Node 手工解析 PE 结构）。
const fs = require('fs')
const crypto = require('crypto')

function parsePE(file) {
  const b = fs.readFileSync(file)
  if (b.readUInt16LE(0) !== 0x5a4d) throw new Error('不是 PE 文件')
  const peOff = b.readUInt32LE(0x3c)
  if (b.readUInt32LE(peOff) !== 0x00004550) throw new Error('PE 签名不正确')
  const nsec = b.readUInt16LE(peOff + 6)
  const optSize = b.readUInt16LE(peOff + 20)
  const sections = []
  for (let i = 0; i < nsec; i++) {
    const s = peOff + 24 + optSize + i * 40
    sections.push({
      name: b.toString('ascii', s, s + 8).replace(/\0/g, ''),
      vsize: b.readUInt32LE(s + 8),
      vaddr: b.readUInt32LE(s + 12),
      rawSize: b.readUInt32LE(s + 16),
      rawPtr: b.readUInt32LE(s + 20)
    })
  }
  const rsrc = sections.find(s => s.name === '.rsrc')
  if (!rsrc) throw new Error('没有 .rsrc 段')
  return {buf: b, rsrc}
}

function rva2off(b, rsrc, rva) {
  return rsrc.rawPtr + (rva - rsrc.vaddr)
}

// 资源目录项遍历。注意两个偏移体系不要混：
//   目录项里的子目录/数据偏移都是「相对资源段起始」的 rsrc 段内偏移；
//   只有数据条目的 DataRVA 是真正的 RVA，需要再换算成文件偏移。
function walkDir(b, rsrc, dirOff, level, path, out) {
  const rsBase = rva2off(b, rsrc, rsrc.vaddr)
  const named = b.readUInt16LE(dirOff + 12)
  const ids = b.readUInt16LE(dirOff + 14)
  const total = named + ids
  for (let i = 0; i < total; i++) {
    const e = dirOff + 16 + i * 8
    const nameId = b.readUInt32LE(e)
    const off = b.readUInt32LE(e + 4)
    const isDir = (off & 0x80000000) !== 0
    const key = (nameId & 0x80000000) ? 'name' : nameId
    if (isDir) {
      walkDir(b, rsrc, rsBase + (off & 0x7fffffff), level + 1, path.concat(key), out)
    } else {
      const de = rsBase + (off & 0x7fffffff)
      const dataRva = b.readUInt32LE(de)
      const dataSize = b.readUInt32LE(de + 4)
      out.push({path: path.concat(key).join('/'), rva: dataRva, size: dataSize})
    }
  }
}

const file = process.argv[2] || 'prj/build/bin/prj.exe'
const {buf, rsrc} = parsePE(file)
const entries = []
walkDir(buf, rsrc, rva2off(buf, rsrc, rsrc.vaddr), 0, [], entries)

const icons = entries.filter(e => e.path.startsWith('3/'))
const groups = entries.filter(e => e.path.startsWith('14/'))
console.log(`${file}: 内嵌 ${groups.length} 个图标组、${icons.length} 个图标图像`)

// 汇总所有图标图像数据的哈希与尺寸（PNG 条目读 IHDR，BMP 条目读 BITMAPINFOHEADER）
const summary = icons.map(e => {
  const off = rva2off(buf, rsrc, e.rva)
  const data = buf.slice(off, off + e.size)
  let dim = '?'
  if (data[0] === 0x89 && data[1] === 0x50) {
    dim = data.readUInt32BE(16) + 'x' + data.readUInt32BE(20) + '(png)'
  } else if (data.length > 24) {
    dim = data.readInt32LE(4) + 'x' + (data.readInt32LE(8) / 2) + '(bmp)'
  }
  return {id: e.path, dim, sha: crypto.createHash('sha1').update(data).digest('hex').slice(0, 12)}
})
summary.sort((a, b) => b.dim.localeCompare(a.dim))
summary.forEach(s => console.log('   ', s.id, s.dim, s.sha))

// 与目标 ico 对比：看新的 256x256(png) 是否出现在 exe 里
const targetIco = process.argv[3]
if (targetIco) {
  const ico = fs.readFileSync(targetIco)
  const n = ico.readUInt16LE(4)
  let match = 0
  for (let i = 0; i < n; i++) {
    const o = 6 + i * 16
    const sz = ico.readUInt32LE(o + 8), io = ico.readUInt32LE(o + 12)
    const data = ico.slice(io, io + sz)
    const sha = crypto.createHash('sha1').update(data).digest('hex').slice(0, 12)
    const hit = summary.find(s => s.sha === sha)
    const dim = ico[o] || 256
    console.log(`    ico ${dim}x${dim} (${sz}B, ${sha}) -> ${hit ? '已嵌入 ' + hit.id : '未找到'}`)
    if (hit) match++
  }
  console.log(match ? `✔ ${targetIco} 有 ${match}/${n} 个尺寸已嵌入该 exe` : `✘ 该 exe 未使用 ${targetIco}`)
}