<script setup>
import {ref, reactive, onMounted} from 'vue'
import {ElMessage} from 'element-plus'
import {api} from '@/api'

const settings = reactive({maxConcurrent: 5, uploadTimeout: 300, commandTimeout: 30, theme: 'light'})
const masterSet = ref(false)
const pwDialog = ref(false)
const pw = reactive({p1: '', p2: ''})
const saving = ref(false)

async function load() {
  const s = await api.getSettings()
  Object.assign(settings, s)
  masterSet.value = await api.isMasterPasswordSet()
}
onMounted(load)

async function save() {
  saving.value = true
  try {
    await api.saveSettings({...settings})
    ElMessage.success('设置已保存')
  } catch (e) {
    ElMessage.error('保存失败：' + e)
  } finally {
    saving.value = false
  }
}

function openPwDialog() {
  pw.p1 = ''
  pw.p2 = ''
  pwDialog.value = true
}

async function setPassword() {
  if (pw.p1.length < 6) { ElMessage.warning('主密码至少 6 位'); return }
  if (pw.p1 !== pw.p2) { ElMessage.error('两次密码不一致'); return }
  try {
    await api.setMasterPassword(pw.p1)
    masterSet.value = true
    pwDialog.value = false
    ElMessage.success('主密码设置成功')
  } catch (e) {
    ElMessage.error('设置失败：' + e)
  }
}
</script>

<template>
  <div class="page-container">
    <div class="card mb-12">
      <h3>常规设置</h3>
      <el-form label-width="120px" class="mt-12">
        <el-form-item label="最大并发数">
          <el-input-number v-model="settings.maxConcurrent" :min="1" :max="20" />
        </el-form-item>
        <el-form-item label="上传超时(秒)">
          <el-input-number v-model="settings.uploadTimeout" :min="60" :max="3600" />
        </el-form-item>
        <el-form-item label="命令超时(秒)">
          <el-input-number v-model="settings.commandTimeout" :min="5" :max="300" />
        </el-form-item>
        <el-form-item label="主题">
          <el-select v-model="settings.theme" style="width: 200px">
            <el-option label="浅色" value="light" />
            <el-option label="深色" value="dark" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="save">保存设置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <div class="card">
      <h3>安全</h3>
      <div class="flex-between mt-12">
        <div>
          <div>主密码状态：
            <el-tag :type="masterSet ? 'success' : 'warning'">{{ masterSet ? '已设置' : '未设置' }}</el-tag>
          </div>
          <div class="text-muted mt-12">主密码用于加密保存服务器登录密码，未设置时密码无法持久化保存。</div>
        </div>
        <el-button type="primary" @click="openPwDialog">{{ masterSet ? '重置主密码' : '设置主密码' }}</el-button>
      </div>
    </div>

    <el-dialog v-model="pwDialog" :title="masterSet ? '重置主密码' : '设置主密码'" width="420px">
      <el-form label-width="80px">
        <el-form-item label="主密码"><el-input v-model="pw.p1" type="password" show-password /></el-form-item>
        <el-form-item label="确认密码"><el-input v-model="pw.p2" type="password" show-password /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="pwDialog = false">取消</el-button>
        <el-button type="primary" @click="setPassword">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>