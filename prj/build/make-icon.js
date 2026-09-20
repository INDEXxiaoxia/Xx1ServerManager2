// 从 icon.png 生成规范的 Windows 多尺寸 icon.ico（含 256×256 PNG 条目）
// 用法: node prj/build/make-icon.js <源PNG> <输出ICO> [备用大图PNG]
// 说明：纯 Node 实现，先手工解码 PNG（zlib + 反滤波）取像素，再自行缩放
// （盒式滤波，逐级折半），最后按 ICO 规范拼装，避免依赖外部图形库。
const fs = require('fs')
const zlib = require('zlib')

const SRC = process.argv[2] || 'icon.png'
const OUT = process.argv[3] || 'prj/build/windows/icon.ico'
const FALLBACK = process.argv[4] // 源图较小时用于补 256×256 的大图

const SIZES = [256, 128, 64, 48, 32, 24, 16]

// ---------- PNG 解码 ----------
function decodePNG(buf) {
  if (buf.readUInt32BE(0) !== 0x89504e47) throw new Error('不是 PNG 文件')
  let pos = 8
  let width = 0, height = 0, bitDepth = 0, colorType = 0
  let palette = null, trns = null
  const idat = []
  while (pos < buf.length) {
    const len = buf.readUInt32BE(pos)
    const type = buf.toString('ascii', pos + 4, pos + 8)
    const data = buf.slice(pos + 8, pos + 8 + len)
    if (type === 'IHDR') {
      width = data.readUInt32BE(0)
      height = data.readUInt32BE(4)
      bitDepth = data[8]
      colorType = data[9]
      if (data[12] !== 0) throw new Error('不支持隔行扫描(interlace)的 PNG')
    } else if (type === 'PLTE') {
      palette = Buffer.from(data)
    } else if (type === 'tRNS') {
      trns = Buffer.from(data)
    } else if (type === 'IDAT') {
      idat.push(data)
    } else if (type === 'IEND') {
      break
    }
    pos += 12 + len
  }
  if (bitDepth !== 8) throw new Error('仅支持 8 位深 PNG，当前 ' + bitDepth)
  const channels = {0: 1, 2: 3, 3: 1, 4: 2, 6: 4}[colorType]
  if (!channels) throw new Error('不支持的 PNG 颜色类型 ' + colorType)

  const raw = zlib.inflateSync(Buffer.concat(idat))
  const bpp = channels // 8 位深时每像素字节数 = 通道数
  const stride = width * bpp
  const out = Buffer.alloc(height * stride)
  let rp = 0
  for (let y = 0; y < height; y++) {
    const filter = raw[rp++]
    const line = raw.slice(rp, rp + stride)
    rp += stride
    const cur = out.slice(y * stride, (y + 1) * stride)
    const prev = y > 0 ? out.slice((y - 1) * stride, y * stride) : null
    for (let x = 0; x < stride; x++) {
      const a = x >= bpp ? cur[x - bpp] : 0
      const b = prev ? prev[x] : 0
      const c = prev && x >= bpp ? prev[x - bpp] : 0
      let v = line[x]
      switch (filter) {
        case 0: break
        case 1: v = (v + a) & 0xff; break
        case 2: v = (v + b) & 0xff; break
        case 3: v = (v + ((a + b) >> 1)) & 0xff; break
        case 4: {
          const p = a + b - c
          const pa = Math.abs(p - a), pb = Math.abs(p - b), pc = Math.abs(p - c)
          const pr = pa <= pb && pa <= pc ? a : (pb <= pc ? b : c)
          v = (v + pr) & 0xff
          break
        }
        default: throw new Error('未知 PNG 滤波类型 ' + filter)
      }
      cur[x] = v
    }
  }

  // 统一为 RGBA
  const rgba = Buffer.alloc(width * height * 4)
  for (let i = 0, px = 0; i < width * height; i++, px += bpp) {
    let r, g, b, a = 255
    if (colorType === 0) { r = g = b = out[px] }
    else if (colorType === 2) { r = out[px]; g = out[px + 1]; b = out[px + 2] }
    else if (colorType === 3) {
      const idx = out[px]
      r = palette[idx * 3]; g = palette[idx * 3 + 1]; b = palette[idx * 3 + 2]
      if (trns && idx < trns.length) a = trns[idx]
    } else if (colorType === 4) { r = g = b = out[px]; a = out[px + 1] }
    else { r = out[px]; g = out[px + 1]; b = out[px + 2]; a = out[px + 3] }
    rgba[i * 4] = r; rgba[i * 4 + 1] = g; rgba[i * 4 + 2] = b; rgba[i * 4 + 3] = a
  }
  return {width, height, rgba}
}

