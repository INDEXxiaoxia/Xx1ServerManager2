import {reactive, nextTick} from 'vue'

const orders = reactive(new Map())
const animations = new WeakMap()
let dragging
function order(scope) {
  if (!orders.has(scope)) {
    let saved
    try { saved = JSON.parse(localStorage.getItem('xx1.order.' + scope)) } catch {}
    orders.set(scope, Array.isArray(saved) ? saved : [])
  }
  return orders.get(scope)
}
export function ordered(rows, scope, key = 'id') {
  const ranks = new Map(order(scope).map((id, i) => [id, i]))
  return [...(rows || [])].sort((a, b) =>
    (ranks.get(a[key]) ?? Infinity) - (ranks.get(b[key]) ?? Infinity))
}
function targetAt(event) {
  const element = event.target.closest?.('[data-order-scope], tr')
  const handle = element?.matches('[data-order-scope]') ? element : element?.querySelector('[data-order-scope]')
  return handle?.dataset.orderScope === dragging?.scope ? handle : null
}
function rowElement(handle) { return handle.closest('tr') || handle }

function listParent(element) { return element.closest?.('.el-tree') || element.parentElement }

function finish(save) {
  if (!dragging) return
  const {scope, original} = dragging
  dragging = null
  if (save) localStorage.setItem('xx1.order.' + scope, JSON.stringify(order(scope)))
  else orders.set(scope, original)
  document.querySelectorAll('.order-moving').forEach(el => el.classList.remove('order-moving'))
}

document.addEventListener('dragover', event => {
  if (!dragging) return
  const target = targetAt(event)
  if (!target) return
  event.preventDefault()
  const {scope, rows, key, id} = dragging
  const ids = ordered(rows, scope, key).map(row => row[key])
  // Slots stay fixed relative to their list while the rows animate between them.
  const origin = dragging.parent.getBoundingClientRect()
  const x = event.clientX - origin.left, y = event.clientY - origin.top
  const slot = dragging.slots.find(r => x >= r.left && x < r.right && y >= r.top && y < r.bottom)
  if (!slot) return
  const from = ids.indexOf(id), to = slot.index
  if (from < 0 || to < 0 || from === to) return
  const elements = dragging.elements
  const positions = elements.map(el => [el, el.getBoundingClientRect()])
  for (const el of elements) animations.get(el)?.cancel()
  ids.splice(to, 0, ids.splice(from, 1)[0])
  const visible = new Set(ids)
  let index = 0
  const next = order(scope).map(old => visible.has(old) ? ids[index++] : old)
  orders.set(scope, next.concat(ids.slice(index)))
  nextTick(() => {
    if (matchMedia('(prefers-reduced-motion: reduce)').matches) return
    for (const [el, old] of positions) {
      const current = el.getBoundingClientRect()
      const dx = old.left - current.left, dy = old.top - current.top
      if (el !== dragging?.source && (dx || dy)) animations.set(el, el.animate([{transform: 'translate(' + dx + 'px,' + dy + 'px)'}, {transform:'none'}], {duration:150,easing:'ease-out'}))
    }
  })
})
document.addEventListener('drop', event => {
  if (dragging && targetAt(event)) { event.preventDefault(); finish(true) }
})
document.addEventListener('dragend', () => finish(false))
export function dragRow(rows, scope, row, key = 'id') {
  return {
    draggable: true, title: '拖动排序', 'data-order-scope': scope, 'data-order-id': row[key],
    onDragstart(event) {
      event.stopPropagation()
      const source = rowElement(event.currentTarget)
      const parent = listParent(source)
      const origin = parent.getBoundingClientRect()
      const ids = ordered(rows, scope, key).map(item => String(item[key]))
      const handles = [...parent.querySelectorAll('[data-order-scope]')].filter(handle =>
        handle.dataset.orderScope === scope && listParent(rowElement(handle)) === parent)
      const elements = handles.map(rowElement)
      const slots = handles.map((handle, i) => {
        const rect = elements[i].getBoundingClientRect()
        return {index: ids.indexOf(handle.dataset.orderId), left:rect.left-origin.left,
          right:rect.right-origin.left, top:rect.top-origin.top, bottom:rect.bottom-origin.top}
      })
      dragging = {scope, rows, key, id:row[key], original:[...order(scope)], parent, elements, slots, source}
      rowElement(event.currentTarget).classList.add('order-moving')
      event.dataTransfer.setData('text/plain', String(row[key]))
      event.dataTransfer.effectAllowed = 'move'
    }
  }
}
