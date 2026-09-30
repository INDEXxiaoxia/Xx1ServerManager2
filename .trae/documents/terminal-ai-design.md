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
| 右侧快捷栏 | `QuickPanel.vue`（同步开关 + 预设命令 + 预设路径） | 加一个「AI 辅助」页签（预设区保持原样） |
| 折叠与布局范式 | `Layout.vue` 的 `collapsed`、`sidebar-handle` | 折叠交互一致性 |
| 终端页 keep-alive | `defineOptions({name:'TerminalPage'})` + `keep-alive :include` | 切页不中断 AI |

## 三、设置页新增「AI 设置」卡片

| 字段 | 默认 | UI 要求 |
|---|---|---|
| 接口地址 | 空 | 占位符 `https://api.deepseek.com/v1`；帮助文字："填到 `/v1` 结尾即可，程序自动拼接 `/chat/completions`，不要填完整接口路径"；上方「常用预设」下拉：DeepSeek / 通义千问（兼容模式）/ OpenAI / 硅基流动 / 本地 Ollama / 自定义（厂商 URL 实现时按官方文档核对后写死） |
| API Key | 空 | 密码框 + 显示/隐藏；**用主密码派生密钥加密后存 `app_settings`**（复用 `internal/crypto`，与服务器密码同策略）；**保存时若输入框为空则不覆盖已存值** |
| 模型 | 空 | 占位符 `deepseek-chat` |
| 全权模式最大轮数 | 20 | 数字 1–100；旁注"达到上限会自动暂停，继续发送消息就接着往下跑" |
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

## 七、右侧面板与接管视觉

### 7.1 右侧面板：两个页签 + 竖排展开按钮（不是折叠手风琴）

早期版本把右侧做成"预设命令 / 预设路径 / AI"三个可折叠分区（手风琴），实测不好用：
三块在 320px 竖列里抢高度，每块都只剩几十像素。现在改成**页签**，跟编译器的侧边面板一致：

```
收起时（右边缘竖排两个按钮）      展开时（页签 + 内容）
┌──┐                          ┌──────────────────────────┐
│预│  ← 预设操作               │ [预设操作] [AI 辅助] [»] │
│设│                          ├──────────────────────────┤
│操│                          │ 同步操作                 │
│作│                          │ 预设命令  [分类▾]   [+]  │
├──┤                          │  ┌────────────────────┐  │
│AI│  ← AI 辅助（接管中亮红点）│  │ 查看磁盘           │  │
│辅│                          │  └────────────────────┘  │
│助│                          │ 预设路径            [+]  │
└──┘                          │  /var/log          [跳转]│
                              └──────────────────────────┘
```

- 收起状态：`.sidebar-rail` 是一列竖排按钮（各占一半高度，`writing-mode: vertical-rl` 竖排文字），
  分别展开「预设操作」与「AI 辅助」；AI 按钮在接管中显示红点（暂停时是黄点）
- 展开状态：顶部页签切换两块内容，右侧 `»` 收起；页签状态存 `xx1.terminal.sidebarTab`
  （`presets` / `ai` / 空串=收起），并兼容旧的布尔键 `xx1.terminal.sidebar`
- 「预设操作」保持原来的双栏布局（同步开关 + 预设命令 + 预设路径，两块各自滚动），不做任何改动
- 两块内容都用 `v-show`：切页签 / 收起面板都不会销毁 AI 对话与 `ai:*` 事件订阅
- 接管进行中会自动切到 AI 页签（保证「停止」够得着），但**不阻止**用户收起面板——
  因为终端上的接管徽标里也有「停止」

### 7.2 接管视觉：只有覆盖层，不改动页面

**踩过的坑**：最初用 `conic-gradient + mask + rotate(360deg)` 做"流光"，旋转的是一个
"只显示边框"的遮罩层，结果就是**一个方框在屏幕上转**。正确做法是让光**沿着边框路径跑**：
用 SVG `<rect>` 的 `stroke-dasharray` + `stroke-dashoffset` 动画（一条亮段绕矩形周长行进），
不旋转任何元素，转角与圆角都自然。

- 结构：`.term-body` 内一个 `position: absolute; inset: 0; pointer-events: none` 的覆盖层，
  内含 `<svg>`（两条 `rect`：主题蓝 + 主题粉，相位差半个周期）+ 右上角小徽标 +（暂停时的）提示条
