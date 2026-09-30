# 运维工具（磁盘空间检测）设计方案

> 本文档对应「运维工具」阶段的实现计划，实现范围严格限定为**只分析、不处置**：
> 报告里出现的处置命令都只是可复制的建议文本，本页不执行任何写操作。

## 一、需求拆解

真实痛点：服务器磁盘被占满后，用户要排查很久才能定位到"是哪个日志 / 哪个目录 / 哪个文件"占的空间。

目标：把「选服务器 → 点一下 → 三秒内出第一段结论 → 逐层下钻到具体文件」变成一次点击。

1. 左侧菜单新增「运维工具」，服务器列表行操作新增「监测运维」（跳该页并预选服务器）
2. 页面内选服务器 + 选盘 → 点「开始检测」
3. **3 秒内**先出瞬时结论（容量 / inode / 日志服务占用 / 已删除句柄），目录占用排行随后到达
4. 目录排行可逐层下钻（`/var/lib/docker` → `containers` → 某容器目录 → `xxx-json.log 5.2 GB`），面包屑可回退
5. 报告给出**差额解释**（"已用 X GB，但目录扫描只找到 Y GB"）
6. 「复制报告」得到 Markdown 全文；「打开终端」跳到该服务器终端去处置
7. 权限不足时提供「用 root 重跑」

工具清单用数组驱动，本期只放「磁盘空间检测」为可用项，其余（内存与进程、服务器体检、日志体积）为"即将上线"占位，便于后续扩展。

## 二、可复用的现成能力

| 能力 | 现状 | 复用于 |
|---|---|---|
| 标记分节 + 一次 SSH 多段输出解析 | `diskCommand()` / `splitDiskSections()`（`internal/sshx/sshx.go`） | 检测脚本的分节范式，照抄 |
| 全盘文件系统采集 | `GetDisks()` / `parseDf()` / `pickKeyDisk()` | 盘下拉与默认盘选择 |
| 目录体积计算 | `DirSize()` / `dirSizeByDu()`（`du -sbP` → `--block-size=1` → `-sk` 三重回退） | 下钻与 BusyBox 兼容策略 |
| 非交互命令执行（支持 sudo） | `ExecCommand(cmd, timeout, useSudo)` + `buildSudoCommand` | root 重跑 |
| 并发任务 + 事件推送 | `tasks.RunConcurrent`、`runtime.EventsEmit`、`monitor.go` 的 `monitors` 任务表 | 异步检测与进度事件 |
| 终端页按 serverId 直达 | `terminal` 路由的 `query.serverId` 已有处理 | 「打开终端」 |
| 剪贴板回退写法 | `terminal/Index.vue` 的 `writeClipboard()` | 「复制报告」 |
| 路由 / 菜单 / 列表操作列 | `router/index.js`、`Layout.vue` 的 `menus`、`List.vue:137` 操作列 | 页面入口 |

**结论：后端新增一个 sshx 子模块 + 一个 ops.go；前端新增一个页面目录，其余全是既有模式复用。**

## 三、界面设计

### 3.1 页面结构：工具卡片首页 → 工具页

左侧已有全局导航菜单，所以**不再在页面内放第二个左侧菜单**。`/ops` 是"工具卡片首页"，
点卡片进入工具页（同一个路由组件内的视图切换 + `?tool=xxx` 记在地址里）：

