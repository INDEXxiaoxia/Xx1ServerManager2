<script setup>
import {ref, onMounted, onBeforeUnmount} from 'vue'
import {ElMessage} from 'element-plus'
import {api, events} from '@/api'
import ServerSelector from '@/components/ServerSelector.vue'

const selectedServers = ref([])
const command = ref('')
const timeout = ref(30)
const presets = ref([])
const presetSelected = ref(null)
const running = ref(false)
const outputs = ref([])
const summary = ref({total: 0, waiting: 0, running: 0, success: 0, failed: 0})
const sessionId = ref('')
const history = ref([])
const groups = ref([])

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
  if (p) command.value = p.command
}

function onOutput(data) {
  if (sessionId.value && data.sessionId !== sessionId.value) return
  const idx = outputs.value.findIndex(o => o.serverId === data.serverId)
  if (idx >= 0) outputs.value[idx] = data
  else outputs.value.push(data)
  updateSummary()
}

function onCompleted(sid) {
  if (sid === sessionId.value) {
    running.value = false
    load()
  }
}

function updateSummary() {
  summary.value.total = outputs.value.length
  summary.value.running = outputs.value.filter(o => o.status === 'running').length
  summary.value.success = outputs.value.filter(o => o.status === 'success').length
  summary.value.failed = outputs.value.filter(o => o.status === 'failed').length
}

async function start() {
  if (!selectedServers.value.length) { ElMessage.warning('请选择服务器'); return }
  if (!command.value.trim()) { ElMessage.warning('请输入命令'); return }
  running.value = true
  outputs.value = []
  summary.value = {total: 0, waiting: 0, running: 0, success: 0, failed: 0}
  try {
    sessionId.value = await api.startBatchCommand(
      selectedServers.value.map(s => s.id),
      command.value,
      Number(timeout.value)
    )
    ElMessage.success('命令已下发')
  } catch (e) {
    ElMessage.error('启动失败：' + e)
    running.value = false
  }
}

onMounted(() => {
  events.on('command:output', onOutput)
  events.on('command:completed', onCompleted)
})
onBeforeUnmount(() => {
  events.off('command:output')
  events.off('command:completed')
})
</script>

<template>
  <div class="page-container">
    <div class="layout-container">
      <!-- 左侧：服务器多选 -->
      <div class="left-panel">
        <div class="card selector-card">
          <ServerSelector v-model:selected-servers="selectedServers" />
        </div>
      </div>

      <!-- 右侧：命令执行 -->
      <div class="right-panel">
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
    </div>
  </div>
</template>

<style scoped>
.layout-container {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}
.left-panel {
  width: 320px;
  flex-shrink: 0;
  position: sticky;
  top: 0;
}
.selector-card {
  padding: 12px;
  height: calc(100vh - 160px);
}
.right-panel { flex: 1; min-width: 0; }
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