// ---------- 缩放（精确目标尺寸的盒式滤波） ----------
// 源尺寸与目标尺寸一般不整除，因此按目标像素反向映射到源矩形逐块求平均，
// 保证输出尺寸精确等于目标尺寸（不能靠反复折半，会出现 156x156 这种尺寸）。
function boxResize(img, target) {
  const {width: sw, height: sh, rgba} = img
  if (sw === target && sh === target) return img
  const out = Buffer.alloc(target * target * 4)
  for (let ty = 0; ty < target; ty++) {
    const y0 = Math.floor(ty * sh / target)
    const y1 = Math.max(y0 + 1, Math.floor((ty + 1) * sh / target))
    for (let tx = 0; tx < target; tx++) {
      const x0 = Math.floor(tx * sw / target)
      const x1 = Math.max(x0 + 1, Math.floor((tx + 1) * sw / target))
      let r = 0, g = 0, b = 0, a = 0, n = 0
      for (let sy = y0; sy < y1 && sy < sh; sy++) {
        for (let sx = x0; sx < x1 && sx < sw; sx++) {
          const s = (sy * sw + sx) * 4
          r += rgba[s]; g += rgba[s + 1]; b += rgba[s + 2]; a += rgba[s + 3]
          n++
        }
      }
      if (!n) n = 1
      const o = (ty * target + tx) * 4
      out[o] = Math.round(r / n)
      out[o + 1] = Math.round(g / n)
      out[o + 2] = Math.round(b / n)
      out[o + 3] = Math.round(a / n)
    }
  }
  return {width: target, height: target, rgba: out}
}

// ---------- 裁掉四周全透明边距（保留少量留白），让图形在小尺寸下也更清晰 ----------
function trimTransparent(img, alphaThreshold = 8, padRatio = 0.02) {
  const {width: w, height: h, rgba} = img
  let minX = w, minY = h, maxX = -1, maxY = -1
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      if (rgba[(y * w + x) * 4 + 3] > alphaThreshold) {
        if (x < minX) minX = x
        if (x > maxX) maxX = x
        if (y < minY) minY = y
        if (y > maxY) maxY = y
      }
    }
  }
  if (maxX < 0 || maxY < 0) return img // 全透明，保持原样
  const boxW = maxX - minX + 1
  const boxH = maxY - minY + 1
  if (boxW === w && boxH === h) return img

  // 取正方形边（含少量留白），保证不拉伸变形
  const side = Math.max(boxW, boxH)
  const size = Math.min(Math.max(w, h), Math.round(side * (1 + padRatio * 2)))
  const cx = (minX + maxX + 1) / 2
  const cy = (minY + maxY + 1) / 2
  let x0 = Math.round(cx - size / 2)
  let y0 = Math.round(cy - size / 2)
  x0 = Math.max(0, Math.min(x0, w - size))
  y0 = Math.max(0, Math.min(y0, h - size))

  const out = Buffer.alloc(size * size * 4)
  for (let y = 0; y < size; y++) {
    const sy = Math.min(h - 1, y0 + y)
    for (let x = 0; x < size; x++) {
      const sx = Math.min(w - 1, x0 + x)
      const s = (sy * w + sx) * 4, d = (y * size + x) * 4
      out[d] = rgba[s]; out[d + 1] = rgba[s + 1]; out[d + 2] = rgba[s + 2]; out[d + 3] = rgba[s + 3]
    }
  }
  console.log(`  裁掉透明边距：${w}x${h} → ${size}x${size}（内容区 ${boxW}x${boxH}）`)
  return {width: size, height: size, rgba: out}
}

// ---------- PNG 编码（ICO 内 256×256 必须用 PNG 压缩） ----------
let crcTable = null
function crc32(buf) {
  if (!crcTable) {
    crcTable = new Int32Array(256)
    for (let n = 0; n < 256; n++) {
      let c = n
      for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
      crcTable[n] = c
    }
  }
  let c = -1
  for (let i = 0; i < buf.length; i++) c = crcTable[(c ^ buf[i]) & 0xff] ^ (c >>> 8)
  return (c ^ -1) >>> 0
}

function chunk(type, data) {
  const len = Buffer.alloc(4)
  len.writeUInt32BE(data.length)
  const body = Buffer.concat([Buffer.from(type, 'ascii'), data])
  const crc = Buffer.alloc(4)
  crc.writeUInt32BE(crc32(body))
  return Buffer.concat([len, body, crc])
}

