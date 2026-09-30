<script setup>
import {ref, reactive, computed, onMounted, watch} from 'vue'
import {ElMessage, ElMessageBox} from 'element-plus'
import {api} from '@/api'
import {UNGROUPED, groupIdOf, matchServer, orderedGroups, groupNameOf, groupColorOf} from '@/utils/groups'

const list = ref([])
const servers = ref([])
const dialogVisible = ref(false)
const form = reactive({id: '', name: '', description: '', color: '#5780A9'})
const selectedGroup = ref('') // 分组 id 或 UNGROUPED
const keyword = ref('')
const loading = ref(false)
const saving = ref(false)
// draft：分组 id → 勾选的服务器 id 数组（未保存的草稿，切分组不丢）
const draft = reactive({})

const colors = ['#5780A9', '#C58A98', '#C79E4F', '#67947A', '#8F919B', '#9B7FB8', '#6FAE9F', '#D08770']

// 左栏顺序与总览/下拉一致：有服务器的分组在前，未分组最后
const groupsView = computed(() => orderedGroups(servers.value, list.value))
const isUngrouped = computed(() => selectedGroup.value === UNGROUPED)
const currentGroup = computed(() => groupsView.value.find(g => g.id === selectedGroup.value) || null)

async function load() {
  loading.value = true
  try {
    const [gs, svs] = await Promise.all([api.listGroups(), api.listServers()])
    list.value = gs
    servers.value = svs
    // 选中项失效（分组被删）时回落到未分组
    if (selectedGroup.value && !groupsView.value.some(g => g.id === selectedGroup.value)) {
      selectedGroup.value = UNGROUPED
    }
    if (!selectedGroup.value) {
      selectedGroup.value = groupsView.value.length ? groupsView.value[0].id : UNGROUPED
    }
    pruneDrafts()
  } catch (e) {
    ElMessage.error('加载失败：' + e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

// pruneDrafts 丢掉已删除分组/已删除服务器的草稿项
function pruneDrafts() {
  const groupIds = new Set(list.value.map(g => g.id))
  const known = new Set(servers.value.map(s => s.id))
  for (const key of Object.keys(draft)) {
    if (!groupIds.has(key)) {
      delete draft[key]
      continue
    }
    draft[key] = draft[key].filter(id => known.has(id))
  }
}

// ---- 草稿：勾选集合即该分组的成员集合（保存后未勾选的本组成员会释放为未分组） ----
// 读取一律不写 draft（渲染期不改状态），只有真正勾选/取消时才创建草稿项。

function memberIdsOf(groupId) {
  return servers.value.filter(s => groupIdOf(s) === groupId).map(s => s.id)
}

function checkedOf(groupId) {
  return draft[groupId] || memberIdsOf(groupId)
}

function isChecked(id) {
  if (isUngrouped.value) return false
  return checkedOf(selectedGroup.value).includes(id)
}

function toggle(id, checked) {
  const gid = selectedGroup.value
  const cur = [...checkedOf(gid)]
  draft[gid] = checked
    ? (cur.includes(id) ? cur : [...cur, id])
    : cur.filter(x => x !== id)
}

// isDirty 某分组的草稿与库中实际成员是否不一致
function isDirty(groupId) {
  if (!(groupId in draft)) return false
  const a = [...draft[groupId]].sort()
  const b = [...memberIdsOf(groupId)].sort()
  return a.length !== b.length || a.some((id, i) => id !== b[i])
}

const visibleServers = computed(() => servers.value.filter(s => matchServer(s, keyword.value)))

const selectedCount = computed(() => (isUngrouped.value ? 0 : checkedOf(selectedGroup.value).length))

const dirty = computed(() => !isUngrouped.value && isDirty(selectedGroup.value))

// conflicts：本次将归入本组、但当前属于其他分组的服务器
const conflicts = computed(() => {
  if (isUngrouped.value) return []
  return checkedOf(selectedGroup.value)
    .map(id => servers.value.find(s => s.id === id))
    .filter(s => s && groupIdOf(s) && groupIdOf(s) !== selectedGroup.value)
})

// released：原本属于本组、但本次未勾选的服务器
const released = computed(() => {
  if (isUngrouped.value) return []
  const cur = checkedOf(selectedGroup.value)
  return servers.value.filter(s => groupIdOf(s) === selectedGroup.value && !cur.includes(s.id))
})

function dirtyGroups() {
  return list.value.filter(g => isDirty(g.id)).map(g => g.id)
}

function selectAllVisible() {
  if (isUngrouped.value) return
  const cur = new Set(checkedOf(selectedGroup.value))
  visibleServers.value.forEach(s => cur.add(s.id))
  draft[selectedGroup.value] = [...cur]
}

function clearAll() {
  if (isUngrouped.value) return
  draft[selectedGroup.value] = []
}

function resetDraft() {
  if (isUngrouped.value) return
  delete draft[selectedGroup.value]
}

function selectGroup(id) {
  selectedGroup.value = id
  keyword.value = ''
}

async function saveAssign() {
  if (isUngrouped.value) return
  saving.value = true
  try {
    const res = await api.assignGroupServers(selectedGroup.value, checkedOf(selectedGroup.value))
    const moved = (res && res.movedIn) || 0
    const rel = (res && res.released) || 0
    ElMessage.success(`已保存：归入 ${moved} 台，移出 ${rel} 台`)
    if (conflicts.value.length) {
      ElMessage.info(`${conflicts.value.length} 台服务器已从原分组转移到「${currentGroup.value.name}」`)
    }
    delete draft[selectedGroup.value]
    await load()
  } catch (e) {
    ElMessage.error('保存失败：' + e)
  } finally {
    saving.value = false
  }
}

// 移出分组：把本组所有成员一次性变成未分组
async function releaseAll() {
  const ids = memberIdsOf(selectedGroup.value)
  if (!ids.length) return
  try {
    await ElMessageBox.confirm(
      `确定把「${currentGroup.value.name}」下的 ${ids.length} 台服务器移出分组吗？它们会变成未分组。`,
      '移出分组', {type: 'warning', confirmButtonText: '移出'}
    )
  } catch (e) {
    return
  }
  try {
    const res = await api.assignGroupServers('', ids)
    ElMessage.success(`已移出 ${(res && res.released) || ids.length} 台服务器`)
    delete draft[selectedGroup.value]
    await load()
  } catch (e) {
    ElMessage.error('移出失败：' + e)
  }
}

// ---- 分组的增删改 ----

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
  try {
    const saved = await api.saveGroup({...form})
    ElMessage.success('保存成功')
    dialogVisible.value = false
    await load()
    if (saved && saved.id) selectedGroup.value = saved.id
  } catch (e) {
    ElMessage.error('保存失败：' + e)
  }
}

async function remove(g) {
  const count = g.count || 0
  const tip = count
    ? `确定删除分组「${g.name}」吗？该分组下 ${count} 台服务器会变成未分组。`
    : `确定删除分组「${g.name}」吗？`
  try {
    await ElMessageBox.confirm(tip, '删除分组', {type: 'warning', confirmButtonText: '删除'})
  } catch (e) {
    return
  }
  try {
    const released = await api.deleteGroup(g.id)
    delete draft[g.id]
    ElMessage.success(released ? `已删除，${released} 台服务器已变为未分组` : '已删除')
    selectedGroup.value = ''
    await load()
  } catch (e) {
    ElMessage.error('删除失败：' + e)
  }
}

// 切分组时若还有别的分组存着未保存草稿，提醒一次（草稿本身不会丢）
watch(selectedGroup, (nv, ov) => {
  const others = dirtyGroups().filter(id => id !== nv && id !== ov)
  if (others.length) {
    ElMessage.info(`还有 ${others.length} 个分组的改动未保存，保存按钮在右下角`)
  }
})
</script>

<template>
  <div class="page-container">
    <div class="groups-layout" v-loading="loading">
      <!-- 左栏：分组列表（含未分组），带台数 -->
      <div class="card pane-left">
        <div class="pane-head">
          <span class="pane-title">分组 ({{ list.length }})</span>
          <el-button size="small" type="primary" @click="openAdd">新增分组</el-button>
        </div>
        <ul class="group-list">
          <li v-for="g in groupsView" :key="g.id"
              class="group-row" :class="{active: selectedGroup === g.id, 'has-ops': !g.isUngrouped}"
              @click="selectGroup(g.id)">
            <i class="dot" :style="{background: g.color}" />
            <span class="g-name">{{ g.name }}</span>
            <!-- 未保存标记固定占位（只切可见性）：出现/消失都不改行内布局 -->
            <span class="dirty-dot" :class="{on: dirtyGroups().includes(g.id)}" title="有未保存的分配改动" />
            <!-- 台数与悬停操作共用一块固定尺寸的尾巴：两者都绝对定位、只切显示，
                 行高与行宽都不随 hover 变化，鼠标上下移动时列表不会抖 -->
            <span class="row-tail">
              <span class="g-count">{{ g.count }}</span>
              <span v-if="!g.isUngrouped" class="g-ops" @click.stop>
                <el-button size="small" text type="primary" @click="openEdit(g)">编辑</el-button>
                <el-button size="small" text type="danger" @click="remove(g)">删除</el-button>
              </span>
            </span>
          </li>
        </ul>
      </div>

      <!-- 右栏：成员分配（勾选即归入，未勾选的本组成员会释放为未分组） -->
      <div class="card pane-right">
        <template v-if="!isUngrouped && currentGroup">
          <div class="pane-head">
            <span class="pane-title">
              成员分配 · {{ currentGroup.name }}
              <span class="muted">当前 {{ currentGroup.count }} 台</span>
            </span>
            <div class="head-actions">
              <el-button size="small" @click="selectAllVisible">全选当前列表</el-button>
              <el-button size="small" @click="clearAll">清空勾选</el-button>
            </div>
          </div>

          <div class="assign-bar">
            <el-input v-model="keyword" placeholder="搜索名称或主机" clearable size="small" class="kw">
              <template #prefix><el-icon><Search /></el-icon></template>
            </el-input>
            <span class="muted">已选 {{ selectedCount }} 台 / 共 {{ servers.length }} 台</span>
          </div>

          <div class="member-list">
            <div v-for="s in visibleServers" :key="s.id" class="member-row" :class="{checked: isChecked(s.id)}">
              <el-checkbox :model-value="isChecked(s.id)" @change="v => toggle(s.id, v)" />
              <span class="m-name" :title="s.name">{{ s.name }}</span>
              <span class="m-host muted">{{ s.host }}</span>
              <span class="m-group" :class="{moving: groupIdOf(s) && groupIdOf(s) !== selectedGroup}"
                    :title="'当前分组：' + groupNameOf(list, groupIdOf(s))">
                <i class="dot" :style="{background: groupColorOf(list, groupIdOf(s))}" />
                {{ groupNameOf(list, groupIdOf(s)) }}
                <em v-if="groupIdOf(s) && groupIdOf(s) !== selectedGroup">将转移</em>
              </span>
            </div>
            <div v-if="!visibleServers.length" class="empty-line muted">没有匹配的服务器</div>
          </div>

          <div class="assign-foot">
            <div class="tips">
              <span v-if="conflicts.length" class="tip-warn">
                其中 {{ conflicts.length }} 台原属其他分组，保存后将转移过来（一个服务器只属于一个分组）
              </span>
              <span v-else-if="released.length" class="tip-muted">
                有 {{ released.length }} 台本组服务器未勾选，保存后会变为未分组
              </span>
              <span v-else class="tip-muted">勾选即归入本组；取消勾选的本组成员保存后会变为未分组</span>
            </div>
            <div class="foot-actions">
              <el-button :disabled="!dirty" @click="resetDraft">还原</el-button>
              <el-button type="danger" plain :disabled="!currentGroup.count" @click="releaseAll">移出分组</el-button>
              <el-button type="primary" :disabled="!dirty" :loading="saving" @click="saveAssign">保存分配</el-button>
            </div>
          </div>
        </template>

        <template v-else>
          <div class="pane-head">
            <span class="pane-title">未分组 <span class="muted">{{ currentGroup ? currentGroup.count : 0 }} 台</span></span>
          </div>
          <div class="hint-line">
            未分组的服务器不能在这里勾选。选中左侧某个分组后勾选即可把它归入该分组；
            要把某台服务器移出分组，就进它所在的分组里取消勾选再保存。
          </div>
          <div class="assign-bar">
            <el-input v-model="keyword" placeholder="搜索名称或主机" clearable size="small" class="kw">
              <template #prefix><el-icon><Search /></el-icon></template>
            </el-input>
          </div>
          <div class="member-list">
            <div v-for="s in visibleServers.filter(x => !groupIdOf(x))" :key="s.id" class="member-row">
              <span class="m-name" :title="s.name">{{ s.name }}</span>
              <span class="m-host muted">{{ s.host }}</span>
              <span class="m-group muted">未分组</span>
            </div>
            <div v-if="!visibleServers.some(x => !groupIdOf(x))" class="empty-line muted">没有未分组的服务器</div>
          </div>
        </template>
      </div>
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
.groups-layout {
  display: grid;
  grid-template-columns: 240px minmax(0, 1fr);
  gap: 12px;
  align-items: start;
}
.pane-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 10px;
}
.pane-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--ink);
}
.pane-title .muted { font-weight: 400; margin-left: 6px; }
.muted { color: var(--ink-3); font-size: 12px; }

