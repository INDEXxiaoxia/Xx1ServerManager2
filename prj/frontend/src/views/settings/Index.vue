<script setup>
import {ref, reactive, onMounted} from 'vue'
import {ElMessage} from 'element-plus'
import {api} from '@/api'
import {applyTheme, THEMES, getStoredTheme} from '@/theme'

const settings = reactive({maxConcurrent: 5, uploadTimeout: 300, commandTimeout: 30, theme: 'light'})
const masterSet = ref(false)
const pwDialog = ref(false)
const pw = reactive({p1: '', p2: ''})
const saving = ref(false)

async function load() {
  const s = await api.getSettings()
  Object.assign(settings, s)
  // 主题以本地持久化值为准（可在解锁前生效），数据库仅作同步
  settings.theme = getStoredTheme()
  masterSet.value = await api.isMasterPasswordSet()
}
onMounted(load)

function onThemeChange() {
  applyTheme(settings.theme)
  save()
}

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
      <h3 class="card-title">常规设置</h3>
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
          <el-select v-model="settings.theme" style="width: 200px" @change="onThemeChange">
            <el-option v-for="t in THEMES" :key="t.name" :label="t.label" :value="t.name">
              <span class="theme-option">
                <i class="theme-dot" :class="'theme-dot--' + t.name" />
                {{ t.label }}
              </span>
            </el-option>
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="save">保存设置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <div class="card">
      <h3 class="card-title">安全</h3>
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

<style scoped>
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
.theme-option {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.theme-dot {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  display: inline-block;
  flex-shrink: 0;
}
.theme-dot--light {
  background: linear-gradient(135deg, #F5F4F1, #D9D5CC);
  border: 1px solid #B8B4AA;
}
.theme-dot--dark {
  background: linear-gradient(135deg, #3A3B44, #1D1E24);
  border: 1px solid #555760;
}
.theme-dot--cyber {
  background: radial-gradient(circle at 30% 30%, #21E6FF, #0B0F1A 60%);
  border: 1px solid #21E6FF;
  box-shadow: 0 0 6px rgba(33, 230, 255, .6);
}
</style>