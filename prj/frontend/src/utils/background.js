import {computed, ref, watch} from 'vue'

const key = 'xx1.background.'
const storedOpacity = Number(localStorage.getItem(key + 'opacity') ?? 25)
export const opacity = ref(Number.isFinite(storedOpacity) ? Math.min(100, Math.max(0, storedOpacity)) : 25)
export const enabled = ref(localStorage.getItem(key + 'enabled') === 'true')
export const imageUrl = ref('')
export const imageName = ref('')
watch(enabled, value => localStorage.setItem(key + 'enabled', String(value)))
watch(opacity, value => localStorage.setItem(key + 'opacity', String(value)))

function imageRecord(file) {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open('xx1.appearance', 1)
    request.onupgradeneeded = () => request.result.createObjectStore('images')
    request.onerror = () => reject(request.error)
    request.onsuccess = () => {
      const db = request.result
      const tx = db.transaction('images', file ? 'readwrite' : 'readonly')
      const store = tx.objectStore('images')
      const operation = file ? store.put(file, 'background') : store.get('background')
      tx.oncomplete = () => { db.close(); resolve(operation.result) }
      tx.onabort = () => { db.close(); reject(tx.error) }
    }
  })
}
function display(preview) {
  imageUrl.value = 'data:image/jpeg;base64,' + preview.data
  imageName.value = preview.name
}
async function restore() {
  const saved = await imageRecord()
  if (!saved) return
  if (saved instanceof Blob) {
    // Never hand an unbounded legacy bitmap to the WebView renderer.
    if (saved.size > 32 * 1024 * 1024) return
    const data = await new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(reader.result.split(',')[1])
      reader.onerror = () => reject(reader.error)
      reader.readAsDataURL(saved)
    })
    const preview = await window.go.main.App.ConvertSavedBackground(saved.name, data)
    await imageRecord(preview)
    display(preview)
  } else if (saved.data) display(saved)
}
const restored = restore().catch(() => {})
export async function uploadBackground() {
  await restored
  const preview = await window.go.main.App.ChooseBackgroundImage()
  if (!preview) return
  await imageRecord(preview)
  display(preview)
  enabled.value = true
}
export const backgroundStyle = computed(() => {
  if (!enabled.value || !imageUrl.value) return undefined
  const tint = 'color-mix(in srgb, var(--page-bg) ' + (100 - opacity.value) + '%, transparent)'
  return {backgroundImage: 'linear-gradient(' + tint + ',' + tint + '), url("' + imageUrl.value + '")',
    backgroundSize: 'cover', backgroundPosition: 'center', backgroundRepeat: 'no-repeat'}
})
