# 多磁盘适配设计方案（总览 + 实时监控）

> 目标：一台服务器有 2 个或更多磁盘/文件系统时，总览卡片与实时监控页都能正确、清晰地呈现每个磁盘的状态，而不是只显示 `/` 一个数字。

## 一、现状与问题

| 环节 | 现状 | 问题 |
|---|---|---|
| 采集 | `sshx.GetSystemStats()` 执行 `df -m / \| tail -1`，只取根分区 | 数据盘（如 `/data`、`/home`）完全看不到 |
| 数据 | 只回传 `diskTotal / diskUsed / diskUsage` 三个标量（单位为 MB） | 没有"多个磁盘"的结构 |
| 存储 | `monitor_history` 表只有 `disk_total / disk_used / disk_usage` 三列 | 历史趋势也只能存一个磁盘 |
| 总览 | 每台服务器一行 `磁盘` 进度条 | 两个磁盘时无法表达 |
| 实时监控 | 一张"磁盘使用率"折线图，单条曲线 | 同上，且无法分辨哪条线是哪个挂载点 |

关键结论：**问题贯穿采集 → 传输 → 存储 → 展示四层**，只改界面是不够的；同时必须兼容已有的历史数据与旧字段。

## 二、设计原则

1. **向后兼容**：保留 `diskTotal/diskUsed/diskUsage` 三个旧字段（含义改为"关键磁盘"），旧历史数据不丢、不报错。
2. **单位统一沿用现有约定**：磁盘容量继续用 **MB**（与 `GetSystemStats`、`fmtBytes` 现有的 MB 口径一致），避免出现两套单位。
3. **真实磁盘优先**：默认过滤 `tmpfs / devtmpfs / overlay / squashfs / ramfs` 等伪文件系统，避免 `tmpfs` 把"磁盘"列表刷成十几条；过滤若失败（老系统无 `df -T`）则用设备名回退，一个字都不漏报。
4. **信息密度可控**：服务器卡片高度必须与磁盘数量无关（1 个磁盘和 6 个磁盘的卡片一样高），详细视图再展开。
5. **不引入悬停风暴**：卡片上的小条用原生 `title` 而非组件化 tooltip（多台服务器 × 多磁盘会有上百个浮层）。

## 三、数据层设计

### 3.1 新增结构体（`internal/models`）

```go
// DiskInfo 单个文件系统的容量信息
type DiskInfo struct {
    Mount       string  `json:"mount"`       // 挂载点 / /data
    Device      string  `json:"device"`      // /dev/sdb1 或 server:/export
    Fstype      string  `json:"fstype"`      // ext4 / xfs / tmpfs
    TotalMB     int64   `json:"totalMB"`
    UsedMB      int64   `json:"usedMB"`
    AvailMB     int64   `json:"availMB"`
    Usage       float64 `json:"usage"`       // 0-100
    IsPseudo    bool    `json:"isPseudo"`    // tmpfs 等伪文件系统
    IsReadOnly  bool    `json:"isReadOnly"`  // ro 挂载（只读：不参与写操作预警）
}
```

`MonitorPoint` 增加两个字段：`DiskJSON string \`json:"-"\``（快照原文，仅落库）与 `Disks []DiskInfo \`json:"disks,omitempty"\``（读取时还原、随桥接返回给前端；理由见「七、实施记录」偏差 1）。

### 3.2 采集（`internal/sshx`）

```
主命令：LC_ALL=C df -PTk 2>/dev/null         # -P POSIX 布局、-T 显示类型（GNU）
回退一：LC_ALL=C df -Pk 2>/dev/null          # 无 -T 时用设备名判断伪文件系统
回退二：保持旧逻辑 df -m /                    # 全都不行时退化为根分区单条
```