- `stroke-dasharray: 130 2010`，`stroke-dashoffset` 在一个周期（2140）内循环 → 无缝、不跳变
- 内发光用 `box-shadow: inset …` 实现，**同样不影响布局**
- **早期版本在终端上方加了一条横幅，把终端高度往下推了 28px**——这就是"影响了页面结构"。
  现在全部改成覆盖层：终端尺寸不变、不需要重新 `fit()`、远端行列数也不变
- 常驻元素只有一个 22px 高的右上角徽标（`AI 接管中 3/20 · 停止`，半透明、`pointer-events` 只在自己身上）；
  需要用户处理（密码 / 确认 / 退出 vim）时才在顶部居中展开一条醒目的提示条 + 主按钮
- `prefers-reduced-motion` 下关掉流动与呼吸动画，只保留内发光与徽标

### 7.3 输入区交互（参照 ChatGPT / Gemini / DSH 这类 agent 工具）

最终形态（自上而下）：

```
┌ AI 辅助 ─────────────────────────────┐
│ ● 执行中 3/20 正在安装 node…  [停止]  │  ← 状态条：只在接管/需要处理时出现
│                                      │
│          对话流（可滚动、自动贴底）    │
│                                      │
│ 上下文：最近200行▾ 6.2KB   预览 重置对话│  ← 功能行：纯文字按钮，无边框
│ ┌──────────────────────────────┐     │
│ │ 给 AI 发消息…（默认 2 行，可增高）│     │
│ │                  [●建议▾] (↑) │     │  ← 权限下拉 + 发送：输入框内右下角
│ └──────────────────────────────┘     │
└──────────────────────────────────────┘
```

| 行为 | 说明 |
|---|---|
| **发送 = 继续对话** | 会话默认带上下文：沿用同一 `sessionId`，后端历史保留，AI 记得前面聊过什么、跑过什么命令 |
| **重置对话 = 新会话** | 清空本地消息 + 后端 `AIReset`，下一次发送是全新的一段（也顺带丢掉旧的终端上下文） |
| **不再有「追加任务」** | 追加任务与发送本质是同一件事（都带上下文），保留两个按钮只会让人困惑，已删掉 |
| **权限（模式）下拉在输入框内右下角** | 与 DSH 一致：`[●建议▾] (↑)` 并排贴右下角。圆点颜色即风险等级（绿=建议 / 黄=直接执行 / 红=完全接管），下拉项里每项带一句说明 |
| **模式切换即确认** | 切到「完全接管」当场弹危险确认（拒绝则留在原模式）；切到「直接执行」弹一次轻量确认；发送时不再重复弹窗 |
| **不做多余提示** | 输入框下方不再有常驻说明文字（模式说明放进下拉项与 tooltip）；上文档位做成纯文字下拉，不带边框与内边距 |
| **命令卡动作不换行** | 复制/插入/执行改成自绘的小按钮（`el-button` 自带外边距，几个挤一行时会被迫换行），与风险徽标同处一行、`nowrap` |
| **自动贴底** | 用"贴底意图"（用户是否在底部附近）决定要不要跟随，而不是每次现算位置——状态条出现/消失会让列表高度突变，现算会判成"已不在底部"从而不滚，最后一条就被遮住一半。另外挂了 MutationObserver 兜住"折叠展开"这类不经 push 的高度变化 |

### 7.4 建议 / 直接执行模式也有会话记忆

后端 `AIChat` 现在也走会话：

- 历史只存**问题与回答**，终端屏幕内容**每次重新取**（否则旧屏幕内容会把上下文撑爆，而且会误导模型）
- 历史条数上限 `aiSuggestHistoryMax = 16`（一问一答算两条），裁完若以 assistant 开头再丢一条
- 「直接执行」跑完一条后，把 `退出码 + 输出` 以 `【终端输出】` 形式写回历史（大块输出按 `aiHistoryKeepRaw=2` 折叠）——
  于是下一条消息里 AI 知道"刚才那条命令跑出了什么"，直接执行模式也能一步一步往下推

### 7.5 暂停时必须把键盘交还给用户

`ai:state` 带 `kind` 字段，前端据此决定锁不锁键盘：

| kind | 含义 | 键盘/粘贴 | 按钮 |
|---|---|---|---|
| `input` | 等密码 / y-n 确认 | **放行** | 「输入完成，继续」（主，右对齐） |
| `altscreen` | 进了 vim/top 等全屏程序 | **放行** | 「输入完成，继续」 |
| `stall` | 长时间无输出 | 保持锁定 | 「继续等待」 |
| （空） | 正常执行中 | 锁定 | 「停止」 |