```
┌ 运维工具 ───────────────────────────────────────────────────────────────┐
│ 运维工具                                                                │
│ 选一个工具开始排查。本模块只做分析，不会修改或删除服务器上的任何文件。  │
│                                                                         │
│ ┌───────────────────────┐ ┌───────────────────────┐ ┌─────────────────┐ │
│ │ ▣  [可用]             │ │ ▣  [即将上线]         │ │ ▣  [即将上线]   │ │
│ │ 磁盘空间检测          │ │ 内存与进程            │ │ 日志体积分析    │ │
│ │ 磁盘满了，先找出是谁  │ │ 内存去哪了、谁在吃CPU │ │ 按服务统计日志  │ │
│ │ · 容量/inode/句柄一次 │ │ · 进程 TOP            │ │ · 体积排行      │ │
│ │ · 目录逐层下钻        │ │ · 增长趋势            │ │ · 增长速率      │ │
│ │ · 命令可一键复制      │ │ · OOM 记录            │ │ · 轮转检查      │ │
│ │          开始使用 →   │ │          开发中       │ │      开发中     │ │
│ └───────────────────────┘ └───────────────────────┘ └─────────────────┘ │
└─────────────────────────────────────────────────────────────────────────┘

点卡片后：

┌ ← 返回工具列表 │ 磁盘空间检测  磁盘满了，先找出是谁占的 ─── 服务器[下拉] ┐
│ 磁盘[下拉] □以root扫描 [开始检测] [刷新磁盘] [打开终端]                  │
│ ① 结论摘要：发现 1 项严重、3 项需注意，涉及约 …                          │
│    四个指标卡：空间使用 / inode / 目录扫描到 / 合计                      │
│ ② 发现清单（按级别排序，含证据折叠、建议命令 + 命令作用说明）             │
│ ③ 目录占用 + 下钻面包屑（占比进度条）                                    │
│ ④ 大文件（文件名 + 所在目录两行显示，路径可复制）                        │
└─────────────────────────────────────────────────────────────────────────┘
```

**卡片设计要求（三套主题都要成立）**：

- 尺寸 `minmax(276px, 1fr)` 自适应网格；卡片 `--card` 底 + `--card-border` 描边 + 16px 圆角 + `--card-shadow`
- 顶部 2px 渐变高光条（`--blue` → `--pink`），悬停时淡入——与终端标签选中条同一套视觉语言
- 右上角一枚 150px 的 radial 光斑，用主题内的 `--halo-1` 令牌，浅色/深色/赛博三套自动换色，不写死 rgba
- 42px 图标砖：`linear-gradient(135deg, var(--blue-soft), var(--pink-soft))` + 图标 `--blue-deep`，
  悬停时加一圈 `--blue-focus` 光环
- 悬停：`translateY(-3px)` + `--card-shadow-hover` + 边框转 `--blue`；间距/字号统一走 4px 栅格
- 未上线卡片降到 0.74 不透明度、按钮文案变"开发中"，点击给一句提示而不是静默无反应
- `prefers-reduced-motion` 下关掉位移与过渡
- 不写死任何颜色：三套主题（浅色 / 深色 / 赛博）全部由 `style.css` 的令牌驱动

### 3.2 长文本处理（超长文件名不再溢出）

| 位置 | 处理 |
|---|---|
| 结论标题 | 后端标题只放「体积 + 文件名」，完整路径放 `detail`（标题带整条路径时，前端再怎么截断都会顶出可视区） |
| 结论标题样式 | `min-width: 0` + `overflow-wrap: anywhere`（`flex-wrap` 只在子项之间换行，不会在子项内部断开） |
| 结论说明 | 同样允许任意位置断行 |
| 大文件行 | 拆成两行：文件名（可断行、完整可见）+ 所在目录（单行省略号 + `title` 提示），右侧复制按钮固定不被挤出 |
| 目录行 | 单行省略号 + `title` 悬浮看全路径（列表场景下换行会让行高参差） |
| 建议命令 | 单行不换行、横向可滚动（命令换行会看错参数） |

### 3.3 「建议命令」必须说明作用

报告里的命令是**给人复制到自己终端去跑的**，所以每条命令都必须带 `CommandNote`，
说清「作用」与「运行后」，例如：

- `truncate -s 0 /var/log/x.log` → 作用：清空内容但保留文件与 inode；运行后：空间立刻释放，
  写入进程不报错、不用重启（这正是它比 `rm` 安全的原因）。
- `lsof +L1` → 作用：列出已删除但仍被进程打开的文件；运行后：只读，按进程名重启服务即可释放。
- `journalctl --vacuum-size=200M` → 作用：把归档日志收缩到 200MB；运行后：删较早的归档条目，
  不影响正在运行的服务，不需要重启。

`CommandNote` 在 UI 上以虚线框小字显示在命令下方，在复制的 Markdown 报告里以引用块跟在命令后面。
`diskanalyze_test.go` 的 `TestEveryCommandHasNote` 强制约束：凡是给了 `Command` 的结论，
`CommandNote` 必须同时包含「作用」和「运行后」，漏了就测试失败。

