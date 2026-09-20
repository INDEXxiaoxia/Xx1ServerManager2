<script setup>
import {ref, reactive, onMounted, onErrorCaptured} from 'vue'
import {ElMessage, ElMessageBox} from 'element-plus'
import {api} from '@/api'

const tab = ref('commands')
const commands = ref([])
const paths = ref([])
const dialogVisible = ref(false)
const editingType = ref('command')
const form = reactive({id: '', name: '', command: '', path: '', description: '', category: ''})
const pageError = ref('')

const categories = ['系统', '网络', '服务', '磁盘', '日志', '其他']

// 组件内错误兜底：渲染错误直接显示在页面内，便于定位
onErrorCaptured((err, instance, info) => {
  pageError.value = `${err && err.message ? err.message : err}\n[${info}]`
  return false
})

async function load() {
  try {
    commands.value = (await api.listPresetCommands()) || []
    paths.value = (await api.listPresetPaths()) || []
  } catch (e) {
    pageError.value = '加载预设数据失败: ' + e
  }
}
onMounted(load)

function openAdd(type) {
  editingType.value = type
  Object.assign(form, {id: '', name: '', command: '', path: '', description: '', category: '其他'})
  dialogVisible.value = true
}
function openEdit(item, type) {
  editingType.value = type
  Object.assign(form, {
    id: item.id, name: item.name,
    command: item.command || '', path: item.path || '',
    description: item.description || '', category: item.category || '其他'
  })
  dialogVisible.value = true
}

async function save() {
  if (!form.name) { ElMessage.warning('请输入名称'); return }
  if (editingType.value === 'command') {
    if (!form.command) { ElMessage.warning('请输入命令'); return }
    await api.savePresetCommand({id: form.id, name: form.name, command: form.command, description: form.description, category: form.category})
  } else {
    if (!form.path) { ElMessage.warning('请输入路径'); return }
    await api.savePresetPath({id: form.id, name: form.name, path: form.path, description: form.description})
  }
  ElMessage.success('保存成功')
  dialogVisible.value = false
  load()
}

async function remove(type, item) {
  await ElMessageBox.confirm(`确定删除「${item.name}」吗？`, '提示', {type: 'warning'})
  if (type === 'command') await api.deletePresetCommand(item.id)
  else await api.deletePresetPath(item.id)
  ElMessage.success('已删除')
  load()
}
</script>

<template>
  <div class="page-container">
    <div class="card">
      <!-- 错误兜底显示 -->
      <div v-if="pageError" class="page-error">{{ pageError }}</div>

      <!-- 工具行：切换 + 新建 -->
      <div class="flex-between mb-12">
        <div class="tab-buttons" role="tablist">
          <button type="button" class="tab-button" :class="{active: tab === 'commands'}" @click="tab = 'commands'">预设命令</button>
          <button type="button" class="tab-button" :class="{active: tab === 'paths'}" @click="tab = 'paths'">预设路径</button>
        </div>
        <el-button v-if="tab === 'commands'" type="primary" @click="openAdd('command')">新建命令</el-button>
        <el-button v-else type="primary" @click="openAdd('path')">新建路径</el-button>
      </div>

      <!-- 预设命令 -->
      <div v-show="tab === 'commands'" class="tab-panel">
        <el-table :data="commands" stripe>
          <el-table-column prop="name" label="名称" min-width="120" />
          <el-table-column prop="category" label="分类" width="90">
            <template #default="{row}"><el-tag size="small">{{ row.category || '其他' }}</el-tag></template>
          </el-table-column>
          <el-table-column prop="command" label="命令" min-width="260" show-overflow-tooltip class-name="mono" />
          <el-table-column prop="description" label="描述" min-width="160" show-overflow-tooltip />
          <!-- 操作列留足宽度（约可容纳 3 个按钮），保证按钮始终不换行 -->
          <el-table-column label="操作" width="220" fixed="right">
            <template #default="{row}">
              <div class="op-bar">
                <el-button size="small" text type="primary" @click="openEdit(row, 'command')">编辑</el-button>
                <el-button size="small" text type="danger" @click="remove('command', row)">删除</el-button>
              </div>
            </template>
          </el-table-column>
        </el-table>
        <div class="count">共 {{ commands.length }} 条</div>
      </div>

      <!-- 预设路径 -->
      <div v-show="tab === 'paths'" class="tab-panel">
        <el-table :data="paths" stripe>
          <el-table-column prop="name" label="名称" min-width="120" />
          <el-table-column prop="path" label="路径" min-width="260" show-overflow-tooltip class-name="mono" />
          <el-table-column prop="description" label="描述" min-width="160" show-overflow-tooltip />
          <!-- 操作列留足宽度（约可容纳 3 个按钮），保证按钮始终不换行 -->
          <el-table-column label="操作" width="220" fixed="right">
            <template #default="{row}">
              <div class="op-bar">
                <el-button size="small" text type="primary" @click="openEdit(row, 'path')">编辑</el-button>
                <el-button size="small" text type="danger" @click="remove('path', row)">删除</el-button>
              </div>
            </template>
          </el-table-column>
        </el-table>
        <div class="count">共 {{ paths.length }} 条</div>
      </div>
    </div>

    <el-dialog v-model="dialogVisible"
               :title="editingType === 'command' ? '预设命令' : '预设路径'" width="480px">
      <el-form label-width="70px">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item v-if="editingType === 'command'" label="分类">
          <el-select v-model="form.category" style="width: 100%">
            <el-option v-for="c in categories" :key="c" :label="c" :value="c" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="editingType === 'command'" label="命令">
          <el-input v-model="form.command" type="textarea" :rows="3" class="mono" />
        </el-form-item>
        <el-form-item v-else label="路径">
          <el-input v-model="form.path" class="mono" placeholder="/var/log" />
        </el-form-item>
        <el-form-item label="描述"><el-input v-model="form.description" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.tab-buttons {
  display: inline-flex;
  gap: 2px;
  padding: 3px;
  background: var(--fill-color);
  border-radius: 999px;
}
.tab-button {
  border: none;
  outline: none;
  cursor: pointer;
  background: transparent;
  padding: 7px 18px;
  border-radius: 999px;
  font-size: 13px;
  color: var(--ink-2);
  font-family: inherit;
  transition: background-color .25s ease, color .25s ease, box-shadow .25s ease;
}
.tab-button:hover {
  color: var(--ink);
}
.tab-button.active {
  background: var(--card);
  color: var(--blue-deep);
  font-weight: 600;
  box-shadow: var(--card-shadow);
}
.tab-panel {
  padding-top: 4px;
}
.op-bar {
  display: flex;
  align-items: center;
  white-space: nowrap;
}
.op-bar .el-button {
  padding-left: 6px;
  padding-right: 6px;
  margin-left: 0;
}
.count {
  margin-top: 10px;
  color: var(--ink-3);
  font-size: 13px;
}
.page-error {
  background: var(--red-soft);
  color: var(--red);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px;
  padding: 10px 12px;
  margin-bottom: 12px;
  font-family: Consolas, Menlo, monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>