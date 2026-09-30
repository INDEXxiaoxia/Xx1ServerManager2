<script setup>
import {ref, reactive, onMounted} from 'vue'
import {ElMessage} from 'element-plus'
import {api} from '@/api'
import {useAppStore} from '@/store'
import {applyTheme, THEMES, getStoredTheme} from '@/theme'

const appStore = useAppStore()
const settings = reactive({maxConcurrent: 5, uploadTimeout: 300, commandTimeout: 30, theme: 'light'})
const masterSet = ref(false)
const pwDialog = ref(false)
const pw = reactive({old: '', p1: '', p2: ''})
const saving = ref(false)
const configDir = ref('')
const configDirExists = ref(false)
const openingDir = ref(false)

// ---- AI 设置 ----
// 常见服务商的 OpenAI 兼容地址（实现时按各家官方文档核对过）。
// 填到 /v1 为止即可，程序会自己拼 /chat/completions。
const AI_PRESETS = [
  {name: 'DeepSeek', baseURL: 'https://api.deepseek.com/v1', model: 'deepseek-chat'},
  {name: '通义千问（兼容模式）', baseURL: 'https://dashscope.aliyuncs.com/compatible-mode/v1', model: 'qwen-plus'},
  {name: 'OpenAI', baseURL: 'https://api.openai.com/v1', model: 'gpt-4o-mini'},
  {name: '硅基流动', baseURL: 'https://api.siliconflow.cn/v1', model: 'deepseek-ai/DeepSeek-V3'},
  {name: '本地 Ollama', baseURL: 'http://localhost:11434/v1', model: 'qwen2.5:14b'},
  {name: '自定义', baseURL: '', model: ''}
]
const ai = reactive({
  baseURL: '', apiKey: '', apiKeySet: false, model: '',
  maxRounds: 20, timeout: 60, contextLines: 200
})
const aiPreset = ref('')
const aiSaving = ref(false)
const aiTesting = ref(false)
const aiTestMsg = ref('')
const aiTestOk = ref(false)

async function loadAI() {
  try {
    const s = await api.getAISettings()
    Object.assign(ai, s, {apiKey: ''}) // Key 不回传明文，输入框留空表示"不修改"
    const hit = AI_PRESETS.find(p => p.baseURL && p.baseURL === s.baseURL)
    aiPreset.value = hit ? hit.name : (s.baseURL ? '自定义' : '')
  } catch (e) {
    /* 未配置时忽略 */
  }
}

function applyPreset(name) {
  const p = AI_PRESETS.find(x => x.name === name)
  if (!p) return
  if (p.baseURL) ai.baseURL = p.baseURL
  if (p.model) ai.model = p.model
}

async function saveAI() {
  aiSaving.value = true
  aiTestMsg.value = ''
  try {
    await api.saveAISettings({...ai})
    ElMessage.success('AI 设置已保存')
    await loadAI()
  } catch (e) {
    ElMessage.error('保存失败：' + e)
  } finally {
    aiSaving.value = false
  }
}

async function testAI() {
  aiTesting.value = true
  aiTestMsg.value = ''
  try {
    const r = await api.testAIConnection()
    aiTestOk.value = !!r.ok
    if (r.ok) {
      aiTestMsg.value = `✅ 连通成功 · 模型 ${r.model || '—'} · ${r.latencyMs}ms`
    } else {
      // 失败时把状态码与响应体原样带出来，用户才分得清是 Key、地址还是模型名的问题
      aiTestMsg.value = '❌ ' + (r.message || '失败')
    }
  } catch (e) {
    aiTestOk.value = false
    aiTestMsg.value = '❌ ' + e
  } finally {
    aiTesting.value = false
  }
}

