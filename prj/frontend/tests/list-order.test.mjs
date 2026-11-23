import 'vue'
import test from 'node:test'
import assert from 'node:assert/strict'

test('a stationary pointer cannot swap animated neighbours back', async () => {
  const listeners = {}
  global.document = {addEventListener: (name, fn) => listeners[name] = fn, querySelectorAll: () => []}
  global.localStorage = {getItem: () => null, setItem() {}}
  global.matchMedia = () => ({matches: true})
  const {dragRow, ordered} = await import('../src/utils/list-order.js')
  const rect = y => ({top:y, bottom:y+20, left:0, right:100})
  const parent = {getBoundingClientRect: () => rect(0), querySelectorAll: () => handles}
  const handles = ['A','B'].map((id,i) => ({
    dataset:{orderId:id,orderScope:'servers'}, parentElement:parent,
    matches: () => true, closest(selector) {return selector.includes('data-order-scope') ? this : null},
    getBoundingClientRect: () => rect(i*20), classList:{add() {}}
  }))
  const rows = [{id:'A'}, {id:'B'}]
  const event = {currentTarget:handles[0], target:handles[1], clientX:10, clientY:31,
    preventDefault() {}, stopPropagation() {}, dataTransfer:{setData() {}}}
  dragRow(rows,'servers',rows[0]).onDragstart(event)
  listeners.dragover(event)
  for (const handle of handles) {
    event.target = handle
    listeners.dragover(event)
    assert.equal(ordered(rows,'servers')[0].id, 'B')
  }
  event.clientY = 8
  listeners.dragover(event)
  assert.equal(ordered(rows,'servers')[0].id,'A')
  listeners.dragend()
})