后端给出的引导文案是"下一步做什么"的口吻，例如：
「命令需要你输入密码（检测到提示：password:）。终端已解锁：请直接输入或粘贴密码，
按回车提交——提交后 AI 会自动继续；若没有继续，再点「输入完成，继续」」。

> 两点属于明确的需求修正：
> ① 早期版本把粘贴也一起锁了，长密码没法粘——现在只在"AI 真正在执行命令"时锁输入，
>    暂停等输入时手敲与粘贴都放行；
> ② 「发送 Ctrl+C」按钮与右侧「停止」功能重叠（都是"把这一步掐掉"），已从界面与后端
>    控制通道里一起去掉，暂停时只保留唯一需要的动作「输入完成，继续」。
>
> 另外：提交密码后**通常不需要点按钮**——AI 一直在等那一步的结束标记，命令跑完标记就出现，
> 循环自动往下走；「输入完成，继续」是兜底（比如命令其实早就结束了、或它还需要再输一次）。
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
| `prj/frontend/src/views/terminal/QuickPanel.vue` | 顶部两个页签（预设操作 / AI 辅助） |
| `prj/frontend/src/views/terminal/AiPanel.vue` | 新增：对话流 / 命令卡 / 输入框 / 停止 / 清空 / 新建对话 / 上下文档位 |
| `prj/frontend/src/views/terminal/Index.vue` | 接管锁定 + 沿边框流光（覆盖层）+ 紧停 |
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
| 最大轮数到达 | 暂停并提示；直接再发一条消息就接着往下跑（会话带上下文），或点「重置对话」重开 |

## 十二、明确不做（本期）

1. 不做多终端并行 AI、不做同步模式下的 AI
2. 不做多 Agent 并行
3. 不做无人值守长跑（到轮数上限就停）
4. 不做流式响应
5. 不做对话持久化
6. 不做对本地 Windows 机器的操作（只操作远端）
7. 不引入 markdown 库（自带约 60 行 `markdown-lite.js`）

## 十三、实现记录（已落地，含踩坑）

### 13.1 后端落点

| 模块 | 说明 |
|---|---|
| `internal/aiclient` | OpenAI 兼容客户端（`/chat/completions`，非流式）。`Endpoint()` 容忍 `…/v1`、`…/v1/`、不带 `/v1`、整条路径、`localhost:11434/v1`（自动补 `http://`）等填法；错误一律带 **HTTP 状态码 + 响应体前 500 字**，并给出"是不是漏了 /v1 / 模型名拼错"的提示 |
| `internal/ansi` | `Strip`（剥离 CSI/OSC/DCS、折叠 `\r` 覆盖、退格、rune 对齐）、`FoldRepeats`、`TailLines`、`Elide`（头 20% + 尾 80%）、`HasAltScreen`、`LooksLikePrompt` |
| `prj/terminal.go` | `terminalTail` 环形缓冲（512KB + 全局偏移 `seq/base`）+ `snapshot(from)`；在原有读输出的 goroutine 里追加一处 |
| `prj/ai.go` | 配置读写（Key 用 `crypto` 加密存 `app_settings`，查询只回 `apiKeySet`）、`AITerminalSnapshot`、`AIChat`、`AIExecOne`、`AIStartTakeover/AIStopTakeover/AITakeoverControl/AIReset`、接管循环、标记注入与回读、命令风险判定、审计落库 |
| `prj/internal/store` | `command_history` 增列 `source` / `ai_session_id`（走 `hasColumn` 幂等迁移），AI 执行的每条命令都留痕 |

配置键：`ai.baseURL` / `ai.apiKey`(密文) / `ai.model` / `ai.maxRounds`(默认 20) / `ai.timeout`(默认 60s) / `ai.contextLines`(默认 200)。
保存时 `apiKey` 留空表示"不改动已存的 Key"，避免用户只改模型名就把 Key 清掉。

### 13.2 三个必须记住的坑（都已被单测固定）

