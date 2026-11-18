<script setup>
import {ref, computed, onMounted} from 'vue'
import {api} from '@/api'
import {groupIdOf, orderedGroups} from '@/utils/groups'
const props = defineProps({modelValue: String, servers: {type: Array, default: () => []}})
const emit = defineEmits(['update:modelValue', 'change'])
const groups = ref([])
onMounted(async () => { groups.value = await api.listGroups() })
const data = computed(() => orderedGroups(props.servers, groups.value).map(g => ({
  value: 'group:' + g.id, label: g.name,
  children: props.servers.filter(s => g.isUngrouped ? !groupIdOf(s) : groupIdOf(s) === g.id)
    .map(s => ({value: s.id, label: `${s.name} (${s.host})`}))
})).filter(g => g.children.length))
function change(value) { emit('update:modelValue', value); emit('change', value) }
</script>
<template>
  <el-cascader :model-value="modelValue" :options="data" :props="{emitPath: false}" filterable placeholder="选择分组 / 服务器" @change="change" />
</template>