- 服务器下拉：`api.listServers()`；选服务器后调 `ListServerDisks` 拉盘（约 1s）；服务器记忆在 `xx1.ops.serverId`
- 盘下拉：列出全部文件系统，默认选中**使用率最高的真实可写盘**（与后端 `pickKeyDisk` 口径一致）
- 「以 root 权限扫描」复选框：勾选后本次扫描与下钻都走 sudo（用已保存的服务器密码）；报告里出现权限不足时，
  在该条结论上直接给「用 root 重跑」按钮（勾选后自动重新检测）
- 从服务器列表「监测运维」进来时：预选服务器 + 预选工具，**不自动开始**（不做无预告的 SSH 连接）
- 组件名 `OpsPage`，加入 `Layout.vue` 的 `<keep-alive :include>`，保证检测进行中切页不中断
- 「开始检测」后进度文案分阶段："正在检查容量、inode、系统日志与已删除句柄…" / "正在扫描目录占用（大目录可能需要十几秒）…"，带「取消」
- 「复制报告」把整份报告导成 Markdown（结论 + 建议命令 + 原始输出 + 目录 TOP + 大文件），方便贴到工单里

### 发现清单配色

| Level | 颜色令牌 | 含义 |
|---|---|---|
| `critical` | `--red` | 就是问题本身（inode 满、已删除句柄、大文件、>90% 占用） |
| `warning` | `--yellow` | 可疑但需人工判断（journal 膨胀、docker 可回收、差额过大） |
| `info` | `--blue` | 提示性（权限不足、伪文件系统、预留块、检查项不可用） |
| `unavailable` | `--ink-3` | 该检查项在此机器上不可用（未安装 docker/lsof/journalctl） |

每条卡片：级别徽标 + Kind 徽标 + 标题 + 体积 + 说明 + `Evidence` 折叠（等宽原始输出）
+ `Command` 复制按钮 + `CommandNote` 命令作用说明（**只复制，不执行**）。

### 目录排行

- 每行：路径 + 体积 + 占该盘已用容量的百分比进度条（≥30% 转红）
- 点击任意一行 → 下钻一层；面包屑可点任意层回退，另有「上一层」「磁盘根目录」
- 下钻走 `DrillDiskDir`（一次 du -xd1，超时 40s）；没有子项或无权限时给出提示

**下钻的加载提示必须是局部的**（`du` 扫大目录十几秒是正常的，但要让人知道在跑）：

- 遮罩只覆盖「目录占用」卡片里的**列表区域**（`.dt-dirs-wrap` 内 `position: absolute`），
  卡片标题、面包屑、页面上其它按钮都不受影响，也绝不用 `v-loading` 挂在页面根上
- 遮罩内容：转圈 + 「正在统计该目录的体积…」+ 被扫描的路径（单行省略）+ 「已用 Ns · 目录越大越慢」
- 已统计的那一层留在原地并压暗（`opacity: .42` + `pointer-events: none`），
  保留"我在这一层往下钻"的上下文，而不是清空成骨架屏
- 下钻进行中禁掉「开始检测」（避免两次扫描互相盖结果）；组件卸载时清掉计时器

**超时中断要如实说**：`du` 被 40s 上限打断时，`ExecCommand` 返回 -1 但**已经拿到部分输出**，
`DiskDrill` 会把这种结果标成 `Partial=true` 并附 `Note`，前端在列表上方用一条黄色提示说明
"只列出了已统计到的部分子目录（体积数字也会偏小）"——不这样做，用户会把不完整的列表当成该目录的全部子项。

## 四、后端设计

### 4.1 采集脚本（新增 `internal/sshx/diskanalyze.go`）

**硬约束**：`buildSudoCommand` 是**按行**给每条命令加 `echo 'pw' | sudo -S ` 前缀，所以每一行必须单行自洽——不得跨行写 `for` 循环、不得跨行 heredoc。挂载点用 `shellQuote(mount)` 直接内插，不用 shell 变量。所有命令走管道、不落临时文件（磁盘已满时不能依赖写盘）。

**分两次 SSH 调用**是"一键"体感的关键：瞬时段先出结果，体积段随后到达。

#### 第 1 段 · 瞬时检查（超时 30s，目标 3s 内出结果）

