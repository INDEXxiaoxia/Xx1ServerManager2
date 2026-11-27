<script setup>
import {ordered} from '@/utils/list-order'
import {ref, computed, onMounted} from 'vue'
import {api} from '@/api'
import {groupIdOf, orderedGroups} from '@/utils/groups'
const props = defineProps({modelValue: String, servers: {type: Array, default: () => []}})
const emit = defineEmits(['update:modelValue', 'change'])
const groups = ref([])
onMounted(async () => { groups.value = await api.listGroups() })
const data = computed(() => ordered(orderedGroups(props.servers, groups.value), 'groups').map(g => ({
  value: 'group:' + g.id, label: g.name,
  children: ordered(props.servers, 'servers').filter(s => g.isUngrouped ? !groupIdOf(s) : groupIdOf(s) === g.id)
    .map(s => ({value: s.id, label: `${s.name} (${s.host})`}))
})).filter(g => g.children.length))
function change(value) { emit('update:modelValue', value); emit('change', value) }
</script>
<template>
  <el-cascader class="group-server-select" popper-class="group-server-popup" :model-value="modelValue" :options="data" :props="{emitPath: false}" placeholder="选择分组 / 服务器" @change="change" />
</template>

<style>
.group-server-select.el-cascader {width:min(264px,100%);flex:0 0 auto;max-width:100%}
.group-server-popup .el-cascader-menu {min-width:min(216px,44vw)}
</style>
