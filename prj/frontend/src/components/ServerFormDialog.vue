<script setup>
import {ref, reactive, watch, computed} from 'vue'
import {ElMessage} from 'element-plus'
import {api} from '@/api'
import {groupIdOf, groupIdsFor, orderedGroups} from '@/utils/groups'

const props = defineProps({
  modelValue: Boolean,
  server: {type: Object, default: null}
})
const emit = defineEmits(['update:modelValue', 'saved'])

// groupId：单选（一个服务器只能属于一个分组），空串 = 未分组
const form = reactive({id: '', name: '', host: '', port: 22, username: '', password: '', groupId: ''})
const groups = ref([])
const servers = ref([])
const testing = ref(false)

const groupOptions = computed(() => orderedGroups(servers.value, groups.value))

watch(() => props.modelValue, (v) => {
  if (v) {
    loadOptions()
    if (props.server && props.server.id) {
      Object.assign(form, {
        id: props.server.id, name: props.server.name, host: props.server.host,
        port: props.server.port, username: props.server.username,
        password: '', groupId: groupIdOf(props.server)
      })
    } else {
      Object.assign(form, {id: '', name: '', host: '', port: 22, username: '', password: '', groupId: ''})
    }
  }
})

async function loadOptions() {
  const [gs, svs] = await Promise.all([api.listGroups(), api.listServers()])
  groups.value = gs
  servers.value = svs
}

async function testConnection() {
  if (!form.host || !form.username || !form.password) {
    ElMessage.warning('请填写主机、用户名和密码')
    return
  }
  testing.value = true
  try {
    const r = await api.testConnection(form.host, Number(form.port), form.username, form.password)
    if (r.connected) ElMessage.success(r.message)
    else ElMessage.error(r.message)
  } catch (e) {
    ElMessage.error('测试失败：' + e)
  } finally {
    testing.value = false
  }
}

async function save() {
  if (!form.name || !form.host || !form.username) {
    ElMessage.warning('请填写名称、主机和用户名')
    return
  }
  try {
    const {groupId, ...rest} = form
    await api.saveServer({...rest, port: Number(form.port), groupIds: groupIdsFor(groupId)})
    ElMessage.success('保存成功')
    emit('update:modelValue', false)
    emit('saved')
  } catch (e) {
    ElMessage.error('保存失败：' + e)
  }
}
</script>

<template>
  <el-dialog :model-value="props.modelValue"
             :title="form.id ? '编辑服务器' : '新增服务器'"
             width="520px" @update:model-value="v => emit('update:modelValue', v)">
    <el-form label-width="90px">
      <el-form-item label="名称">
        <el-input v-model="form.name" placeholder="服务器名称" />
      </el-form-item>
      <el-form-item label="主机">
        <el-input v-model="form.host" placeholder="IP 地址或主机名" />
      </el-form-item>
      <el-form-item label="SSH 端口">
        <el-input-number v-model="form.port" :min="1" :max="65535" />
      </el-form-item>
      <el-form-item label="用户名">
        <el-input v-model="form.username" placeholder="登录用户名" />
      </el-form-item>
      <el-form-item label="密码">
        <el-input v-model="form.password" type="password" show-password
                  :placeholder="form.id ? '留空则保持原密码' : '登录密码'" />
      </el-form-item>
      <el-form-item label="所属分组">
        <el-select v-model="form.groupId" clearable placeholder="选择分组（可留空）" style="width: 100%">
          <el-option v-for="g in groupOptions" :key="g.id" :label="g.name" :value="g.id" :disabled="g.isUngrouped" />
        </el-select>
      </el-form-item>
      <div class="form-hint">一个服务器只能属于一个分组；留空即未分组。</div>
    </el-form>
    <template #footer>
      <el-button :loading="testing" @click="testConnection">测试连接</el-button>
      <el-button type="primary" @click="save">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.form-hint {
  margin: -6px 0 0 90px;
  font-size: 12px;
  color: var(--ink-3);
}
</style>