```
echo 'X1SA1'
df -PTk MOUNT 2>/dev/null || df -Pk MOUNT 2>/dev/null || df -m MOUNT 2>/dev/null
echo 'X1SA2'
df -Pi MOUNT 2>/dev/null || df -i MOUNT 2>/dev/null
echo 'X1SA3'
cat /proc/mounts 2>/dev/null
echo 'X1SA4'
journalctl --disk-usage 2>/dev/null || echo 'X1NA'
echo 'X1SA5'
docker system df 2>/dev/null || echo 'X1NA'
echo 'X1SA6'
lsof +L1 2>/dev/null | head -40
echo 'X1SAEND'
```

- 各检查项都靠「命令不存在 → 输出为空 / X1NA」来判定不可用，**不使用 `if command -v`**：
  `if` 是 shell 关键字，行首关键字在 sudo 模式下会被 `echo 'pw' | sudo -S if ...` 拆坏（见 4.6）。
- 预留块检查不单独发命令（原计划的 `tune2fs` 已去掉）：改为在结论里由
  "使用率 ≥ 99% 但仍有可用空间" 推断并解释 ext4 的 5% 保留块。
- `lsof` 不可用时（输出里没有 `COMMAND` 表头）由 Go 侧补一条
  `find /proc/[0-9]*/fd -lname '*deleted*'` 兜底（只能确认存在，拿不到体积，报告里会写明）。

#### 第 2 段 · 体积段（超时 60s，可取消）

```
echo 'X1SB1'      # 能力探测（便宜）
du -d1 -k /tmp >/dev/null 2>&1 && echo 'X1DUOK' || echo 'X1DUOLD'
find MOUNT -xdev -maxdepth 0 -printf 'X1FINDOK' 2>/dev/null || echo 'X1FINDOLD'
echo 'X1SB2'      # 一级目录占用
du -x -d1 -k MOUNT 2>&1 | sort -rn | head -30
echo 'X1SB3'      # 专项大文件（>100MB）
find MOUNT/var/lib/docker/containers MOUNT/var/log MOUNT/var/lib/mysql MOUNT/var/log/journal MOUNT/opt MOUNT/root -xdev -type f -size +104857600 -printf '%s %p\n' 2>&1 | sort -rn | head -30
echo 'X1SB4'      # 全盘大文件（>200MB）
find MOUNT -xdev -type f -size +209715200 -printf '%s %p\n' 2>&1 | sort -rn | head -20
echo 'X1SBEND'
```

多路回退：
- 一级目录：主命令**没有返回任何条目**时（`-d1` 不被支持、或 du 被超时杀掉），自动用
  `du -x -k --max-depth=1 MOUNT` 重试一次，并在报告备注里说明；
- 大文件：探测到 `find` 不支持 `-printf` 时，改用
  `find <paths> -xdev -type f -size +204800k -exec ls -l {} \; | awk '{print $5, $9}'`（路径含空格时可能显示不全，报告里会提示）。

`2>&1` 是故意的：把 `du`/`find` 的 `Permission denied` 一起带回来，解析器按"首字段必须是整数"
自动跳过这些行，同时让报告能识别出「有目录因权限不足没算进去」。

#### 下钻

`du -x -d1 -k <path> | sort -rn | head -25`（超时 30s，同样三重回退）。

### 4.2 解析与推断（纯函数，可单测）

**必须注意的坑**：`df -Pi` 输出是 6 列 `Filesystem Inodes IUsed IFree IUse% Mounted on`，会被现有 `parseDf` 的六列分支**当成 1K 块容量解析**，产出 `total = inodes/1024` 的假磁盘。因此 inode 段必须用独立的 `parseDfInodes()`，绝不能复用 `parseDf`。

解析函数清单：`parseDfInodes`、`parseDuDepth1`、`parseFindSizeLines`、`parseJournalUsage`、`parseDockerDf`、`parseLsofDeleted`、`parseReservedBlocks`（`parseReadOnlyMounts` 已有）。

推断规则 → `[]DiskFinding`：