/* ---- 左栏 ---- */
.group-list { list-style: none; }
.group-row {
  position: relative;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 8px;
  border-radius: 10px;
  cursor: pointer;
  transition: background-color .2s ease;
}
.group-row:hover { background: var(--hover-bg); }
.group-row.active { background: var(--blue-soft); }
.group-row.active .g-name { color: var(--blue-deep); font-weight: 600; }
.dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  flex-shrink: 0;
}
.g-name {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  line-height: 20px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* 尾巴：固定 88×20，内部元素全部绝对定位 —— hover 只切换显示，不参与布局 */
.row-tail {
  position: relative;
  flex: 0 0 88px;
  height: 20px;
  align-self: center;
}
.row-tail > * {
  position: absolute;
  right: 0;
  top: 50%;
  transform: translateY(-50%);
}
.g-count {
  font-size: 12px;
  line-height: 1;
  color: var(--ink-3);
  background: var(--hover-bg);
  border-radius: 999px;
  padding: 4px 7px;
}
.group-row.active .g-count { background: var(--card); }
.dirty-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
  visibility: hidden;
}
.dirty-dot.on {
  visibility: visible;
  background: var(--yellow);
}
.g-ops {
  display: inline-flex;
  align-items: center;
  visibility: hidden;
  opacity: 0;
  transition: opacity .15s ease;
}
.group-row:hover .g-ops {
  visibility: visible;
  opacity: 1;
}
/* 只有真有操作的行走（未分组只有台数，不能被藏掉） */
.group-row.has-ops:hover .g-count { visibility: hidden; }
.g-ops :deep(.el-button) {
  padding: 0 4px;
  height: 20px;
  font-size: 12px;
}
.g-ops :deep(.el-button + .el-button) { margin-left: 6px; }

