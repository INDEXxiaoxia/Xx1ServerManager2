import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import {reactive,ref,computed,watch,nextTick} from 'vue'
const raw = await readFile(new URL('../src/views/terminal/AiPanel.vue',import.meta.url),'utf8')
const script = raw.split('<script setup>')[1].split('</script>')[0].replace(/^import .*$/gm,'')
function panel(api) {
 const props = reactive({activeTerminal:{key:'one',termId:'term-one',serverId:'s1'},terminals:[{key:'one'},{key:'two'}],sync:false})
 const handlers = new Map(); let state
 const names = ['ref','reactive','computed','watch','nextTick','onMounted','onActivated','onBeforeUnmount','defineProps','defineEmits','localStorage','api','events','ElMessage']
 const args = [ref,reactive,computed,watch,nextTick,()=>{},()=>{},()=>{},()=>props,()=>((_,s)=>state=s),{getItem:()=>null,setItem(){}},api,{on(n,cb){handlers.set(n,cb);return()=>handlers.delete(n)}},{warning(){},error(){},success(){}}]
 const p = new Function(...names,script+'; return {send,chat,chats,input,aiReady,mode,emitTakeover,execCommand};')(...args)
 p.aiReady.value=true
 return {p,props,handlers,getState:()=>state}
}
test('reply and automatic execution remain on original terminal after switch',async()=>{
 let resolve; const calls=[]
 const {p,props} = panel({aiChat:()=>new Promise(r=>resolve=r),aiExecOne:async(...args)=>{calls.push(args);return {output:'done'}}})
 p.mode.value='exec';p.input.value='task'; const request=p.send()
 props.activeTerminal={key:'two',termId:'term-two',serverId:'s2'};await nextTick()
 resolve({text:'reply',commands:[{command:'pwd'}]});await request
 assert.equal(p.chats.one.messages.some(m=>m.text==='reply'),true)
 assert.equal(p.chats.two.messages.length,0)
 assert.equal(calls[0][0],'term-one')
})
test('background takeover keeps receiving events and unlocks only on completion',async()=>{
 const {p,props,handlers,getState} = panel({aiStartTakeover:async()=>{}})
 p.mode.value='takeover';p.input.value='task';await p.send()
 const sid=p.chats.one.sessionId
 props.activeTerminal={key:'two',termId:'term-two',serverId:'s2'};await nextTick()
 assert.deepEqual(getState().lockedKeys,['one']);assert.equal(getState().full,false)
 handlers.get('ai:state:'+sid)({state:'paused',kind:'input',message:'password'})
 assert.deepEqual(getState().lockedKeys,[]);assert.deepEqual(getState().busyKeys,['one'])
 handlers.get('ai:step:'+sid)({command:'pwd'})
 assert.equal(p.chats.two.messages.length,0)
 handlers.get('ai:state:'+sid)({state:'done'})
 assert.deepEqual(getState().busyKeys,[])
})
