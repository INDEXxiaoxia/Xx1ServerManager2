<script setup>
import {themeState} from '@/theme'
import {BellFilled} from '@element-plus/icons-vue'
import {ordered, dragRow} from '@/utils/list-order'

import {computed, ref, onMounted} from 'vue'
import {useTransfers} from '@/store/transfers'
import {useBatchStore} from '@/store/batch'
const batch = useBatchStore()
const batchTasks = computed(()=>Object.values(batch.uploadTasks || {}))
const store = useTransfers()
onMounted(() => store.bind())
const active = computed(() => store.tasks.filter(t => ['queued','running','cancelling'].includes(t.status)).length + batchTasks.value.filter(t=>['running','waiting'].includes(t.status)).length)
const page = ref(1)
const rows = computed(() => [...batchTasks.value.map(t=>({...t,batch:true,id:t.taskId})), ...ordered(store.tasks,'transfers')])
const visibleRows = computed(()=>rows.value.slice((page.value-1)*40,page.value*40))
function clearFinished(){
  store.tasks = store.tasks.filter(t => ['queued','running','cancelling'].includes(t.status))
  batch.uploadTasks = Object.fromEntries(Object.entries(batch.uploadTasks).filter(([,t])=>['running','waiting'].includes(t.status)))
  page.value = 1
}
const names = {queued:'排队中',running:'上传中',cancelling:'取消中',cancelled:'已取消',success:'完成',failed:'失败'}
function bytes(n) { return n >= 1048576 ? (n/1048576).toFixed(1)+' MB' : (n/1024).toFixed(1)+' KB' }
</script>
<template>
  <button v-if="store.tasks.length || batchTasks.length" class="transfer-float" @click="store.visible = true"><el-icon v-if="themeState.name === 'sakura'"><BellFilled /></el-icon><template v-else>↑</template> 上传任务 {{ active ? '('+active+')' : '' }}</button>
  <el-drawer v-model="store.visible" title="上传任务" size="420px" append-to-body>
    <p class="text-muted">切换页面不影响上传。失败可重新上传；关闭应用会中断任务。</p>
    <el-button size="small" @click="clearFinished">清除已结束任务</el-button>
    <el-pagination v-if="rows.length>40" v-model:current-page="page" :page-size="40" :total="rows.length" layout="prev,pager,next" />
    <template v-for="t in visibleRows" :key="t.id"><div v-if="t.batch" class="transfer-item"><strong>{{t.serverName}} · {{t.status}}</strong><div class="transfer-path">{{t.remotePath}}</div><el-progress :percentage="Math.round(t.progress || 0)" /><small>{{t.errorMsg}}</small></div>
    <div v-else v-bind="dragRow(store.tasks, 'transfers', t)" :key="t.id" class="transfer-item" :data-status="t.status">
      <strong>{{ t.serverName }} · {{ names[t.status] }}</strong>
      <div class="transfer-path">{{ t.remotePath }}</div>
      <el-progress :percentage="t.status === 'success' ? 100 : t.total ? Math.min(99,Math.round(t.transferred/t.total*100)) : 0" :status="t.status === 'success' ? 'success' : t.status === 'failed' ? 'exception' : undefined" />
      <small>{{ bytes(t.transferred) }} / {{ bytes(t.total) }} · {{ bytes(t.speed) }}/s <span v-if="t.speed > 0">· 剩余约 {{ Math.ceil((t.total-t.transferred)/t.speed) }} 秒</span></small>
      <div v-if="t.error" class="transfer-path">{{ t.error }}</div>
      <el-button v-if="t.status === 'queued' || (t.status === 'running' && !t.directory)" size="small" @click="store.cancel(t)">取消</el-button>
      <el-button v-if="['failed','cancelled'].includes(t.status)" size="small" @click="store.retry(t)">重新上传</el-button>
    </div>
  </template>
  </el-drawer>
</template>
<style scoped>
.transfer-float {position:fixed;bottom:20px;right:24px;z-index:1900;background:var(--card);color:var(--blue);border:1px solid var(--line);border-radius:24px;padding:10px 16px;box-shadow:var(--card-shadow);cursor:pointer}
.transfer-item {padding:16px 0;border-bottom:1px solid var(--line)}
.transfer-path {overflow-wrap:anywhere;margin:8px 0;color:var(--ink-2)}
</style>