| Level | Kind | 规则 |
|---|---|---|
| critical | `usage` | 目标盘使用率 ≥ 90% → "可用空间仅剩 X，继续写入会失败" |
| critical | `inode` | inode 使用率 ≥ 85%（≥95% 升级为 critical）→ "空间还有余量但 inode 先满，通常是海量小文件"；inode 使用率高于空间使用率时额外点明瓶颈在 inode |
| critical | `deleted-open` | 已删除句柄段有行 → 列出进程、PID、体积；说明"用 `rm` 删了但进程还持有句柄，空间不会释放"；lsof 不可用（只能确认存在）时标注体积未知并说明原因 |
| warning | `gap` | `df.UsedMB - Σ(du 一级)` 占比 > 30% → 列出四类可能原因（已删除句柄 / 权限不足 / 挂载点被覆盖 / du 与 df 口径差异） |
| warning | `journal` | journald 占用 ≥ 1 GB → 建议 `journalctl --vacuum-size=200M` |
| warning | `docker` | `docker system df` 的可回收 ≥ 1 GB → 列出镜像/容器/数据卷/构建缓存明细与 prune 建议 |
| warning/critical | `bigfile` | ≥1GB 的文件逐个成条（≥5GB 升级为 critical）；日志类给 `truncate -s 0`（并解释为什么不要 `rm`），其余给 `ls -lh` 让用户先确认；容器 `*-json.log` 额外给日志轮转建议 |
| info | `reserved` | 使用率 ≥ 99% 但仍有可用空间 → 解释 ext4 等文件系统的 5% 保留块（root 仍可写属正常） |
| info | `permission` | 扫描输出里出现 `Permission denied` → 提示可用 root 重跑（前端在该条上直接给按钮） |
| info | `pseudo` | 目标盘是 tmpfs/overlay 或只读挂载 → 提示不参与"快满"预警 |
| unavailable | `inode` / `journal` / `docker` | 对应检查不可用（该 fs 不统计 inode / 未安装 journalctl / 没有 docker）→ 明确说明并跳过，不算错误 |

> 挂载点被覆盖（mount-shadow）没有做成独立检查：它的表现就是"差额很大"，已在 `gap` 结论里列为可能原因之一，避免为一个小概率场景多发一轮命令。

### 4.3 汇总口径（写进报告文案，避免误导）

`du` 统计的是 apparent/块占用，与 `df` 口径不同。报告**不做"这些加起来正好等于已用"的暗示**，而是显式给出 `ScannedMB` / `GapMB` 与差额解释——这个差额本身就是最重要的诊断信号。

### 4.4 数据模型

```go
type DiskFinding struct {
    Level, Kind, Title, Detail, Evidence, Command, CommandNote string
    SizeMB int64
}
type DiskDirItem struct {
    Path string; SizeMB int64; Percent float64; HasChildren bool
}
type DiskReport struct {
    TaskID, ServerID, ServerName string
    Mount, Device, Fstype string
    TotalMB, UsedMB, AvailMB int64
    Usage float64
    InodeTotal, InodeUsed int64
    InodeUsage float64
    Findings []DiskFinding
    TopDirs  []DiskDirItem
    BigFiles []DiskDirItem
    ScannedMB int64
    GapMB int64
    ReclaimableMB int64
    Partial bool
    Stage string            // quickscan | done
    Notes []string
    ElapsedMs int64
    ScannedAt string        // "2006-01-02 15:04:05"
}
```

> 落库/传输口径：`DiskReport` 只在事件里传给前端，不作为绑定方法的参数或返回值，
> 因此它不出现在 `wailsjs` 的 KnownStructs 里（事件按普通 JS 对象消费即可）。
> 所有切片字段在组装时都强制成非 nil —— Go 的 nil 切片会序列化成 JSON `null`，
> 前端 `v-for` 会直接报错。

### 4.5 绑定方法与事件

```go
func (a *App) ListServerDisks(serverID string) ([]models.DiskInfo, error)
func (a *App) StartDiskAnalysis(serverID, mount string, useSudo bool, taskID string) (string, error)
func (a *App) CancelDiskAnalysis(taskID string) error
func (a *App) DrillDiskDir(serverID, mount, path string, useSudo bool) (models.DrillResult, error)
```

**taskID 由前端生成、并且必须先订阅事件再调用 `StartDiskAnalysis`**：检测是异步的，
如果等 IPC 返回拿到 taskID 再订阅，那么"SSH 连不上 → 立刻报错"这类极快失败的首个事件
会在订阅之前发出，界面就会一直停在"分析中"。`StartDiskAnalysis` 收到空 taskID 时自己生成
（供脚本/测试调用）。

状态表：`a.opsMu sync.Mutex` + `a.diskTasks map[string]*diskTask`（与 `a.monitors` 同构）。
取消不是只置标记：`diskTask.abort()` 会**直接掐断 SSH 连接**，正在跑的 `du`/`find` 立刻返回，
用户点「取消」不用等到超时。同一服务器同时只允许一个检测在跑。