**① 结束标记必须在「原始字节流」里找，不能在清洗后的文本里找。**
标记行末尾是 `\r` + `ESC[K`（把它从用户眼前擦掉），而 `ansi.Strip` 会把"单独的 `\r` 覆盖行"整行丢弃——
在清洗后的文本里找标记永远找不到，每一步都会等到超时。
正确做法：`findMarker(rawText, key)` 定位，然后再 `ansi.Strip(rawText[:idx])` 得到输出。
（`TestFindMarkerAgainstRealisticStream` 同时断言"清洗后确实找不到"，防止后人改回去。）

**② 注入文本里不能出现完整标记。**
终端会回显我们注入的命令行，如果格式串里写着完整标记，回显就会命中标记 →
"命令还没跑"被判成"已经跑完"。所以标记用 `%s` 拼出来：

```
printf '\nX1%sS%sD%sX1E\r\033[K' '' '<stepID>' "$?"
```

回显里只有 `X1%sS%sD%s` 这种碎片，永远拼不出完整标记。`findMarker` 另外要求标记后面必须是数字，
双层保险。

**③ `ansi.Strip` 必须把 `\r\n` 当普通换行。**
PTY 输出基本都是 CRLF，如果只按"`\r` 就是回到行首、丢弃本行"处理，每一行都会被吃掉。

### 13.3 接管循环的暂停/恢复语义

`paused` 不是"停下来等人"，而是"**继续轮询标记 + 同时听控制指令**"：

| 触发 | 行为 |
|---|---|
| 提示词（`password:`、`[Y/n]`、`是否继续` 等）/ 备用屏（vim/top）/ 静默超过 2 分钟 | emit `paused` + 原因；循环继续等标记 |
| 用户点「继续等待」 | 清掉暂停标记、重置静默计时（用户已经手动处理了终端里的提示，命令会继续跑完并打出标记） |
| 用户点「发送 Ctrl+C」 | 注入 `\x03`，收走已有输出，状态 `interrupted`，本轮结束 |
| 用户点「停止」/ 紧急释放 | 注入 `\x03` 并结束整个任务，状态 `cancelled` |
| 单步超过 5 分钟硬上限 | 注入 `\x03` 收尾，状态 `timeout`（不让终端被跑飞的命令长期占住） |

接管开始前会先探测 shell（`printf '%s' "$0"`）：fish/csh/tcsh 不支持这套 POSIX 语法，
直接拒绝进入接管模式并说明原因（建议模式不受影响）。