function encodePNG(img) {
  const {width: w, height: h, rgba} = img
  // 逐行自适应选择滤波类型（最小绝对差启发式），显著提升压缩率
  const stride = w * 4
  const raw = Buffer.alloc(h * (1 + stride))
  for (let y = 0; y < h; y++) {
    const line = rgba.slice(y * stride, (y + 1) * stride)
    const prev = y > 0 ? rgba.slice((y - 1) * stride, y * stride) : null
    let bestType = 0
    let best = null
    let bestScore = Infinity
    for (let type = 0; type <= 4; type++) {
      const out = Buffer.alloc(stride)
      let score = 0
      for (let x = 0; x < stride; x++) {
        const a = x >= 4 ? line[x - 4] : 0
        const b = prev ? prev[x] : 0
        const c = prev && x >= 4 ? prev[x - 4] : 0
        let v
        switch (type) {
          case 0: v = line[x]; break
          case 1: v = line[x] - a; break
          case 2: v = line[x] - b; break
          case 3: v = line[x] - ((a + b) >> 1); break
          default: {
            const p = a + b - c
            const pa = Math.abs(p - a), pb = Math.abs(p - b), pc = Math.abs(p - c)
            v = line[x] - (pa <= pb && pa <= pc ? a : (pb <= pc ? b : c))
            break
          }
        }
        v &= 0xff
        out[x] = v
        score += v < 128 ? v : 256 - v
      }
      if (score < bestScore) { bestScore = score; bestType = type; best = out }
    }
    raw[y * (1 + stride)] = bestType
    best.copy(raw, y * (1 + stride) + 1)
  }
  const ihdr = Buffer.alloc(13)
  ihdr.writeUInt32BE(w, 0)
  ihdr.writeUInt32BE(h, 4)
  ihdr[8] = 8   // bit depth
  ihdr[9] = 6   // color type: RGBA
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', ihdr),
    chunk('IDAT', zlib.deflateSync(raw, {level: 9})),
    chunk('IEND', Buffer.alloc(0))
  ])
}

// ---------- ICO 组装：<64 用 BMP(DIB) 条目，>=64 用 PNG 条目（Windows 规范做法） ----------
function bmpEntry(img) {
  const {width: w, height: h, rgba} = img
  const header = Buffer.alloc(40)
  header.writeUInt32LE(40, 0)
  header.writeInt32LE(w, 4)
  header.writeInt32LE(h * 2, 8)  // XOR + AND 掩码，高度写两倍
  header.writeUInt16LE(1, 12)
  header.writeUInt16LE(32, 14)
  header.writeUInt32LE(w * h * 4, 20)
  const xor = Buffer.alloc(w * h * 4)
  for (let y = 0; y < h; y++) {
    const src = (h - 1 - y) * w * 4 // BMP 自下而上
    for (let x = 0; x < w; x++) {
      const s = src + x * 4, d = (y * w + x) * 4
      xor[d] = rgba[s + 2]      // B
      xor[d + 1] = rgba[s + 1]  // G
      xor[d + 2] = rgba[s]      // R
      xor[d + 3] = rgba[s + 3]  // A
    }
  }
  const maskStride = Math.ceil(w / 32) * 4
  const and = Buffer.alloc(maskStride * h) // 32 位带 alpha，掩码全 0
  return Buffer.concat([header, xor, and])
}

function buildICO(images) {
  const dir = Buffer.alloc(6 + images.length * 16)
  dir.writeUInt16LE(0, 0)
  dir.writeUInt16LE(1, 2)
  dir.writeUInt16LE(images.length, 4)
  let offset = dir.length
  const bodies = []
  images.forEach((img, i) => {
    const data = img.size >= 64 ? encodePNG(img.img) : bmpEntry(img.img)
    const o = 6 + i * 16
    dir[o] = img.size >= 256 ? 0 : img.size
    dir[o + 1] = img.size >= 256 ? 0 : img.size
    dir[o + 2] = 0 // 颜色数
    dir[o + 3] = 0
    dir.writeUInt16LE(1, o + 4)      // planes
    dir.writeUInt16LE(32, o + 6)     // bpp
    dir.writeUInt32LE(data.length, o + 8)
    dir.writeUInt32LE(offset, o + 12)
    offset += data.length
    bodies.push(data)
  })
  return Buffer.concat([dir, ...bodies])
}

// ---------- 主流程 ----------
const src = decodePNG(fs.readFileSync(SRC))
console.log(`源图 ${SRC}: ${src.width}x${src.height}`)
let master = src
if (src.width < 256 && FALLBACK && fs.existsSync(FALLBACK)) {
  const fb = decodePNG(fs.readFileSync(FALLBACK))
  console.log(`源图偏小，改用备用大图 ${FALLBACK}: ${fb.width}x${fb.height}`)
  master = fb
}
if (!process.argv.includes('--no-trim')) master = trimTransparent(master)
const images = SIZES.filter(s => s <= master.width).map(size => ({size, img: boxResize(master, size)}))
if (!images.length) throw new Error('源图小于 16x16，无法生成图标')
const ico = buildICO(images)
fs.mkdirSync(require('path').dirname(OUT), {recursive: true})
fs.writeFileSync(OUT, ico)
console.log(`已生成 ${OUT}: ${ico.length} 字节，包含 ${images.map(i => i.size).join('/')}（其中 >=64 为 PNG 条目，<64 为 BMP 条目）`)

// 同时把源图规整为 1024x1024 的 appicon.png（macOS 图标与安装包图标的基础图）
if (process.argv[5] === '--appicon') {
  const APPICON = 'prj/build/appicon.png'
  const appImg = boxResize(master, 1024)
  fs.writeFileSync(APPICON, encodePNG(appImg))
  console.log(`已生成 ${APPICON}: ${fs.statSync(APPICON).size} 字节，1024x1024`)
}