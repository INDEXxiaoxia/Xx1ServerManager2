# 终端 AI（建议 / 直接执行 / 完全接管）设计方案

> **状态：设计完成，尚未实现。** 待「运维工具」阶段验收通过后再开始编码。
> 本文档是实现依据，编码时以本文档为准；若实现中发现与本文档冲突，先改文档再改代码。

## 一、需求拆解

在现有 SSH 终端页右侧增加 AI 对话区，三种模式（默认建议模式）：

| 模式 | AI 给命令后 | 是否自动回读结果 | 终端可操作性 | 轮数 |
|---|---|---|---|---|
| **建议模式**（默认） | 命令卡 + 「复制」/「插入到终端」（不回车） | 否 | 正常 | 1 |
| **直接执行模式** | 自动注入当前终端执行**这一条**，回读结果后**停** | 是 | 正常 | 1 |
| **完全接管模式** | 自动注入 + 回读 + 自己决定下一条，连续循环 | 是 | **锁键盘 + 禁切标签 + 禁同步** | 配置值（默认 20） |

核心约束（用户明确要求）：

1. **AI 执行的命令必须出现在用户看得见的终端里**，就跟用户自己敲一样——因此复用当前可见终端，不做隐藏执行通道
2. 任何 AI 模式**只对当前一个终端**生效；并行（同步）模式下**禁止**启用 AI
3. 完全接管模式：开启时弹危险警告、终端四周流光 + 内发光、顶部"AI 接管中，请勿操作"、有停止按钮；遇到报错能读终端报错并自行处理；任务完成或达最大轮数自动停下
4. 停下或完成的任务可以**追加新任务**继续；聊天记录**不落库**；切页不中断运行
5. 对话区要有**清空 / 新建对话**按钮
6. 长终端内容优先保留最新的部分，避免上下文过长

## 二、可复用的现成能力

| 能力 | 现状 | 复用于 |
|---|---|---|
| 终端读写与尺寸 | `TerminalWrite` / `TerminalWriteMany` / `TerminalResize` | 注入命令、Ctrl+C |
| 终端输出事件流 | `terminal:data:<id>` / `terminal:close:<id>` | 前端显示（后端另建环形缓冲供 AI 读） |
| 一次性命令执行（非交互） | `RunRemoteCommand` → `ExecCommand`（含 sudo） | 可作为"环境侦察"的备用通道 |
| 命令历史留痕 | `command_history` + `AddCommandHistory` | AI 执行的审计台账（加 `source` 列） |
| KV 设置 + 加密 | `app_settings`、`GetSetting/SetSetting`、`internal/crypto` | AI 配置与 API Key 加密存储 |
| 主密码锁定清场 | `auth.go` → `closeAllTerminals` / `StopAllMonitors` | AI 会话同样要 cancel |
| 右侧快捷栏 | `QuickPanel.vue`（同步开关 + 预设命令 + 预设路径） | 改为三折叠并新增 AI 区块 |
| 折叠与布局范式 | `Layout.vue` 的 `collapsed`、`sidebar-handle` | 折叠交互一致性 |
| 终端页 keep-alive | `defineOptions({name:'TerminalPage'})` + `keep-alive :include` | 切页不中断 AI |

## 三、设置页新增「AI 设置」卡片

| 字段 | 默认 | UI 要求 |
|---|---|---|
| 接口地址 | 空 | 占位符 `https://api.deepseek.com/v1`；帮助文字："填到 `/v1` 结尾即可，程序自动拼接 `/chat/completions`，不要填完整接口路径"；上方「常用预设」下拉：DeepSeek / 通义千问（兼容模式）/ OpenAI / 硅基流动 / 本地 Ollama / 自定义（厂商 URL 实现时按官方文档核对后写死） |
| API Key | 空 | 密码框 + 显示/隐藏；**用主密码派生密钥加密后存 `app_settings`**（复用 `internal/crypto`，与服务器密码同策略）；**保存时若输入框为空则不覆盖已存值** |
| 模型 | 空 | 占位符 `deepseek-chat` |
| 全权模式最大轮数 | 20 | 数字 1–100；旁注"达到上限会自动暂停并等待你追加任务" |
| 请求超时(秒) | 60 | |
| 终端上下文行数 | 200 | 旁注"发送给 AI 的终端尾行数，越长越费 token" |
| 「测试连通」按钮 | — | `TestAIConnection()`：POST `/chat/completions`，`max_tokens=1`；成功 `✅ 连通 · 模型 deepseek-chat · 812ms`；失败显示 **HTTP 状态码 + 响应体前 500 字**（否则用户分不清 Key 错 / URL 错 / 模型名错） |

