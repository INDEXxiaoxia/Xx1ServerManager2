<script setup>
import {ref, reactive, onMounted} from 'vue'
import {ElMessage, ElMessageBox} from 'element-plus'
import {api} from '@/api'

const list = ref([])
const dialogVisible = ref(false)
const editing = ref(null)
const form = reactive({id: '', name: '', description: '', color: '#5780A9'})

const colors = ['#5780A9', '#C58A98', '#C79E4F', '#67947A', '#8F919B', '#9B7FB8', '#6FAE9F', '#D08770']

async function load() {
  list.value = await api.listGroups()
}
onMounted(load)

function openAdd() {
  Object.assign(form, {id: '', name: '', description: '', color: '#5780A9'})
  dialogVisible.value = true
}
function openEdit(g) {
  Object.assign(form, {id: g.id, name: g.name, description: g.description, color: g.color || '#5780A9'})
  dialogVisible.value = true
}

async function save() {
  if (!form.name) { ElMessage.warning('请输入分组名称'); return }
  await api.saveGroup({...form})
  ElMessage.success('保存成功')
  dialogVisible.value = false
  load()
}

async function remove(g) {
  await ElMessageBox.confirm(`确定删除分组「${g.name}」吗？`, '提示', {type: 'warning'})
  await api.deleteGroup(g.id)
  ElMessage.success('已删除')
  load()
}
</script>

<template>
  <div class="page-container">
    <div class="card">
      <div class="flex-between mb-12">
        <h3>分组管理</h3>
        <el-button type="primary" @click="openAdd">新增分组</el-button>
      </div>
      <el-table :data="list" stripe>
        <el-table-column label="颜色" width="80">
          <template #default="{row}">
            <span class="color-dot" :style="{background: row.color || '#5780A9'}" />
          </template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="140" />
        <el-table-column prop="description" label="描述" min-width="200" show-overflow-tooltip />
        <el-table-column label="操作" width="140">
          <template #default="{row}">
            <el-button size="small" text type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" text type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑分组' : '新增分组'" width="420px">
      <el-form label-width="70px">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="描述"><el-input v-model="form.description" /></el-form-item>
        <el-form-item label="颜色">
          <div class="color-picker">
            <span v-for="c in colors" :key="c" class="color-dot selectable"
                  :class="{active: form.color === c}" :style="{background: c}" @click="form.color = c" />
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.color-dot {
  display: inline-block;
  width: 18px;
  height: 18px;
  border-radius: 50%;
}
.color-picker .color-dot {
  width: 26px;
  height: 26px;
  margin-right: 8px;
  cursor: pointer;
  border: 2px solid transparent;
}
.color-picker .color-dot.active {
  border-color: transparent;
  box-shadow: 0 0 0 3px var(--card), 0 0 0 5px var(--ink-2);
}
</style>