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
    <div class="wash wash-left" />
    <div class="wash wash-right" />
    <div class="glint" />
    <div class="unlock-card">
      <div class="brand-row">
        <h2 class="brand">Xx1ServerManager2</h2>
      </div>
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
  position: relative;
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--paper);
  overflow: hidden;
}
.wash {
  position: absolute;
  border-radius: 50%;
  filter: blur(100px);
  pointer-events: none;
}
.wash-left {
  width: 46vmax;
  height: 46vmax;
  left: -14vmax;
  top: -12vmax;
  background: radial-gradient(circle at center, var(--halo-1), transparent 62%);
  animation: breathe 11s ease-in-out infinite;
}
.wash-right {
  width: 42vmax;
  height: 42vmax;
  right: -12vmax;
  bottom: -14vmax;
  background: radial-gradient(circle at center, var(--halo-2), transparent 62%);
  animation: breathe 13s ease-in-out infinite reverse;
}
.glint {
  position: absolute;
  width: 16vmax;
  height: 16vmax;
  top: 18%;
  right: 22%;
  border-radius: 50%;
  background: radial-gradient(circle at center, rgba(255, 176, 32, .18), transparent 60%);
  filter: blur(50px);
  pointer-events: none;
}
.unlock-card {
  position: relative;
  width: 360px;
  background: var(--card);
  border-radius: 20px;
  padding: 40px 32px 32px;
  box-shadow: var(--card-shadow);
  animation: rise .6s cubic-bezier(.16, 1, .3, 1) both;
}
.brand-row {
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 6px;
}
.brand {
  font-size: 21px;
  font-weight: 700;
  letter-spacing: 1px;
  color: var(--ink);
  margin: 0;
}
.desc {
  text-align: center;
  color: var(--ink-3);
  font-size: 13px;
  margin-bottom: 26px;
}
.btn {
  width: 100%;
  margin-top: 20px;
  border-radius: 12px;
}
.btn:hover {
  transform: translateY(-1px);
}
</style>