存储键：`ai.baseURL` / `ai.apiKey` / `ai.model` / `ai.maxRounds` / `ai.timeout` / `ai.contextLines`，对应扩展 `models.Settings` 与 `store.GetSettings/SaveSettings`。

## 四、AI 如何"读"终端

### 4.1 后端环形缓冲（必需）

```go
type TerminalSession struct {
    // ...现有字段
    tail   *byteRing // 512KB 原始字节环形缓冲（含 ANSI）
    tailMu sync.Mutex
    seq    int64     // 累计写入字节数，用于"自某偏移以来"精确切片
}
```

只在现有 `out.Read` goroutine 里追加一处，其余代码零改动：

```go
func (a *App) AITerminalSnapshot(termID, mode string, lines int, selection string) (AIContext, error)
// mode: selection | last50 | last200 | sinceLast | all
```

### 4.2 ANSI 清洗器（新增 `internal/ansi/strip.go`）

- CSI / OSC / DCS 剥离（颜色、窗口标题、bracketed paste 标记）
- **`\r` 折叠**：每行只保留最后一个 `\r` 之后的内容（进度条、spinner、`curl` 会输出上千个覆盖行；这一步能砍掉约 80% 体积且保住信息）
- `\b` 退格处理、`\r\n` 归一、连续重复行折叠为 `上一行 ×N`
- **按 rune 边界处理**，避免切裂多字节中文

### 4.3 上下文控制（"长内容优先最新的"）

- 优先级：**用户选区 > 最近 N 行 > 自上次 AI 命令以来**
- 超过 32KB（约 8k token）：保留头 20% + 尾 80%，中间标注 `…已省略 N 行…`
- **历史轮次压缩**：较早轮次的终端快照替换为一行摘要 `[第 3 轮终端内容，8.2KB，已折叠]`，只保留最近 2 轮原文——这是多轮接管不爆 context 的关键
- **UI 透明**：输入框下方一行小字"将附带终端最近 200 行（约 6.2 KB）"，可点开预览实际发送内容；旁边「读取档位」下拉

### 4.4 单条命令的输出控制

单步回读上限 64KB（头 8KB + 尾 56KB + 省略标注），再按 `ai.contextLines` 截尾后才发模型，并明确告知"已按摘要发送，点击查看完整输出"。

## 五、AI 如何"写"终端

### 5.1 注入与结束判定

- 注入用 `\r` 提交（复用 `onQuickJump` 的经验：PTY 的 ICRNL 依赖 CR），走现成 `TerminalWrite`
- **单行命令**：`{ CMD ; } ; printf '\nX1S<step>D%sX1E\r\033[K' "$?"`
- **多行命令**：`CMD` + `\r` + 单独一行 `printf '\nX1S<step>D%sX1E\r\033[K' "$?"`
- 标记行末尾 `\r` + `ESC[K` **擦除**：用户看不见，但原始字节流里保留，可精确定位
- 结果提取：以"发送时刻的 `seq`"为起点向后匹配 `X1S<step>D(\d+)X1E`，中间即输出，捕获数字即退出码
- **先探测 shell**（`echo $0` / `$BASH_VERSION`）：fish/csh 等不支持该语法 → 禁用接管模式并明确提示（唯一硬性环境要求）

### 5.2 卡死防护

| 触发 | 对策 |
|---|---|
| `sudo` 密码提示 / `[Y/n]` / `Do you want` / `Continue?` | 每 400ms 检测新字节并匹配提示词 → **暂停**，三个按钮：〈我手动输入并继续〉〈发送 Ctrl+C〉〈中止任务〉 |
| 进入备用屏（`ESC[?1049h`，vim/top/less） | 判定交互式程序 → 暂停并提示人工接管 |
| 无输出静默 > 120s | 提示"可能卡住或长时间运行中"，给〈继续等待〉〈Ctrl+C〉〈中止〉 |
| 单步硬超时 | 默认 300s，到点收尾并标记 `timeout` |

## 六、接管循环与状态机

```
状态: idle → running ⇄ paused → stopped | done | error | maxRounds

每轮:
  1. 组织 messages（system + 压缩历史 + 最新终端尾）
  2. 调模型，期望严格 JSON：
     {"thought":"...","command":"...","finish":false,"summary":"..."}
     解析兜底：去 ```json 围栏 → 平衡括号提取首个 {...} → 仍失败则当纯文本展示、
     不执行任何命令并停止（绝不猜命令）
  3. finish=true 或 command 为空 → 完成，输出总结
  4. 注入 + 等标记 + 回读 → emit 结果卡 → 下一轮
  5. 轮数 > maxRounds → emit "已达最大轮数(20)，任务暂停"，保留上下文供追加