async function load() {
  const s = await api.getSettings()
  Object.assign(settings, s)
  // 主题以本地持久化值为准（可在解锁前生效），数据库仅作同步
  settings.theme = getStoredTheme()
  masterSet.value = await api.isMasterPasswordSet()
  try {
    const info = await api.getConfigDirInfo()
    configDir.value = (info && info.dir) || ''
    configDirExists.value = !!(info && info.exists)
  } catch (e) {
    configDir.value = ''
  }
  await loadAI()
}
onMounted(load)

// 在系统文件管理器中打开配置文件（数据库）所在目录
async function openConfigDir() {
  openingDir.value = true
  try {
    const dir = await api.openConfigDir()
    ElMessage.success('已打开配置目录：' + dir)
  } catch (e) {
    ElMessage.error('打开配置目录失败：' + e)
  } finally {
    openingDir.value = false
  }
}

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
  pw.old = ''
  pw.p1 = ''
  pw.p2 = ''
  pwDialog.value = true
}

async function setPassword() {
  if (pw.p1.length < 6) { ElMessage.warning('主密码至少 6 位'); return }
  if (pw.p1 !== pw.p2) { ElMessage.error('两次密码不一致'); return }
  try {
    if (masterSet.value) {
      if (!pw.old) { ElMessage.warning('请输入当前主密码'); return }
      // 重置会先用旧密钥解出已存密码、再用新密钥重新加密，密码不会丢
      const res = await appStore.changePassword(pw.old, pw.p1)
      const n = (res && res.reencrypted) || 0
      const failed = (res && res.failed) || 0
      ElMessage.success(`${n} 台服务器的密码已用新主密码重新加密`)
      if (failed > 0) {
        ElMessage.warning(`${failed} 台服务器的密码解不开（历史密文），已原样保留，请在服务器列表里重新填写密码`)
      }
    } else {
      await appStore.setPassword(pw.p1)
      ElMessage.success('主密码设置成功')
    }
    masterSet.value = true
    pwDialog.value = false
  } catch (e) {
    ElMessage.error('操作失败：' + e)
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
          <el-tooltip
            :content="configDir ? `配置文件目录：${configDir}` : '打开配置文件所在目录'"
            placement="top"
          >
            <el-button :loading="openingDir" @click="openConfigDir">
              <el-icon><FolderOpened /></el-icon>&nbsp;打开配置文件目录
            </el-button>
          </el-tooltip>
        </el-form-item>
        <el-form-item v-if="configDir" label="配置文件目录">
          <span class="mono cfg-path" :class="{missing: !configDirExists}">{{ configDir }}</span>
        </el-form-item>
      </el-form>
    </div>

    <div class="card mb-12">
      <h3 class="card-title">AI 设置（SSH 终端的 AI 助手）</h3>
      <el-form label-width="120px" class="mt-12">
        <el-form-item label="常用预设">
          <el-select v-model="aiPreset" style="width: 260px" placeholder="选一个常见服务商自动填地址" @change="applyPreset">
            <el-option v-for="p in AI_PRESETS" :key="p.name" :label="p.name" :value="p.name" />
            <el-option label="自定义" value="自定义" />
          </el-select>
          <span class="text-muted field-hint">选完仍可手动改下面的地址</span>
        </el-form-item>

        <el-form-item label="接口地址">
          <el-input v-model="ai.baseURL" placeholder="https://api.deepseek.com/v1" style="width: 420px" />
          <div class="field-hint text-muted">
            填到 <b>/v1</b> 结尾即可，程序会自动拼接 <span class="mono">/chat/completions</span>；
            不要填完整的接口路径。
          </div>
        </el-form-item>

        <el-form-item label="API Key">
          <el-input
            v-model="ai.apiKey"
            type="password"
            show-password
            style="width: 420px"
            :placeholder="ai.apiKeySet ? '已保存（留空表示不修改）' : 'sk-xxxxxxxx'"
          />
          <div class="field-hint text-muted">
            用主密码派生密钥加密后保存在本地数据库，不会明文落盘。
          </div>
        </el-form-item>

        <el-form-item label="模型">
          <el-input v-model="ai.model" placeholder="deepseek-chat" style="width: 260px" />
        </el-form-item>

        <el-form-item label="接管最大轮数">
          <el-input-number v-model="ai.maxRounds" :min="1" :max="100" />
          <span class="text-muted field-hint">完全接管模式下最多自动执行多少轮，达到上限会暂停并等你追加要求</span>
        </el-form-item>

        <el-form-item label="请求超时(秒)">
          <el-input-number v-model="ai.timeout" :min="5" :max="600" />
        </el-form-item>

        <el-form-item label="终端上下文行数">
          <el-input-number v-model="ai.contextLines" :min="20" :max="2000" :step="50" />
          <span class="text-muted field-hint">每次问 AI 时附带终端最近多少行，行数越多越费 token</span>
        </el-form-item>

        <el-form-item>
          <el-button type="primary" :loading="aiSaving" @click="saveAI">保存 AI 设置</el-button>
          <el-button :loading="aiTesting" @click="testAI">
            <el-icon><Connection /></el-icon>&nbsp;测试连通
          </el-button>
          <span v-if="aiTestMsg" class="test-msg" :class="{ok: aiTestOk}">{{ aiTestMsg }}</span>
        </el-form-item>
      </el-form>
    </div>

    <div class="card mb-12">
      <h3 class="card-title">安全</h3>
      <div class="flex-between mt-12">
        <div class="sec-info">
          <div>主密码状态：
            <el-tag :type="masterSet ? 'success' : 'warning'">{{ masterSet ? '已设置' : '未设置' }}</el-tag>
            <el-tag v-if="appStore.locked" type="info" class="ml-6">已锁定</el-tag>
          </div>
          <div class="text-muted mt-12">
            主密码用于加密保存服务器登录密码。未设置时，密码仍以本机密钥加密存放在本地数据库中，
            重启后照常可用，不会丢失；设置主密码后会改用主密码派生密钥重新加密，安全性更高。
          </div>
        </div>
        <el-button type="primary" @click="openPwDialog">{{ masterSet ? '重置主密码' : '设置主密码' }}</el-button>
      </div>
    </div>

    <el-dialog v-model="pwDialog" :title="masterSet ? '重置主密码' : '设置主密码'" width="440px">
      <el-form label-width="90px">
        <el-form-item v-if="masterSet" label="当前密码">
          <el-input v-model="pw.old" type="password" show-password placeholder="当前主密码" />
        </el-form-item>
        <el-form-item label="新主密码"><el-input v-model="pw.p1" type="password" show-password /></el-form-item>
        <el-form-item label="确认密码"><el-input v-model="pw.p2" type="password" show-password /></el-form-item>
      </el-form>
      <p v-if="masterSet" class="dialog-hint">
        重置后已保存的服务器密码会用新主密码重新加密，不会丢失。
      </p>
      <template #footer>
        <el-button @click="pwDialog = false">取消</el-button>
        <el-button type="primary" @click="setPassword">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* AI 设置里的说明小字：单独占一行，不与输入框挤在一排 */
.field-hint {
  font-size: 12px;
  line-height: 1.7;
  margin-left: 10px;
  max-width: 520px;
}
.el-form-item .field-hint {
  display: block;
  margin-left: 0;
  margin-top: 4px;
}
.el-form-item .field-hint.ml-inline {
  display: inline;
  margin-left: 10px;
}
.test-msg {
  margin-left: 12px;
  font-size: 12px;
  color: var(--red);
  word-break: break-all;
}
.test-msg.ok {
  color: var(--green);
}
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
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
.ml-6 { margin-left: 6px; }
.sec-info { max-width: 620px; }
.dialog-hint {
  margin: -4px 0 0 90px;
  font-size: 12px;
  color: var(--ink-3);
  line-height: 1.7;
}
.cfg-path {
  font-size: 12px;
  color: var(--ink-2);
  word-break: break-all;
}
.cfg-path.missing { color: var(--red); }
</style>