<script setup>
import {ordered, dragRow} from '@/utils/list-order'

import {computed} from 'vue'
const props=defineProps({stats:{type:Array,default:()=>[]},containers:{type:Array,default:()=>[]}})
const cards=computed(()=>props.containers.map(c=>{
 const s=props.stats.find(s=>s.Name===c.Names || (s.ID && c.ID.startsWith(s.ID)) || (s.Container && c.ID.startsWith(s.Container))) || {}
 return {...s,id:c.ID,name:c.Names,image:c.Image,state:c.Status,running:c.State==='running' || (!c.State && /^Up\b/.test(c.Status||''))}
}))
function percent(v){const n=parseFloat(v);return Number.isFinite(n)?Math.max(0,Math.min(100,n)):0}
function color(v){const n=percent(v);return n>=90?'var(--red)':n>=70?'var(--yellow)':'var(--blue)'}
</script>
<template>
 <div class="dashboard-summary"><span>共 <b>{{cards.length}}</b> 个容器</span><span class="running">运行中 {{cards.filter(c=>c.running).length}}</span><span>已停止 / 其他 {{cards.filter(c=>!c.running).length}}</span></div>
 <el-empty v-if="!cards.length" description="该服务器暂无容器" />
 <div class="container-dashboard">
  <article v-for="c in ordered(cards, 'containerCards')" v-bind="dragRow(cards, 'containerCards', c)" :key="c.id" class="container-gauge-card">
   <header><div class="container-identity"><strong :title="c.name">{{c.name}}</strong><small :title="c.image">{{c.image}}</small></div><el-tag :type="c.running?'success':'info'" size="small">{{c.running?'运行中':'已停止 / 其他'}}</el-tag></header>
   <div class="state-line">{{c.state}}</div>
   <div class="gauges">
    <div><el-progress type="dashboard" :width="116" :stroke-width="8" :percentage="percent(c.CPUPerc)" :color="color(c.CPUPerc)"><template #default><strong>{{c.CPUPerc || '—'}}</strong></template></el-progress><div class="gauge-label">CPU 使用率</div></div>
    <div><el-progress type="dashboard" :width="116" :stroke-width="8" :percentage="percent(c.MemPerc)" :color="color(c.MemPerc)"><template #default><strong>{{c.MemPerc || '—'}}</strong></template></el-progress><div class="gauge-label">内存使用率</div></div>
   </div>
   <dl><div><dt>内存 / 限额</dt><dd>{{c.MemUsage || '—'}}</dd></div><div><dt>网络 收 / 发</dt><dd>{{c.NetIO || '—'}}</dd></div><div><dt>磁盘 读 / 写</dt><dd>{{c.BlockIO || '—'}}</dd></div><div><dt>进程数</dt><dd>{{c.PIDs || '—'}}</dd></div></dl>
   <small v-if="parseFloat(c.CPUPerc)>100" class="multicore">多核 CPU 可超过 100%，仪表显示上限为 100%。</small>
  </article>
 </div>
</template>
<style scoped>
.dashboard-summary{display:flex;gap:20px;flex-wrap:wrap;margin:0 0 20px;color:var(--ink-2)}.running{color:var(--green)}
.container-dashboard{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,310px),1fr));gap:16px;align-items:start}
.container-gauge-card{border:1px solid var(--line);border-radius:14px;padding:18px;background:var(--card);box-shadow:var(--card-shadow);min-width:0}
header{display:flex;align-items:center;gap:12px}.container-identity{flex:1;min-width:0}.container-identity strong,.container-identity small{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.container-identity strong{color:var(--ink);font-size:15px}.container-identity small{color:var(--ink-3);margin-top:5px}.state-line{font-size:12px;color:var(--ink-3);margin-top:12px;min-height:18px}
.gauges{display:flex;justify-content:space-evenly;gap:12px;padding:16px 0}.gauges strong{font-size:19px;color:var(--ink)}.gauge-label{text-align:center;font-size:12px;color:var(--ink-2);margin-top:-15px}dl{margin:12px 0 0}dl>div{display:flex;gap:10px;justify-content:space-between;padding:9px 0;border-top:1px solid var(--line);font-size:12px}dt{color:var(--ink-3);flex-shrink:0}dd{margin:0;color:var(--ink-2);text-align:right;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.multicore{display:block;color:var(--ink-3);margin-top:8px}
</style>