解析要点（对应真实 `df` 的坑）：
- **按字段数判断而非按列号**：`df -PTk` 有 7 列、`df -Pk` 有 6 列，用"前 N-1 个字段为数值/设备，最后一个字段及之后为挂载点"的方式取值（挂载点含空格时也能正确取到）。
- 包名/设备名过长换行、`Filesystem` 表头行、空行 —— 一律跳过。
- 容量字段为 `-`（如 `/proc`）或非数字 —— 跳过该行。
- 设备名形如 `(none)`、`map -hosts`（macOS）—— 视为伪文件系统。
- 伪文件系统判定：`fstype ∈ {tmpfs, devtmpfs, devfs, overlay, aufs, squashfs, ramfs, proc, sysfs, cgroup, cgroup2, nfsd, fuse.gvfsd-fuse, 9p, nsfs}`，或设备名不以 `/dev/` 开头且不是 `host:path` 形式的网络盘。

排序：`/` 永远第一（最常用），其余按挂载点字母序，保证同一次刷新的顺序稳定。

### 3.3 回传结构（`GetSystemStats`）

```json
{
  "cpuUsage": 12.4, "memTotal": 8000, "memUsed": 3200, "memUsage": 40.0,
  "disks": [ { "mount": "/", "usage": 62.1, ... }, { "mount": "/data", "usage": 88.3, ... } ],
  "diskTotal": 100000, "diskUsed": 88300, "diskUsage": 88.3,   // = 关键磁盘（使用率最高）
  "netRx": 1234, "netTx": 567
}
```

- `diskTotal/diskUsed/diskUsage` 保留，取**使用率最高的真实磁盘**（伪文件系统若只有它就用它），旧界面不需要改也能继续显示。
- 实时监控事件 `monitor:data:<id>` 直接带上 `disks`，前端无需额外请求。

### 3.4 存储与迁移（`internal/store`）

```sql
-- 幂等增量迁移（先查 PRAGMA table_info，缺列才执行，失败不致命）
ALTER TABLE monitor_history ADD COLUMN disk_json TEXT;
CREATE INDEX IF NOT EXISTS idx_monitor_history_server_id ON monitor_history(server_id, id DESC);
```

- 写入：`saveMonitorPoint` 把 `disks` 序列化成 JSON 存进 `disk_json`。
- 读取：`GetMonitorHistory` 解析 `disk_json`；**NULL/解析失败的历史点，回退为一条"旧字段构成的根分区数据"**（`mount: "/"`，来自 `disk_total/disk_used/disk_usage`），历史曲线因此不会断裂。
- 旧库升级后老数据仍可读：`disk_json` 为空 → 走回退分支。

## 四、界面设计

### 4.1 总览（`views/dashboard/Index.vue`）

卡片内"磁盘"区块（卡片高度与磁盘数量无关）：

```
┌─────────────────────────────────────────────┐
│ ● 角落服务器                        [在线]   │
│ 192.168.106.202:22                          │
│ CPU    ▓▓▓▓▓▓▓░░░░░░░░░░░░░░  28.4%         │  ← 现有
│ 内存   ▓▓▓▓▓▓▓▓▓▓░░░░░░░░░░░  40.1%         │  ← 现有
│ 磁盘   /data  ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓░  88.3% ⚠     │  ← 关键磁盘（最高占用）
│ [▁▁▁▁▁▁▁] [▁▁▁▁▁▁▁▁▁▁▁] [▁▁▁▁]  3 个文件系统 │  ← 新增：全磁盘迷你条（悬停看详情）
│ 内存 3.1 GB / 7.8 GB                        │
└─────────────────────────────────────────────┘
```

- 关键磁盘那一行：左侧标签显示挂载点（`磁盘 /data`），值显示使用率 + 挂载点，超过 80% 变红并加 ⚠。
- 迷你条：每磁盘一段 4px 高的细条，颜色按占用绿→黄→红渐变，用原生 `title` 显示 `挂载点 · 62.1%（148 GB / 238 GB，ext4）`；**仅当磁盘数 ≥ 2 时才出现**，所以单磁盘服务器观感与现在完全一致。
- 磁盘数 ≥ 5 时，迷你条只显示前 4 个，末尾以 `+N` 收口，避免撑破卡片。
- 点击整张卡片仍跳转到实时监控。