主密码锁定时 `auth.Lock()` 会一并 `closeAllDiskTasks()`（与关闭终端/监控/文件连接同处）。

事件：`ops:disk:progress:<taskID>` / `ops:disk:report:<taskID>` / `ops:disk:done:<taskID>`。

### 4.6 脚本的 sudo 行约束（踩过就必须写下来）

`sshx.buildSudoCommand` 是**按行**处理的：给每一行加 `echo '<密码>' | sudo -S ` 前缀。因此脚本必须满足：

1. **每行单行自洽**——不能跨行写 `if / for / while`、不能跨行 heredoc（多行 `for` 循环的中间行会被单独包成
   `echo pw | sudo -S find ...` 而语法错误）。所以大文件扫描改成 `find <多个路径> ...` 一次搞定，
   不用 `for` 循环。
2. **行首不能是 shell 关键字**（`if`/`for`/`while`/`case`/`{`）——`sudo if` 不是可执行命令。
   检查项存在性因此改为"跑一次看输出"，而不是 `if command -v`。
3. **不使用 shell 变量赋值行**（`VAR=x` 在 sudo 模式下只会设进 sudo 的临时环境）。
   挂载点用 `shellQuote()` 直接内插字面量。
4. 顺带修了一个既有缺陷：原实现用 `echo '%s'` 拼密码，密码含单引号时整条命令都会被拆坏；
   现改为先做 `'` → `'\''` 转义（`diskanalyze_test.go` 里有对应单测）。

`diskanalyze_test.go` 里的 `TestDiskScriptsAreSudoLineSafe` 会对每个脚本逐行断言上述 1、2 两条，
以后新增脚本/新增检查项时不会悄悄踩回去。

## 五、组件与文件改动清单

| 文件 | 改动 |
|---|---|
| `prj/internal/sshx/diskanalyze.go` | 新增：脚本构造 + 全部解析 + 推断 |
| `prj/internal/sshx/diskanalyze_test.go` | 新增：纯函数单测 |
| `prj/internal/models/models.go` | 追加 `DiskFinding` / `DiskDirItem` / `DiskReport` |
| `prj/ops.go` | 新增：4 个绑定 + 任务表 + 事件推送 |
| `prj/app.go` | 追加 `opsMu` / `diskTasks` 字段与初始化 |
| `prj/internal/sshx/sshx.go` | 修 `buildSudoCommand` 密码单引号未转义 |
| `prj/frontend/src/api/index.js` | 新增 4 个封装 |
| `prj/frontend/wailsjs/**` | `wails generate module` 重新生成 |
| `prj/frontend/src/router/index.js` | 新增 `/ops` 路由 |
| `prj/frontend/src/views/Layout.vue` | `menus` 加「运维工具」+ `keep-alive include` 加 `OpsPage` |
| `prj/frontend/src/views/servers/List.vue` | 操作列加「监测运维」+ 列宽 250 → 330 |
| `prj/frontend/src/views/ops/Index.vue` | 新增：服务器选择 + 工具清单 + 工作区 |
| `prj/frontend/src/views/ops/DiskTool.vue` | 新增：盘选择 + 结论摘要 + 发现清单 + 目录下钻 + 大文件 |
| `prj/frontend/src/utils/clipboard.js` | 新增：剪贴板回退 + `fmtMB/fmtPct` 共用格式化 |
| `prj/auth.go` | `Lock()` 里加 `closeAllDiskTasks()` |

> 前端构建注意：本机默认 `node` 是 v16，而 Vite 5 需要 Node 18+。
> 构建时把 nvm 的 v24 放到 PATH 前面（`$env:PATH = 'D:\nvm\v24.18.1;' + $env:PATH`）再跑 `npm run build` / `wails build`。

## 六、边界情况与失败模式