```

system prompt 写死约束：命令必须非交互（`-y` / `--no-pager` / `-f`）、禁止 `vim/top/less`、禁止破坏性操作（`rm -rf /`、改 sshd/防火墙/分区）、缺信息时先用只读命令侦察、cwd 通过每条命令后 `printf "\n__X1PWD__%s\n" "$PWD"` 自行维护。

> 顺带解决既有遗留问题：`terminal-quickpanel-sync-design.md` §六"不做项"里"交互式 shell 的 cwd 无法取得"的限制，在 AI 通道内通过自维护 cwd 解决。

### 接口与事件

```go
func (a *App) AIChat(req AIRequest) (AIResponse, error)
func (a *App) AIExecOne(termID, command string) (AIExecResult, error)
func (a *App) AIStartTakeover(req AIRequest) (string, error)
func (a *App) AIStopTakeover(sid string) error
func (a *App) AIAppendTask(sid, text string) error
func (a *App) AIReset(termID string) error
func (a *App) TestAIConnection() (AITestResult, error)
```

事件：`ai:message:<sid>`、`ai:step:<sid>`、`ai:state:<sid>`（含 `state/round/maxRounds/startedAt/reason`）。

**v1 用非流式**（流式 SSE 列入后续增强）：等待时间主要花在命令执行上，非流式可接受，且省掉中断与增量渲染的复杂度。

## 七、三折叠右侧栏与接管视觉

```
┌─ 右栏 300/340px ──────────┐
│ ▸ 预设命令        (12)    │  折叠状态存 localStorage
│ ▾ 预设路径        (5)     │  xx1.terminal.section.{cmds,paths,ai}
│ ▸ AI 助手  [建议▾] ●      │  模式选择器在这里；接管中变红点
│   ┌ 对话流（用户/AI/命令卡/结果卡）┐
│   [输入框................]⏎ │
│   将附带终端最近 200 行 (6.2KB)│
└───────────────────────────┘
```

- 三块做 **accordion**：展开的那块 `flex: 1 1 auto`，折叠的只有标题行（32px）——避免三块抢高度
- 沿用现有 `sidebar-handle`（22px 竖条）与 `Layout.vue` 折叠范式；AI 长回答时右栏宽度切两档（340 → 480px）
- 折叠/宽度变化**必须挂到 `refitAll()` 同一入口**（否则终端仍按旧宽度排版）
- **接管视觉**：`.term-body.takeover` 内发光 `inset box-shadow` + `::after` 旋转 `conic-gradient` 流光边框，三主题都用 `--blue/--yellow` 令牌；尊重 `prefers-reduced-motion`
- 顶部横幅「**AI 接管中，请勿操作**」+ `第 3/20 轮` + 当前命令 + 已运行时长 + 「停止」
- **开启接管的前置弹窗**：标题「⚠️ 完全接管模式（实验功能）」，正文明确写"AI 将自动连续执行命令，包括删除、重启、安装等有风险操作，不逐条征求同意。**请仅在测试服务器上使用，不要在任何业务服务器上使用**，出问题本软件无法回退"，确认按钮文案「我已知悉风险，仅用于测试环境」

## 八、接管期间的单终端锁定（防错乱）

- 接管开始时记录 `lockedTabKey`；期间：`.tab-add` / `.tab-close` 禁用、其他标签点击被拦截并提示、隐藏 pane 的 `mousedown` 拦截
- 「同步操作」若开着 → **自动关闭并置灰**（禁止多终端并行）
- 终端输入拦截：`term.onData` 在接管中丢弃写入并首次提示"AI 接管中，请勿操作"
- **紧急释放**：连按 3 次 `Esc` 立即停止接管（防 UI 卡住点不到按钮）
- 停止/出错/完成 → 全部解除并 `refitAll()`
- 切页不中断：后端 goroutine + 终端页 keep-alive 已保活；主密码锁定 → `auth.Lock` 里同时 cancel 所有 AI 会话
- 聊天记录**不落库**（内存态，重启即消失）；「清空对话 / 新建对话」复位上下文

## 九、安全与留痕

- API Key 与服务器密码同等待遇加密；锁定时 AI 面板整体置灰并清内存 Key
- AI 每次执行的命令**全量写入 `command_history`**：增列 `source`（`manual|batch|ai`）+ `ai_session_id`（`store.migrate()` 已有 `hasColumn` 范式）
- 全权模式**不做命令拦截**（真全权），但每步都留下醒目的命令卡与退出码，可追溯

## 十、组件与文件改动清单

| 文件 | 改动 |
|---|---|
| `prj/internal/aiclient/client.go` + `_test.go` | 新增：OpenAI 兼容客户端（httptest 覆盖成功/401/超时/畸形 JSON） |
| `prj/internal/ansi/strip.go` + `_test.go` | 新增：ANSI 清洗 + `\r` 折叠（含中文、进度条用例） |
| `prj/ai.go` | 新增：全部 AI 绑定 + 接管循环 + 标记注入 |
| `prj/terminal.go` | `TerminalSession` 加环形缓冲与 `seq`；`Snapshot` |
| `prj/auth.go` | 锁定 / 断开时 cancel 所有 AI 会话 |
| `prj/app.go` | `aiMu` / `aiSessions` 字段 |
| `prj/internal/models/models.go` | AI DTO + `Settings` 扩展 |
| `prj/internal/store/store.go` | `ai.*` 设置键；`command_history` 增列 |
| `prj/frontend/src/views/terminal/QuickPanel.vue` | 改三折叠 + AI 区块（模式选择器） |
| `prj/frontend/src/views/terminal/AiPanel.vue` | 新增：对话流 / 命令卡 / 输入框 / 停止 / 清空 / 新建对话 / 上下文档位 |
| `prj/frontend/src/views/terminal/Index.vue` | 接管锁定 + 流光 + 横幅 + 紧急释放 |
| `prj/frontend/src/views/settings/Index.vue` | AI 设置卡片 + 测试连通 |
| `prj/frontend/src/theme.js` / `style.css` | 流光与内发光令牌（三主题） |
| `prj/frontend/src/utils/markdown-lite.js` | 新增：极简代码块/行内代码渲染（不引入 markdown 库） |
| `prj/frontend/src/api/index.js` + `wailsjs/**` | 新增封装并重新生成绑定 |

## 十一、边界情况与失败模式

| 场景 | 处理 |
|---|---|
| 未配置 AI / Key 未填 | AI 区显示引导卡片，附「去设置」按钮 |
| API 返回 401 / 404 / 超时 | 对话流内红色错误卡，显示状态码与响应体前 500 字；不重试、不乱猜 |
| 模型返回非 JSON | 三级兜底解析，最终按纯文本展示并停止，绝不猜命令 |
| 非 POSIX shell（fish/csh） | 禁用接管与直接执行模式，只保留建议模式 |
| 终端会话断开（掉线 / `exit`） | 立即停止 AI 并提示"终端已断开" |
| 命令进入交互式程序 | 暂停并请人工接管（见 5.2） |
| 输出量暴涨（如 `cat` 大文件） | 单步 64KB 截断 + 摘要标注；`ExecCommand` 补字节上限，避免后端内存爆 |
| 接管中用户误触键盘 / 切标签 | 拦截并提示；连按 3 次 Esc 可紧急停止 |
| 接管中切到其他页面 | 继续运行（后端 goroutine + keep-alive），回来自动恢复渲染 |
| 主密码锁定 | cancel 全部 AI 会话 + 关闭终端 + 清内存 Key |
| 最大轮数到达 | 暂停并提示，可「追加任务」继续或「新建对话」重开 |

## 十二、明确不做（本期）

1. 不做多终端并行 AI、不做同步模式下的 AI
2. 不做多 Agent 并行
3. 不做无人值守长跑（到轮数上限就停）
4. 不做流式响应
5. 不做对话持久化
6. 不做对本地 Windows 机器的操作（只操作远端）
7. 不引入 markdown 库（自带约 60 行 `markdown-lite.js`）

## 十三、验收清单（实现后逐条验证）

1. 设置页能配置 base URL / Key / 模型 / 最大轮数，示例文案让人清楚"填到 /v1 为止"；「测试连通」对错误 Key 能显示 401 与响应体
2. 建议模式：问"怎么查 XXX.jar 的进程"→ 得到命令卡 → 「复制」可粘贴运行；「插入到终端」只键入不回车
3. 建议模式：终端跑出一堆报错后问"现在的终端内容发生了什么"→ AI 能读到当前终端最新内容并解释
4. 直接执行模式：AI 命令自动在可见终端执行一次并回读结果，随后停下等待
5. 完全接管模式：开启时弹危险警告；确认后终端出现流光与「AI 接管中，请勿操作」横幅
6. 接管模式：输入"帮我安装 X"→ AI 逐条执行、监控输出、遇错自行修正，完成后自动停止并给总结
7. 接管模式：点「停止」能立刻停；点「追加任务」能从停止处继续
8. 接管中：无法切换标签、无法开启同步、键盘输入被拦截并提示；连按 3 次 Esc 可紧急停止
9. 接管中切到别的页面再回来：任务仍在继续，对话流完整
10. 「清空对话 / 新建对话」可用；应用重启后对话不残留
11. 主密码锁定：AI 立即停止，面板置灰
12. 未配置 AI 时进入终端页：AI 区给出「去设置」引导，不影响其他功能
13. `go vet` / `go test` / `npm run build` / `wails build` 全绿