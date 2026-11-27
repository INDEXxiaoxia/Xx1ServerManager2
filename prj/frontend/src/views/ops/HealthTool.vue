<script setup>
import {computed,ref} from 'vue'
import {api} from '@/api'
const props = defineProps({serverId:String})
const busy=ref(false), sections=ref([]), findings=ref([]), error=ref('')
const sampled=ref(''), readings=ref([])
const scope=[
  {title:'系统与硬件', detail:'主机型号、内核、处理器与内存',icon:'Monitor'},
  {title:'资源与参数', detail:'负载、挂载点、磁盘与内核参数',icon:'DataAnalysis'},
  {title:'服务与日志', detail:'失败服务、启动记录与系统警告',icon:'Document'}
]
const selected = ref('基本信息')
const detail = computed(()=>sections.value.find(s=>s.title===selected.value) || sections.value[0])
const detailIcons = ['Monitor','Cpu','DataAnalysis','Coin','Setting','Document']
const summary = computed(()=>{
  const lines = (detail.value?.text || '').split('\n')
  if(detail.value?.title==='基本信息'){
    const os = lines.find(l=>l.startsWith('PRETTY_NAME='))?.slice(12).replace(/^"|"$/g,'')
    return [['主机名称',lines[0]],['操作系统',os],['内核与架构',lines[1]]].filter(([,v])=>v)
  }
  if(detail.value?.title==='处理器与内存')return lines.flatMap(line=>{
    const m=line.match(/^(Architecture|Model name|CPU\(s\)|Thread\(s\) per core):\s*(.+)$/)
    return m?[[{'Architecture':'架构','Model name':'处理器','CPU(s)':'逻辑核心','Thread(s) per core':'每核线程'}[m[1]],m[2]]]:[]
  })
  return []
})
const command = `export LC_ALL=C
printf '==基本信息==\n'
hostname
uname -srmo
cat /etc/os-release 2>/dev/null
for f in sys_vendor product_name product_version; do cat /sys/class/dmi/id/$f 2>/dev/null; done
printf '==处理器与内存==\n'
lscpu 2>&1
free -h
printf '==资源指标==\n'
awk '/MemAvailable:/{a=$2}/MemTotal:/{t=$2} END{if(t&&a)printf "可用内存 %.1f%%\\n",100*a/t}' /proc/meminfo
awk -v n="$(getconf _NPROCESSORS_ONLN)" '{if(n>0)printf "每核一分钟负载 %.2f\\n",$1/n}' /proc/loadavg
df -P -x tmpfs -x devtmpfs
printf '==磁盘与系统参数==\n'
lsblk -o NAME,SIZE,TYPE,MOUNTPOINT
sysctl vm.swappiness vm.overcommit_memory fs.file-max net.core.somaxconn 2>&1
uptime
ss -s
printf '==服务与启动记录==\n'
systemctl --failed --no-pager 2>&1 | head -40
last reboot | head -8
printf '==系统警告与内核日志==\n'
journalctl -b -p warning --no-pager -n 50 2>&1
journalctl -k -b --no-pager -n 30 2>&1
true`
async function scan(){
 busy.value=true; error.value=''
 try {
   const out=await api.runRemoteCommand(props.serverId,command,45)
   sections.value=Array.from(out.matchAll(/^==(.+?)==\n([\s\S]*?)(?=^==|(?![\s\S]))/gm),m=>({title:m[1],text:m[2].trim()||'无可见记录'}))
   const metrics=sections.value.find(x=>x.title==='资源指标')?.text||''
   const mem=metrics.match(/可用内存 ([\d.]+)/),load=metrics.match(/每核一分钟负载 ([\d.]+)/)
   const disks=Array.from(metrics.matchAll(/^.*?\s(\d+)%\s+(.+)$/gm),m=>({percent:Number(m[1]),mount:m[2]})).sort((a,b)=>b.percent-a.percent)
   readings.value=[
     {title:'可用内存',value:mem?mem[1]+'%':'—',detail:'占物理内存比例',warn:mem&&Number(mem[1])<10},
     {title:'每核负载',value:load?load[1]:'—',detail:'最近一分钟',warn:load&&Number(load[1])>1},
     {title:'最高磁盘占用',value:disks.length?disks[0].percent+'%':'—',detail:disks[0]?.mount||'无挂载点数据',warn:disks[0]?.percent>=85}
   ]
   const notes=[]
   if(readings.value[0].warn)notes.push('可用内存低于 10%，建议查看内存占用最高的进程与 OOM 记录。')
   if(readings.value[1].warn)notes.push('每核负载超过 1，建议结合进程排行检查持续负载。')
   for(const disk of disks.filter(d=>d.percent>=85))notes.push(disk.mount+' 已用 '+disk.percent+'%，建议检查目录占用。')
   findings.value=notes
   sampled.value=new Date().toLocaleTimeString()
 }catch(e){error.value=String(e)}finally{busy.value=false}
}
</script>
<template>
  <section class="health">
    <header class="health-banner">
      <div class="health-emblem"><el-icon><Monitor /></el-icon></div>
      <div class="heading"><h2>{{ sampled ? '服务器检测报告' : '服务器检测' }}</h2>
        <span>{{ sampled ? '最近采样 · '+sampled : '读取系统状态，检查内存、负载和磁盘使用情况。' }}</span></div>
      <el-button type="primary" :loading="busy" @click="scan">{{ busy?'正在采集…':sampled?'重新检测':'开始检测' }}</el-button>
    </header>
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <div v-if="!sampled" class="tiles">
      <article v-for="item in scope" :key="item.title"><el-icon class="scope-icon"><component :is="item.icon"/></el-icon><h3>{{item.title}}</h3><span>{{item.detail}}</span></article>
    </div>
    <template v-else>
      <div class="tiles"><article v-for="r in readings" :key="r.title" :class="{warning:r.warn}">
        <span>{{r.title}}</span><strong>{{r.value}}</strong><small>{{r.detail}}</small></article></div>
      <section class="report"><h3>资源评估 <el-tag size="small" :type="findings.length?'warning':'info'">{{findings.length}} 项提醒</el-tag></h3>
        <div v-if="findings.length" class="notes"><p v-for="note in findings" :key="note"><el-icon><Warning /></el-icon>{{note}}</p></div>
        <span v-else>{{readings.some(r=>r.value==='—')?'部分指标缺失，可在下方查看采集输出。':'本次采样的内存、负载和磁盘均在设定阈值内。'}}</span>
      </section>
      <section class="inspector" v-if="detail">
        <nav aria-label="检测详情分类"><h3>检测详情</h3>
          <button v-for="(section,i) in sections" :key="section.title" :class="{chosen:section.title===detail.title}" @click="selected=section.title">
            <el-icon><component :is="detailIcons[i]" /></el-icon>{{section.title}}<el-icon><ArrowRight /></el-icon>
          </button>
        </nav>
        <article class="detail" :key="detail.title"><h3>{{detail.title}}</h3>
          <dl v-if="summary.length"><template v-for="[label,value] in summary" :key="label"><dt>{{label}}</dt><dd>{{value}}</dd></template></dl>
          <details v-if="summary.length"><summary>查看原始输出</summary><pre>{{detail.text}}</pre></details>
          <pre v-else>{{detail.text}}</pre>
        </article>
      </section>
    </template>
  </section>