### 4.2 实时监控（`views/servers/Monitor.vue`）

布局调整：CPU / 内存保持左右两栏，**"磁盘使用率"改为整行卡片**（内部左右分栏）：

```
┌──────────────────────────────────────────────────────────────────┐
│ 磁盘使用率                            [全部磁盘 ▾]  [峰值]        │
│ ┌────────────────────────────┬─────────────────────────────────┐ │
│ │  /        ▓▓▓▓▓▓░░░░ 62.1% │                                 │ │
│ │  /data    ▓▓▓▓▓▓▓▓▓▓ 88.3% │        折线图（每磁盘一条线）    │ │
│ │  /boot    ▓▓▓░░░░░░░ 31.0% │        80% 警戒虚线              │ │
│ │  ─────────────────────────  │                                 │ │
│ │  3 个文件系统 · 最大 88.3% │                                 │ │
│ └────────────────────────────┴─────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
```

- 左侧：每个挂载点一行（进度条 + 使用率 + 已用/总量），磁盘多时该列表自身滚动（最大 5 行高度）。
- 右侧：折线图，**一个磁盘一条线**，颜色取自主题的磁盘调色板；图例可点击隐藏/显示某条线；tooltip 按使用率降序排列，方便一眼看出谁最紧张。
- 警戒线：`markLine` 在 80% 画一条虚线（浅色），让"哪块盘快满了"一眼可见。
- 磁盘数 > 6 时，图默认只画使用率最高的 5 条 + 其余合并提示"另有 N 个文件系统未显示"；下拉可切到某个具体磁盘单独看。
- 历史数据：从 `disk_json` 还原每条曲线；旧数据只有单个数值时，作为 `/`（根分区）那一条线接上。

### 4.3 配色（`theme.js`）

新增 `diskPalette`（每个主题 6 色，与现有蓝/绿/黄/粉语言延续）：

| 主题 | 磁盘调色板 |
|---|---|
| light | `#C79E4F` `#5780A9` `#67947A` `#C58A98` `#9B7FB8` `#8F919B` |
| dark | `#D0A85C` `#6C9FD6` `#7FBA96` `#D294A3` `#A88FD0` `#8B8F9A` |
| cyber | `#FFB020` `#21E6FF` `#00FFA3` `#FF3EA5` `#A88FD0` `#7DEAFF` |

`/` 恒用调色板第 1 色（与"磁盘"语义色一致），其余按挂载点顺序取后续颜色，保证同一挂载点在所有页面/曲线里颜色稳定。

## 五、边界情况

| 场景 | 处理 |
|---|---|
| 只有 1 个磁盘 | 总览迷你条隐藏、监控页左侧只 1 行 —— 与现状视觉一致 |
| 服务器有 tmpfs/overlay | 默认过滤；监控页提供"显示全部文件系统（含 tmpfs）"开关，排查容器环境时可看 |
| `df -T` 不存在（BusyBox/BSD） | 回退 `df -Pk` + 设备名判定；再不行回退旧逻辑，只显示根分区并在卡片上标注"仅根分区" |
| 权限受限看不到某些挂载点 | `df` 通常会跳过；解析不到的行直接忽略，不影响其他磁盘 |
| 挂载点含空格 | 解析时"最后一个及之后的字段"整体作为挂载点，不按列号切 |
| 只读挂载（ro） | 单独标注 `只读`，使用率不做"快满"红色告警（写不进去不是容量问题） |
| 磁盘 20+ 的宿主机 | 一次 `df` 全量回传（体积很小），UI 侧截断 + 折叠，不截断数据 |
| 旧库历史数据 | 迁移加列 + 回退逻辑，历史曲线不断档 |
| 采集失败 | `disks` 为空数组，前端展示"暂无磁盘数据"，不报错、不显示 0% 假数据 |

## 六、实施步骤（评审通过后执行）

