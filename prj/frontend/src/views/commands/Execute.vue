<script setup>
import {ref, computed, watch, onMounted} from 'vue'
import {ElMessage} from 'element-plus'
import {api} from '@/api'
import {useBatchStore} from '@/store/batch'

// 服务器选择由批量操作页统一提供（左侧面板）
const props = defineProps({
  selectedServers: {type: Array, default: () => []}
})

const store = useBatchStore()
const selectedServers = computed(() => props.selectedServers || [])

const presets = ref([])
const presetSelected = ref(null)
const history = ref([])
const groups = ref([])

// 命令内容、超时、执行结果都保存在 store 中，切 tab / 切页返回后依然保留
const command = computed({
  get: () => store.executeCommand,
  set: (v) => { store.executeCommand = v }
})
const timeout = computed({
  get: () => store.executeTimeout,
  set: (v) => { store.executeTimeout = v }
})
const outputs = computed(() => store.executeList)
const summary = computed(() => store.executeSummary)
const running = computed(() => store.executeRunning)

async function load() {
  presets.value = await api.listPresetCommands()
  history.value = await api.getCommandHistory(20)
  groups.value = await api.listGroups()
}
onMounted(load)

function getServerGroupColor(server) {
  if (!server.groupIds || !server.groupIds.length) return 'var(--ink-3)'
  const g = groups.value.find(x => x.id === server.groupIds[0])
  return g?.color || 'var(--ink-3)'
}

function usePreset(p) {
  if (p) store.executeCommand = p.command
}

async function start() {
  if (!selectedServers.value.length) { ElMessage.warning('请选择服务器'); return }
  if (!command.value.trim()) { ElMessage.warning('请输入命令'); return }
  try {
    const sid = await api.startBatchCommand(
      selectedServers.value.map(s => s.id),
      command.value,
      Number(timeout.value)
    )
    store.setExecuteSession(sid)
    ElMessage.success('命令已下发')
  } catch (e) {
    ElMessage.error('启动失败：' + e)
  }
}

// 执行结束后刷新历史记录（store 中的事件监听负责收敛 running 状态）
watch(running, (v, old) => {
  if (old && !v) load()
})
</script>

<template>
  <div class="execute-panel">
    <div class="card mb-12">
      <h3 class="card-title">批量命令执行</h3>
      <el-alert v-if="!selectedServers.length" title="请在左侧选择目标服务器" type="warning" show-icon :closable="false" class="mt-12" />
          <template v-else>
            <div class="mt-12">
              <span class="text-muted">目标服务器：</span>
              <el-tag v-for="s in selectedServers" :key="s.id" class="server-tag"
                      :style="{borderColor: getServerGroupColor(s), borderWidth: '2px'}">
                {{ s.name }} ({{ s.host }})
              </el-tag>
            </div>
            <el-form label-width="90px" class="mt-12">
              <el-form-item label="预设命令">
                <el-select v-model="presetSelected" placeholder="选择预设命令填充" clearable style="width: 100%" @change="p => p && usePreset(p)">
                  <el-option v-for="p in presets" :key="p.id" :label="`${p.name} - ${p.command}`" :value="p" />
                </el-select>
              </el-form-item>
              <el-form-item label="执行命令">
                <el-input v-model="command" type="textarea" :rows="5" class="mono" placeholder="例如：uptime" />
              </el-form-item>
              <el-form-item label="超时(秒)">
                <el-input-number v-model="timeout" :min="5" :max="300" />
              </el-form-item>
              <el-form-item>
                <el-button type="primary" :loading="running" @click="start">开始执行</el-button>
              </el-form-item>
            </el-form>
          </template>
    </div>

        <div class="card mb-12" v-if="outputs.length">
          <div class="flex-between">
            <h3 class="card-title">执行结果</h3>
            <div>
              <el-tag type="success" class="tag-item">成功 {{ summary.success }}</el-tag>
              <el-tag type="danger" class="tag-item">失败 {{ summary.failed }}</el-tag>
              <el-tag type="warning">执行中 {{ summary.running }}</el-tag>
            </div>
          </div>
          <el-row :gutter="12" class="mt-12">
            <el-col v-for="o in outputs" :key="o.serverId" :span="12" class="mb-12">
              <div class="output-card">
                <div class="flex-between">
                  <span class="server-name">{{ o.serverName }}</span>
                  <el-tag :type="o.status === 'success' ? 'success' : (o.status === 'failed' ? 'danger' : 'warning')" size="small">
                    {{ o.status }}
                  </el-tag>
                </div>
                <div class="mono output-text" v-if="o.stdout">{{ o.stdout }}</div>
                <div class="mono output-text err" v-if="o.stderr">{{ o.stderr }}</div>
                <div class="mono output-text err" v-if="o.errorMsg">{{ o.errorMsg }}</div>
                <div class="text-muted" v-if="o.duration">耗时 {{ o.duration }} ms</div>
              </div>
            </el-col>
          </el-row>
        </div>

        <div class="card">
          <h3 class="card-title">最近执行历史</h3>
          <el-table :data="history" stripe size="small" class="mt-12">
            <el-table-column prop="serverName" label="服务器" width="130" />
            <el-table-column prop="command" label="命令" min-width="200" show-overflow-tooltip class-name="mono" />
            <el-table-column label="状态" width="90">
              <template #default="{row}">
                <el-tag :type="row.status === 'success' ? 'success' : 'danger'" size="small">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="exitCode" label="退出码" width="80" />
            <el-table-column prop="duration" label="耗时(ms)" width="90" />
            <el-table-column prop="createdAt" label="时间" width="170" />
          </el-table>
        </div>
  </div>
</template>

<style scoped>
/* 服务器选择在左侧独立面板，这里只渲染右侧内容 */
.execute-panel { min-width: 0; }
.card-title {
  position: relative;
  display: inline-block;
  padding-left: 12px;
  font-size: 16px;
  font-weight: 600;
  margin: 0;
}
.card-title::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 14px;
  border-radius: 999px;
  background: linear-gradient(180deg, var(--blue), var(--pink));
}
.tag-item { margin-right: 8px; margin-bottom: 4px; }
.server-tag {
  margin-right: 8px;
  margin-bottom: 4px;
  border-style: solid;
}
.mr-1 { margin-right: 8px; }
.output-card {
  border: 1px solid var(--line);
  border-radius: 14px;
  padding: 12px 14px;
  background: var(--panel-2);
}
.server-name { font-weight: 600; }
.output-text {
  margin-top: 8px;
  background: var(--terminal-bg);
  color: var(--terminal-fg);
  border-radius: 10px;
  padding: 10px 12px;
  max-height: 180px;
  overflow-y: auto;
}
.output-text.err { color: var(--err-fg); }
</style>