模型回复解析走三级兜底：直接 JSON → 去 ``` 围栏 → 提取第一个花括号平衡的对象；
**全失败就当纯文本展示并停止，绝不猜命令**。

### 13.4 前端落点与三个关键决定

| 文件 | 说明 |
|---|---|
| `terminal/AiPanel.vue`（新） | 三模式切换（输入框下方的 chip，切换即确认）、读取档位、对话流（用户/AI 文本/命令卡/结果卡/状态条）、顶部状态条（暂停时给「输入完成，继续 / Ctrl+C / 停止」）、发送、重置对话、上下文预览；**对话按终端隔离**；新消息自动滚到底 |
| `terminal/QuickPanel.vue` | 顶部两个页签（预设操作 / AI 辅助）；预设操作保持原来的双栏布局；AI 页签内嵌 `AiPanel`（`v-show`，切页签/收起都不丢对话） |
| `terminal/Index.vue` | 收起时的竖排双展开按钮、接管锁定（禁切标签 / 禁新建关闭 / 运行时拦键盘粘贴 / 禁同步）、接管覆盖层（沿边框跑的光 + 内发光 + 右上徽标 + 暂停提示条）、连按 3 次 Esc 紧急停止 |
| `utils/markdown-lite.js`（新） | 极简渲染（代码块/行内代码/粗体/标题/列表），先转义 HTML 再拼标签，不引入 markdown 依赖 |
| `views/settings/Index.vue` | 「AI 设置」卡片：常用预设下拉（DeepSeek / 通义兼容模式 / OpenAI / 硅基流动 / 本地 Ollama / 自定义）、地址占位符与"填到 /v1 为止"的说明、Key 密码框、模型、最大轮数、超时、上下文行数、测试连通 |

**① 对话按终端隔离。**
`AiPanel` 里 `chats` 是一个以**终端 key** 为键的响应式表，每个终端各有一份
`{messages, sessionId, takeoverState, takeoverFull, lastContext}`；切换终端就是换一段对话
（切回来记录还在），终端标签被关掉时对应记录会被清掉，后端也会随 `TerminalClose`
一起收掉该终端的 AI 会话。接管进行中禁止切换终端，所以不会出现"接管跑在 A、面板展示 B"。

**② 新消息自动滚到底。**
每次 push 消息后 `nextTick` 把 `.ai-log` 的 `scrollTop` 设为 `scrollHeight`；
终端切换与页面激活时也各滚一次。

**③ 侧栏折叠改用 `v-show`，不再用 `v-if`。**
侧栏收起时若销毁 `QuickPanel`，AI 对话记录与 `ai:*` 事件订阅会一起丢掉，
正在跑的接管就再也没人收事件了。现在收起只是隐藏。

**④ sessionId 由前端生成，且必须"先订阅事件、再发起请求"。**
AI 失败往往很快（Key 错、模型名错），等 IPC 返回再订阅会漏掉首个 `ai:state` 事件，
界面会一直停在"执行中"。

### 13.5 单条执行与接管的会话生命周期

- 一条命令的"直接执行"也登记成一个会话 → 于是「停止 / 继续等待 / Ctrl+C」对单条执行同样可用
- 只为这次执行创建的会话跑完即销毁；**复用了已有会话（例如刚跑完接管的对话）则不动它**，否则会把整段对话一起清掉
- 用同一个 sessionId 再次发起任务时，会先复位 `cancelled/paused/round/control` ——
  上一次 `stop()` 留下的 `cancelled=true` 不清掉的话，新循环会刚起步就退出
- 前端捕获到异常（任务根本没起来）时，必须把 `takeoverFull` 收回来，否则终端会永久停在锁定态

### 13.6 单测清单

- `internal/aiclient/client_test.go`：地址拼接的 9 种填法、成功、401/404/非 JSON/空 choices、reasoning 回退、空回复、缺配置、超时、`Ping` 的 `max_tokens=1`
- `internal/ansi/strip_test.go`：颜色与 OSC 标题、`\r` 折叠（含 spinner）、**CRLF 保留**、退格、中文与非法字节、bracketed paste、备用屏检测、提示词识别与"取最靠后的提示"、重复行折叠、尾行、截断（头尾都在）
- `prj/ai_test.go`：注入文本**不含完整标记**、单行/多行注入、真实终端流下的标记定位与输出提取、伪命中跳过、回显剥离、stepID 唯一性、回复解析四级场景（JSON/围栏/散文/垃圾→停止）、`commands` 数组、建议模式解析与代码块兜底、平衡括号提取、风险判定三档、`clampInt`

## 十四、验收清单

**已自查**：`go vet ./...` 与 `go test ./... -count=1` 全绿（新增 aiclient / ansi / ai 三类共 40+ 用例）、`npm run build` 与 `wails build` 均成功产出 `prj/build/bin/ServerManager2.exe`；设置页字段/预设/测试连通、右侧双页签、接管锁定与沿边框流光、会话上下文延续、事件订阅时机等都做了代码级核对。

**需要真机（配好 AI、连一台测试服务器）验收**：

1. 设置页能配置 base URL / Key / 模型 / 最大轮数，示例文案让人清楚"填到 /v1 为止"；「测试连通」对错误 Key 能显示 401 与响应体
2. 建议模式：问"怎么查 XXX.jar 的进程"→ 得到命令卡 → 「复制」可粘贴运行；「插入到终端」只键入不回车
3. 建议模式：终端跑出一堆报错后问"现在的终端内容发生了什么"→ AI 能读到当前终端最新内容并解释
4. 直接执行模式：AI 命令自动在可见终端执行一次并回读结果，随后停下等待
5. 完全接管模式：**切到该模式时就弹危险警告**；确认后终端出现沿边框流光与右上角「AI 接管中」徽标（页面布局不变）
6. 接管模式：输入"帮我安装 X"→ AI 逐条执行、监控输出、遇错自行修正，完成后自动停止并给总结
7. 接管模式：点「停止」能立刻停；停下后**直接再发一条消息**就接着往下跑（会话带上下文），点「重置对话」则开一段全新会话
8. 接管中：无法切换标签、无法开启同步、键盘输入被拦截并提示；连按 3 次 Esc 可紧急停止
9. 接管中切到别的页面再回来：任务仍在继续，对话流完整
10. 「清空对话 / 新建对话」可用；应用重启后对话不残留
11. 主密码锁定：AI 立即停止，面板置灰
12. 未配置 AI 时进入终端页：AI 区给出「去设置」引导，不影响其他功能
13. `go vet` / `go test` / `npm run build` / `wails build` 全绿