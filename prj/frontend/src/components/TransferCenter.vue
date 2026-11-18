<script setup>
import {computed, onMounted} from 'vue'
import {useTransfers} from '@/store/transfers'
const store = useTransfers()
onMounted(() => store.bind())
const active = computed(() => store.tasks.filter(t => ['queued','running','cancelling'].includes(t.status)).length)
const names = {queued:'排队中',running:'上传中',cancelling:'取消中',cancelled:'已取消',success:'完成',failed:'失败'}
function bytes(n) { return n >= 1048576 ? (n/1048576).toFixed(1)+' MB' : (n/1024).toFixed(1)+' KB' }
</script>
<template>
  <button v-if="store.tasks.length" class="transfer-float" @click="store.visible = true">↑ 上传任务 {{ active ? '('+active+')' : '' }}</button>
  <el-drawer v-model="store.visible" title="上传任务" size="420px" append-to-body>
    <p class="text-muted">切换页面不影响上传。失败可重新上传；关闭应用会中断任务。</p>
    <el-button size="small" @click="store.tasks = store.tasks.filter(t => ['queued','running','cancelling'].includes(t.status))">清除已结束任务</el-button>
    <div v-for="t in store.tasks" :key="t.id" class="transfer-item">
      <strong>{{ t.serverName }} · {{ names[t.status] }}</strong>
      <div class="transfer-path">{{ t.remotePath }}</div>
      <el-progress :percentage="t.status === 'success' ? 100 : t.total ? Math.min(99,Math.round(t.transferred/t.total*100)) : 0" :status="t.status === 'success' ? 'success' : t.status === 'failed' ? 'exception' : undefined" />
      <small>{{ bytes(t.transferred) }} / {{ bytes(t.total) }} · {{ bytes(t.speed) }}/s <span v-if="t.speed > 0">· 剩余约 {{ Math.ceil((t.total-t.transferred)/t.speed) }} 秒</span></small>
      <div v-if="t.error" class="transfer-path">{{ t.error }}</div>
      <el-button v-if="['queued','running'].includes(t.status)" size="small" @click="store.cancel(t)">取消</el-button>
      <el-button v-if="['failed','cancelled'].includes(t.status)" size="small" @click="store.retry(t)">重新上传</el-button>
    </div>
  </el-drawer>
</template>
<style scoped>
.transfer-float {position:fixed;bottom:20px;right:24px;z-index:1900;background:var(--card);color:var(--blue);border:1px solid var(--line);border-radius:24px;padding:10px 16px;box-shadow:var(--card-shadow);cursor:pointer}
.transfer-item {padding:16px 0;border-bottom:1px solid var(--line)}
.transfer-path {overflow-wrap:anywhere;margin:8px 0;color:var(--ink-2)}
</style>
