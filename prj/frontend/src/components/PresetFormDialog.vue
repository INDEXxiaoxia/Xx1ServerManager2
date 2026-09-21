<script setup>
import {ref, reactive, watch, computed} from 'vue'
import {ElMessage} from 'element-plus'
import {api} from '@/api'

// 预设命令 / 预设路径 共用的新增·编辑对话框。
// 终端快捷侧边栏与「预设管理」页都用它，避免两处各写一套表单。
const props = defineProps({
  modelValue: {type: Boolean, default: false},
  // 'command' | 'path'
  type: {type: String, default: 'command'},
  // 编辑时传入原条目；为空表示新增
  item: {type: Object, default: null},
  // 新增时的默认分类（终端侧栏按当前筛选分类带入）
  defaultCategory: {type: String, default: '其他'}
})
const emit = defineEmits(['update:modelValue', 'saved'])

const CATEGORIES = ['系统', '网络', '服务', '磁盘', '日志', '其他']

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const form = reactive({id: '', name: '', command: '', path: '', description: '', category: '其他'})
const saving = ref(false)

const isCommand = computed(() => props.type === 'command')
const title = computed(() => (isCommand.value ? '预设命令' : '预设路径'))

watch(visible, (v) => {
  if (!v) return
  const it = props.item
  if (it) {
    Object.assign(form, {
      id: it.id || '',
      name: it.name || '',
      command: it.command || '',
      path: it.path || '',
      description: it.description || '',
      category: it.category || '其他'
    })
  } else {
    Object.assign(form, {
      id: '', name: '', command: '', path: '', description: '',
      category: props.defaultCategory || '其他'
    })
  }
})

async function save() {
  const name = form.name.trim()
  if (!name) { ElMessage.warning('请输入名称'); return }

  if (isCommand.value) {
    const command = form.command.trim()
    if (!command) { ElMessage.warning('请输入命令'); return }
    saving.value = true
    try {
      await api.savePresetCommand({
        id: form.id, name, command,
        description: form.description.trim(),
        category: form.category || '其他'
      })
    } catch (e) {
      ElMessage.error('保存失败：' + e)
      saving.value = false
      return
    }
  } else {
    const path = form.path.trim()
    if (!path) { ElMessage.warning('请输入路径'); return }
    if (!path.startsWith('/') && !path.startsWith('~')) {
      ElMessage.warning('路径建议以 / 或 ~ 开头')
      return
    }
    saving.value = true
    try {
      await api.savePresetPath({
        id: form.id, name, path,
        description: form.description.trim()
      })
    } catch (e) {
      ElMessage.error('保存失败：' + e)
      saving.value = false
      return
    }
  }

  saving.value = false
  ElMessage.success(form.id ? '已更新' : '已添加')
  visible.value = false
  emit('saved')
}
</script>

<template>
  <el-dialog v-model="visible" :title="(form.id ? '编辑' : '新增') + title" width="460px" append-to-body>
    <el-form label-width="70px">
      <el-form-item label="名称">
        <el-input v-model="form.name" :placeholder="isCommand ? '如：查看磁盘占用' : '如：应用日志目录'" />
      </el-form-item>

      <el-form-item v-if="isCommand" label="分类">
        <el-select v-model="form.category" style="width: 100%">
          <el-option v-for="c in CATEGORIES" :key="c" :label="c" :value="c" />
        </el-select>
      </el-form-item>

      <el-form-item v-if="isCommand" label="命令">
        <el-input
          v-model="form.command"
          type="textarea"
          :rows="3"
          class="mono"
          placeholder="如：df -h"
        />
      </el-form-item>

      <el-form-item v-else label="路径">
        <el-input v-model="form.path" class="mono" placeholder="/var/log" />
      </el-form-item>

      <el-form-item label="描述">
        <el-input v-model="form.description" placeholder="可选" />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
</style>