/* ---- 右栏 ---- */
.head-actions { display: flex; gap: 6px; }
.assign-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 8px;
}
.assign-bar .kw { width: 220px; }
.member-list {
  border: 1px solid var(--line);
  border-radius: 12px;
  background: var(--panel-2);
  max-height: 52vh;
  overflow-y: auto;
  padding: 4px;
}
.member-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 8px;
  border-radius: 8px;
}
.member-row:hover { background: var(--fill-color); }
.member-row.checked { background: var(--blue-soft); }
.m-name {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.m-host { width: 140px; flex-shrink: 0; }
.m-group {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  flex-shrink: 0;
  max-width: 200px;
  font-size: 12px;
  color: var(--ink-2);
}
.m-group.moving { color: var(--yellow); }
.m-group em {
  font-style: normal;
  font-size: 11px;
  color: var(--yellow);
}
.empty-line { padding: 14px 8px; text-align: center; }
.assign-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 10px;
  flex-wrap: wrap;
}
.tips { font-size: 12px; line-height: 1.6; min-width: 0; }
.tip-warn { color: var(--yellow); }
.tip-muted { color: var(--ink-3); }
.foot-actions { display: flex; gap: 6px; flex-shrink: 0; }
.hint-line {
  font-size: 12px;
  color: var(--ink-3);
  line-height: 1.8;
  background: var(--hover-bg);
  border-radius: 10px;
  padding: 10px 12px;
  margin-bottom: 10px;
}

/* ---- 颜色选择 ---- */
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