1. `models`：新增 `DiskInfo`，`MonitorPoint` 加 `DiskJSON`。
2. `sshx`：实现 `parseDf`（含 3 级回退与伪文件系统过滤）+ 单测（样例输出覆盖 GNU/BSD/换行/空格/`-`/表头）。
3. `monitor.go`：`GetSystemStats` 装配 `disks` + 关键磁盘旧字段；`saveMonitorPoint` 存 JSON。
4. `store`：迁移加列 + 索引；读时解析与回退；单测覆盖"旧行回退"。
5. `theme.js`：新增 `diskPalette`。
6. `Monitor.vue`：磁盘卡片改为整行 + 左侧列表 + 多线折线图 + 显示全部开关。
7. `Index.vue`（总览）：关键磁盘行 + 迷你条（原生 title）。
8. 验证：`go test ./...`、`go vet`、`npm run build`、`wails build`；用真实服务器（角落服务器 192.168.106.202 若有第二块盘）核对 `df -PTk` 与界面数值一致。

## 七、实施记录（已完成）

| 层 | 文件 | 内容 |
|---|---|---|
| 模型 | `internal/models/models.go` | 新增 `DiskInfo`；`MonitorPoint` 增 `DiskJSON`（仅落库）与 `Disks`（随桥接返回） |
| 采集 | `internal/sshx/sshx.go` | 新增 `GetDisks` / `parseDf` / `dfUnitDivisor` / `isPseudoDisk` / `parseReadOnlyMounts` / `sortDisks` / `pickKeyDisk`；`GetSystemStats` 装配 `disks` 与关键磁盘旧字段；移除旧的单盘 `parseDisk` |
| 落库 | `monitor.go`、`internal/store/store.go` | `disksJSON()` 序列化快照；`migrate()` 幂等补 `disk_json` 列与索引；`disksFromJSON()` 旧数据回退为根分区一条 |
| 配色 | `frontend/src/theme.js` | `getDiskPalette` / `diskColorOf`（按挂载点稳定配色）/ `usageColor`（绿→黄→红语义色） |
| 监控页 | `frontend/src/views/servers/Monitor.vue` | 磁盘改整行卡片：左列表（含只读标记，超长自身滚动）+ 右多线折线图（图例可点、tooltip 降序、80% 警戒虚线）+ 聚焦下拉 + 显示全部开关 |
| 总览 | `frontend/src/views/dashboard/Index.vue` | 关键磁盘行（标签带挂载点，≥80% 变红加 ⚠）+ 迷你条（真实盘 ≥2 才出现，最多 4 段 + `+N`，原生 `title`） |
| 单测 | `internal/sshx/sshx_test.go`、`internal/store/store_test.go` | 13 个新用例：df 三种布局、设备名换行、含空格挂载点、容量 `-`、单位推断、伪盘判定、关键盘选取、只读解析与转义、分段拆分、排序；老库迁移、往返读写、损坏快照回退、重复迁移幂等 |

与设计稿的三处偏差（实现时按"更稳/更省"取舍）：

1. **快照字段拆成两个**：`DiskJSON` 标 `json:"-"` 只用于落库，桥接侧只传已解析的 `Disks`，避免 60 个历史点各带一份双重编码的 JSON 字符串。
2. **df 与 /proc/mounts 合并成一条命令**：用 `<<<SM2-DF>>>` / `<<<SM2-MOUNTS>>>` 标记分段，每轮采集只多一次 SSH 往返（原设计是两条命令）。
3. **无 `-T` 时的伪盘判定改为"宁多勿漏"**：只按明确的伪文件系统名（tmpfs/devtmpfs/overlay…）过滤，认不出的设备名（如 ZFS 的 `pool/dataset`）一律当真实盘保留，避免误藏真实存储。

验证：`go test ./...`（20 个用例全过，含新增 13 个）、`go vet ./...`、`go build ./...`、`npm run build`、`wails build` 全部通过，产物 `prj/build/bin/prj.exe` 已更新。

待真机核对：解析逻辑以单测样例（GNU / BSD / 无 `-T` 三种布局）为准，尚未连真实服务器比对界面数值；建议在有第二块盘的机器上确认卡片迷你条与监控页多曲线是否符合预期。