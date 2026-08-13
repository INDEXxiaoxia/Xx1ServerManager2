<script setup>
import {ref, reactive} from 'vue'
import {ElMessage} from 'element-plus'
import {useAppStore} from '@/store'

const appStore = useAppStore()
const password = ref('')
const loading = ref(false)

async function doUnlock() {
  if (!password.value) {
    ElMessage.warning('请输入主密码')
    return
  }
  loading.value = true
  try {
    const ok = await appStore.unlock(password.value)
    if (!ok) {
      ElMessage.error('主密码错误')
    }
  } catch (e) {
    ElMessage.error('解锁失败：' + e)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="unlock-wrap">
    <div class="unlock-card">
      <h2 class="brand">ServerManager2</h2>
      <p class="desc">请输入主密码以解锁加密的服务器凭据</p>
      <el-input v-model="password" type="password" show-password
                placeholder="主密码" size="large"
                @keyup.enter="doUnlock" />
      <el-button type="primary" size="large" class="btn" :loading="loading" @click="doUnlock">
        解锁
      </el-button>
    </div>
  </div>
</template>

<style scoped>
.unlock-wrap {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}
.unlock-card {
  width: 360px;
  background: #fff;
  border-radius: 12px;
  padding: 40px 32px;
  box-shadow: 0 8px 30px rgba(0, 0, 0, 0.2);
}
.brand {
  text-align: center;
  margin-bottom: 8px;
  color: #4a4a4a;
}
.desc {
  text-align: center;
  color: #909399;
  font-size: 13px;
  margin-bottom: 24px;
}
.btn {
  width: 100%;
  margin-top: 20px;
}
</style>