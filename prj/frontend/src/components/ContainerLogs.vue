<script setup>
import {ref,computed,watch,nextTick,onMounted,onBeforeUnmount} from 'vue'
import {api,events} from '@/api'
import {findLogMatches, nextLogMatch} from '@/utils/log-search'
const props=defineProps({serverId:String,engine:String,container:Object})
const mode=ref('recent'),lines=ref(200),text=ref(''),error=ref(''),search=ref(''),searchOpen=ref(false),box=ref(null),searchRef=ref(null),follow=ref(true)
let id='',off=null,revision=0
const committedSearch=ref(''),activeStart=ref(-1),matchRef=ref(null)
const matches=computed(()=>findLogMatches(text.value,committedSearch.value))
const activeIndex=computed(()=>matches.value.findIndex(m=>m.start===activeStart.value))
const activeMatch=computed(()=>matches.value[activeIndex.value] || null)
async function revealMatch(){
 await nextTick()
 if(!box.value || !matchRef.value)return
 const viewport=box.value.getBoundingClientRect(), target=matchRef.value.getBoundingClientRect()
 box.value.scrollTop += target.top-viewport.top-box.value.clientHeight/2+target.height/2
}
function submitSearch(){
 committedSearch.value=search.value
 activeStart.value=matches.value.at(-1)?.start ?? -1
 follow.value=false
 revealMatch()
}
function moveMatch(direction){
 if(search.value!==committedSearch.value){submitSearch();return}
 const index=nextLogMatch(activeIndex.value,matches.value.length,direction)
 activeStart.value=matches.value[index]?.start ?? -1
 follow.value=false
 revealMatch()
}
function openSearch(){searchOpen.value=true;nextTick(()=>searchRef.value?.focus())}
function closeSearch(){searchOpen.value=false;committedSearch.value='';activeStart.value=-1}
function onScroll(){if(activeMatch.value)return;follow.value=box.value.scrollHeight-box.value.scrollTop-box.value.clientHeight<40}
function appendLog(chunk){
 const merged=text.value+chunk,removed=Math.max(0,merged.length-1024*1024)
 if(activeStart.value>=0)activeStart.value=Math.max(-1,activeStart.value-removed)
 text.value=merged.slice(-1024*1024)
 scroll()
}
watch(search,v=>{if(!v){committedSearch.value='';activeStart.value=-1}})
function stop(){revision++;off?.();off=null;if(id){window.go.main.App.CancelFileTransfer(id).catch(()=>{});id=''}}
async function scroll(){await nextTick();if(follow.value&&box.value)box.value.scrollTop=box.value.scrollHeight}
async function load(){stop();const rev=revision;text.value='';error.value='';follow.value=true;committedSearch.value='';activeStart.value=-1
 if(mode.value==='live'){
  id=crypto.randomUUID();off=events.on('container:logs:'+id,s=>{if(rev!==revision)return;appendLog(s)})
  try{await window.go.main.App.WatchContainerLogs(id,props.serverId,props.engine,props.container.ID,lines.value)}catch(e){if(rev===revision)error.value=String(e)}
 }else{
  try{const result=await api.runRemoteCommand(props.serverId,`${props.engine} logs --tail ${lines.value} --timestamps ${props.container.ID} 2>&1`,30);if(rev===revision){text.value=result;scroll()}}catch(e){if(rev===revision)error.value=String(e)}
 }
}
function find(e){if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='f'){e.preventDefault();openSearch()}}
watch([mode,lines,()=>props.container.ID],load,{immediate:true})
onMounted(()=>window.addEventListener('keydown',find))
onBeforeUnmount(()=>{stop();window.removeEventListener('keydown',find)})
</script>
<template><div class="logs" tabindex="0">
 <div class="log-overlays">
  <div class="log-tools"><el-radio-group v-model="mode" size="small"><el-radio-button value="live">实时</el-radio-button><el-radio-button value="recent">最近</el-radio-button></el-radio-group><el-select v-model="lines" size="small" style="width:95px"><el-option v-for="n in [200,500,1000]" :value="n" :label="n+' 行'" :key="n" /></el-select><el-button size="small" @click="searchOpen ? closeSearch() : openSearch()">搜索 Ctrl F</el-button><el-checkbox v-model="follow" @change="scroll">跟随</el-checkbox></div>
  <form v-if="searchOpen" class="log-search" role="search" @submit.prevent="submitSearch" @keydown.esc.stop="closeSearch">
   <el-input ref="searchRef" v-model="search" clearable size="small" placeholder="搜索日志" aria-label="搜索日志" class="search-input" @keydown.enter.prevent.stop="submitSearch" />
   <span class="match-count" aria-live="polite">{{committedSearch ? (activeIndex+1)+' / '+matches.length : ''}}</span>
   <el-button size="small" text :disabled="!matches.length" title="上一个匹配项" aria-label="上一个匹配项" @click="moveMatch(-1)"><el-icon><ArrowUp /></el-icon></el-button>
   <el-button size="small" text :disabled="!matches.length" title="下一个匹配项" aria-label="下一个匹配项" @click="moveMatch(1)"><el-icon><ArrowDown /></el-icon></el-button>
   <el-button size="small" type="primary" native-type="submit">搜索</el-button>
  </form>
 </div>
 <div v-if="error" class="log-error">{{error}}</div>
 <pre ref="box" class="log-content" @scroll="onScroll"><template v-if="activeMatch">{{text.slice(0,activeMatch.start)}}<mark ref="matchRef" class="log-match">{{text.slice(activeMatch.start,activeMatch.end)}}</mark>{{text.slice(activeMatch.end)}}</template><template v-else>{{text || '等待日志…'}}</template></pre>
 </div></template>
<style scoped>
.logs{height:100%;min-height:200px;position:relative;display:flex;flex-direction:column;background:var(--terminal-bg);color:var(--terminal-fg);border-radius:8px;overflow:hidden}
.log-overlays{position:absolute;top:8px;right:10px;z-index:2;display:flex;flex-direction:column;align-items:flex-end;gap:8px;max-width:calc(100% - 20px);pointer-events:none}
.log-tools,.log-search{pointer-events:auto;border:1px solid var(--line);border-radius:8px;background:var(--panel-2);box-shadow:var(--card-shadow);display:flex;align-items:center;gap:6px;padding:8px;max-width:100%;box-sizing:border-box}
.log-tools{justify-content:flex-end;flex-wrap:wrap}
.log-search{width:400px;color:var(--ink-2)}
.search-input{flex:1;min-width:80px}.match-count{font-size:11px;white-space:nowrap;font-variant-numeric:tabular-nums;color:var(--ink-3)}
.log-search .el-button{margin-left:0;flex-shrink:0}.log-search .el-button.is-text{padding:5px}
.log-content{flex:1;min-height:0;overflow:auto;margin:0;padding:64px 12px 12px;white-space:pre-wrap;overflow-wrap:anywhere;font:12px/1.6 Consolas,monospace}
.log-match{background:var(--yellow);color:var(--terminal-bg);outline:2px solid var(--yellow);border-radius:2px}
.log-error{padding:8px;color:var(--red)}
</style>
