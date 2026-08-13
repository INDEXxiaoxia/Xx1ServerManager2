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
    <template v-if="appStore.masterPasswordSet && !appStore.unlocked">
      <Unlock />
    </template>
    <template v-else>
      <router-view />
    </template>
  </div>
</template>

<style>
.app-root {
  height: 100vh;
  width: 100vw;
  overflow: hidden;
}
</style>