| 场景 | 处理 |
|---|---|
| 未解锁 / 凭据缺失 | 错误卡片显示后端文案，可重试 |
| SSH 连接失败 | 错误卡片（`连接服务器失败：…`），可重试 |
| `du` 超时 / 用户取消 | `Partial=true`，摘要里标「结果不完整」；取消是掐断连接，立即生效 |
| 取消发生在体积段 | 重新推送一次瞬时段报告 + 提示"已取消目录扫描"，不留"进行中"假象 |
| 未选盘 | 默认落在使用率最高的真实可写盘 |
| 工具缺失（docker/journalctl/lsof） | 该项标 `unavailable` + 说明，整体仍出报告，不弹全局错误条 |
| 老版本 du 不支持 -d1 | 主命令无结果时用 `--max-depth=1` 重试，并在备注里说明 |
| find 不支持 -printf | 改用 `ls -l` + `awk` 取体积，备注里说明（路径含空格时可能显示不全） |
| 非 root 权限不足 | `permission` 项 + 结论上的「用 root 重跑」按钮 |
| 该 fs 不统计 inode | `unavailable` + "该文件系统不统计 inode"，不影响其它结论 |
| 同时分析多台 | 允许（taskID 独立事件通道）；同一服务器同一时刻只跑一个，重复点会提示 |
| 换服务器 / 离开页面 | 换服务器会销毁组件并把在跑的任务取消（不留没人看的后台扫描）；仅切路由不影响（keep-alive 保活 + 事件继续收） |
| 磁盘 100% 满 | 全程管道、不写临时文件 |
| 大目录几百 GB | `head` 截断 + 超时 + 可取消 + 按需下钻，绝不一次扫全盘两遍 |

## 七、明确不做（本期）

1. 不做一键处置 / 清理（用户明确要求本页只分析）
2. 不做报告落库与历史对比
3. 不做定时巡检与主动告警
4. 不做多服务器并行对比
5. 不做 Windows 服务器（现有监控体系本身是 Linux 假设）
6. 不做服务/端口/证书类工具（工具清单里只留占位）

## 八、实施步骤（已完成）

1. `models` 追加结构体 ✅
2. `internal/sshx/diskanalyze.go` + `diskanalyze_test.go` → `go test ./...` ✅（新增 20 个用例）
3. `prj/ops.go` + `app.go` 字段 + `auth.go` 锁定清理 → `go vet ./...` ✅ → `wails generate module` ✅
4. `api/index.js` 封装 → 前端路由 / 菜单 / 列表按钮 / `views/ops/*` ✅
5. `npm run build` ✅ + `wails build` ✅（产物 `prj/build/bin/ServerManager2.exe`）

> 构建时注意 Node 版本（见第五节末尾）：本机默认 node 是 v16，Vite 5 需要 18+。

## 九、验收清单

**已在提交前自查的项**：
- 13. `go vet ./...` / `go test ./...` 全绿；`npm run build` 与 `wails build` 均成功产出 exe
- 1/2 的静态接线：`Layout.vue` 菜单项、`/ops` 路由、`OpsPage` 加入 keep-alive、`List.vue` 操作列按钮与列宽
- 结论推断的正确性：`diskanalyze_test.go` 用静态样本覆盖了 inode 告急、已删除句柄、journal、docker、
  大文件（含容器 json 日志的轮转建议）、差额、伪文件系统/预留块、检查项不可用、体积未知等分支
- 脚本的 sudo 可用性：`TestDiskScriptsAreSudoLineSafe` 逐行断言不出现行首 shell 关键字与空行

**需要真实 Linux 服务器验收的项**（我这边没有可用的测试机，请你在测试服务器上跑）：
1. 菜单出现「运维工具」；未选服务器时工作区为空态引导
2. 列表行「监测运维」→ 跳 `/ops`、服务器已预选、默认盘已选，但不自动开始
3. 盘下拉列出全部真实盘，默认 = 使用率最高的真实盘
4. 点「开始检测」→ 3s 内出瞬时段 → 随后出目录排行；进度与「取消」可用
5. 测试机构造：5G 大文件 / 被 nginx 持有的已删除文件 / 海量小文件致 inode>85% → 三类发现都能报出
6. 点 `/var/lib/docker` 下钻 → `containers` → 某容器目录 → 看见 5G 的 `-json.log`；面包屑可回退
7. 「复制报告」得到完整 Markdown
8. 「打开终端」跳到终端页并为该服务器开好标签
9. 「用 root 重跑」→ 权限不足项消失、差额缩小
10. 缺 docker/journalctl/lsof 的环境：对应项显示"不可用"，页面不报错、仍出结果
11. 强制 du 超时（如对 `/` 大盘跑）→ 标「结果不完整」；或看老版本 du 机器上是否走 `--max-depth` 重试
12. 三主题 + 1200px 窄窗下样式正常