<script setup>
import {onMounted} from 'vue'
import {useAppStore} from '@/store'
import Unlock from '@/views/Unlock.vue'

const appStore = useAppStore()

onMounted(() => {
  appStore.init()
})
</script>

<template>
  <div class="app-root">
    <!-- 应用信息未就绪前先给一个极简启动态：避免锁定态启动时先闪一下主界面 -->
    <div v-if="!appStore.ready" class="boot">
      <span class="boot-dot" />
      <span class="boot-text">Xx1ServerManager2</span>
    </div>
    <Unlock v-else-if="appStore.locked" />
    <router-view v-else />
  </div>
</template>

<style>
.app-root {
  height: 100vh;
  width: 100vw;
  overflow: hidden;
}
</style>

<style scoped>
.boot {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  background: var(--paper);
}
.boot-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--blue), var(--pink));
  animation: breathe 1.8s ease-in-out infinite;
}
.boot-text {
  font-size: 14px;
  font-weight: 600;
  letter-spacing: .6px;
  color: var(--ink-2);
}
</style>