</template>
<style scoped>
.health{--accent:var(--blue);--wash:var(--blue-soft);--radius:12px;display:grid;gap:14px;color:var(--ink)}
.health-banner,.tiles article,.report{border:1px solid var(--line);border-radius:var(--radius);background:var(--card)}
.health-banner{display:flex;flex-wrap:wrap;align-items:center;gap:18px;padding:clamp(18px,3vw,38px);background:linear-gradient(115deg,var(--wash),transparent 78%),var(--card);border-inline-start:4px solid var(--accent)}
.heading{flex:1 1 220px}.heading h2{margin:0 0 6px;font-size:clamp(18px,2vw,26px)}
.health-emblem,.scope-icon{color:var(--accent)}.health-emblem{display:grid;place-items:center;width:60px;height:60px;font-size:34px;border:1px solid var(--line);border-radius:var(--radius);background:var(--card)}
.tiles{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(185px,100%),1fr));gap:12px}
.tiles article,.report{padding:18px 20px;min-width:0}.tiles strong{display:block;font-size:30px;color:var(--accent);font-variant-numeric:tabular-nums}.tiles .warning strong,.notes i{color:var(--yellow)}
.health span,.health small{color:var(--ink-2);overflow-wrap:anywhere}.scope-icon{font-size:24px}
.health h3{font-size:15px;margin:10px 0 6px}.report h3{display:flex;gap:10px;align-items:center;margin:0 0 12px}
.notes{max-height:240px;overflow:auto}.notes p{display:flex;gap:10px;align-items:baseline;margin:0;padding:7px 0;overflow-wrap:anywhere}
pre{background:var(--panel-2);padding:14px;border-radius:6px;white-space:pre-wrap;overflow-wrap:anywhere;max-height:46vh;overflow:auto}
:root[data-theme="dark"] .health{--accent:#90b9eb;--wash:#273446}
:root[data-theme="cyber"] .health{--radius:3px;--accent:#54e6ed;--wash:#102d39}
:root[data-theme="sakura"] .health{--radius:16px;--accent:#97537c;--wash:#f5e1ed}

.health-banner{padding:20px 24px;border-inline-start-width:1px;background:linear-gradient(105deg,var(--wash),var(--card) 78%)}
.inspector{display:grid;grid-template-columns:minmax(165px,21%) minmax(0,1fr);border:1px solid var(--line);border-radius:var(--radius);background:var(--card);overflow:hidden}
.inspector nav{padding:12px;background:var(--panel-2);border-right:1px solid var(--line)}
.inspector h3{margin:0 0 12px}.inspector nav h3{padding:6px 8px;font-size:13px;color:var(--ink-2)}
.inspector button{border:1px solid transparent;border-radius:7px;display:flex;align-items:center;gap:9px;text-align:left;width:100%;padding:9px 10px;margin:2px 0;background:none;color:var(--ink-2);cursor:pointer;font:inherit}
.inspector button i:last-child{margin-left:auto;opacity:0}.inspector button.chosen i:last-child{opacity:1}
.inspector button:hover{background:var(--hover-bg)}.inspector button.chosen{color:var(--accent);background:var(--wash);border-color:var(--line);font-weight:600}
.detail{min-width:0;padding:20px clamp(14px,2vw,30px)}.detail dl{display:grid;grid-template-columns:max-content 1fr;margin:0}.detail dt,.detail dd{margin:0;padding:12px 0;border-bottom:1px solid var(--line)}.detail dt{color:var(--ink-3);padding-right:24px}.detail dd{overflow-wrap:anywhere}.detail details{margin-top:14px}.detail summary{cursor:pointer;color:var(--accent)}
:root[data-theme="cyber"] .inspector{box-shadow:inset 0 1px var(--accent)}:root[data-theme="sakura"] .health-banner{background:linear-gradient(110deg,var(--wash),var(--card) 65%);border-color:#d9b8cf}
@media(max-width:850px){.inspector{grid-template-columns:1fr}.inspector nav{display:flex;flex-wrap:wrap;gap:4px;border:0;border-bottom:1px solid var(--line)}.inspector nav h3{width:100%}.inspector nav button{width:auto}.inspector button i:last-child{display:none}}
</style>
