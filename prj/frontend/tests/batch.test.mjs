import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import {createRequire} from 'node:module'
import {pathToFileURL} from 'node:url'
import {createPinia, setActivePinia} from 'pinia'
const require = createRequire(import.meta.url)
const source = (await readFile(new URL('../src/store/batch.js',import.meta.url),'utf8'))
  .replace("from 'pinia'", `from '${pathToFileURL(require.resolve('pinia/dist/pinia.mjs')).href}'`)
  .replace("import {events} from '@/api'",'const events = {on(){},off(){}}')
const {useBatchStore} = await import('data:text/javascript;base64,'+Buffer.from(source).toString('base64'))
test('early results and completion survive session acknowledgement', () => {
  setActivePinia(createPinia()); const s = useBatchStore()
  s.executeSessionId = 'old'; s.beginExecute()
  s.applyCommandOutput({sessionId:'new',serverId:'one',status:'success',stdout:'ok'})
  s.finishExecute('new'); s.setExecuteSession('new')
  assert.equal(s.executeOutputs.one.stdout,'ok'); assert.equal(s.executeRunning,false)
  s.applyCommandOutput({sessionId:'old',serverId:'one',status:'failed'})
  assert.equal(s.executeOutputs.one.status,'success')
})
test('upload progress before acknowledgement is preserved', () => {
  setActivePinia(createPinia()); const s = useBatchStore()
  s.beginUpload(); s.applyUploadProgress({sessionId:'upload',taskId:'t',progress:100,status:'success'})
  s.finishUpload('upload'); s.setUploadSession('upload')
  assert.equal(s.uploadTasks.t.progress,100); assert.equal(s.uploadRunning,false)
})
