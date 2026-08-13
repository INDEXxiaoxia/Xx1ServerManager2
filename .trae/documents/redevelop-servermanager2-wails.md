# ServerManager2 重构开发计划（Wails 2 + Vue 3 + Go）

## 1. 摘要

在 `f:\forProgram\INDEXxiaoxia\GUI\Xx1ServerManager2\prj` 目录下，基于 Wails 2 + Vue 3 + Go 重构 Xx1ServerManager，完整复刻原有功能，并新增服务器状态实时监控、文件管理、服务管理、实时 SSH 连接四大能力。

- **技术栈**：Wails 2.11（已装）+ Go 1.24（已装）+ Vue 3 + Vite + Element Plus + Pinia + Vue Router
- **数据存储**：SQLite（`modernc.org/sqlite` 纯 Go 驱动）
- **SSH 库**：`golang.org/x/crypto/ssh`、`github.com/pkg/sftp`
- **前端增强**：ECharts（监控图表）、xterm.js（SSH 终端）、Element Plus（UI）
- **实时通信**：Wails 内置 Event 机制（EventEmit / EventsOn）实现监控与终端数据流

---

## 2. 现状分析

### 2.1 原项目（Xx1ServerManager）功能清单
来源：`f:\forProgram\2025\Xx1ServerManager`，技术栈为 Electron + Vue 3 + Python FastAPI + Paramiko。

| 模块 | 现有功能 | 参考文件 |
|------|---------|---------|
| 服务器管理 | 增删改查、分组关联、测试连接、状态 | `backend/app/api/servers.py`、`schemas.py` |
| 分组管理 | 增删改 | `backend/app/api/groups.py` |
| 预设管理 | 命令/路径预设 | `backend/app/api/presets.py` |
| 批量上传 | SFTP 上传、并发控制（5）、WebSocket 进度 | `backend/app/services/task_manager.py`、`ssh_client.py` |
| 批量命令 | 并发执行、实时输出、超时 | 同上 |
| 主密码 | PBKDF2 派生 + AES-256-CBC 加密密码存储 | `backend/app/utils/crypto.py` |
| 配置存储 | JSON 配置文件（用户主目录） | `backend/app/config/manager.py` |
| 监控（雏形） | CPU/内存/磁盘/进程（单次获取） | `ssh_client.py#get_system_stats` |

### 2.2 前端页面（现有）
`frontend/src/views/`：`Layout.vue`、`servers/List.vue`、`files/Upload.vue`、`commands/Execute.vue`、`groups/Index.vue`、`presets/Index.vue`。

### 2.3 环境
- Go 1.24.7、Wails v2.11.0 已安装，可直接 `wails init` / `wails dev` / `wails build`。
- `prj` 目录尚不存在，需创建。

---

## 3. 目标架构

### 3.1 目录结构（在 `prj/` 下）

```
prj/
├── main.go                 # Wails 入口，绑定 App 服务方法
├── wails.json              # Wails 配置
├── go.mod / go.sum
├── app/                    # 被 Wails 绑定的服务层（App 结构体方法）
│   ├── app.go              # App 结构体，聚合各服务
│   ├── server.go           # 服务器 CRUD + 测试连接
│   ├── group.go            # 分组 CRUD
│   ├── preset.go           # 预设命令/路径
│   ├── upload.go           # 批量上传
│   ├── command.go          # 批量命令执行
│   ├── filemanager.go      # 文件管理（浏览/上传/下载/删除/改名/权限）
│   ├── monitor.go          # 实时监控轮询
│   ├── service.go          # systemd 服务管理
│   ├── terminal.go         # 交互式 SSH 终端
│   ├── auth.go             # 主密码管理
│   └── settings.go         # 应用设置
├── internal/
│   ├── models/             # 数据模型（Server/Group/Preset/Task 等）
│   ├── store/              # SQLite 数据访问层
│   ├── crypto/             # 主密码派生 + AES-256-CBC 加解密
│   ├── sshx/               # SSH 客户端封装（连接/命令/SFTP/交互 shell）
│   ├── tasks/              # 批量任务调度（并发控制）
│   └── util/               # 通用工具
└── frontend/               # Vue 3 + Element Plus 前端（wails init 生成）
    └── src/
        ├── main.js / App.vue
        ├── router/index.js
        ├── store/           # Pinia：server/group/monitor/terminal
        ├── api/             # 封装 window.go 调用 + Wails Events
        ├── composables/     # useMonitor / useTerminal / useUpload
        ├── components/      # 通用组件
        └── views/
            ├── Layout.vue
            ├── dashboard/Index.vue      # 总览仪表盘
            ├── servers/List.vue         # 服务器列表
            ├── servers/Detail.vue       # 服务器详情（含监控）
            ├── servers/Monitor.vue      # 实时监控图表
            ├── files/Manager.vue        # 文件管理
            ├── files/Upload.vue         # 批量上传
            ├── commands/Execute.vue     # 批量执行
            ├── services/Index.vue       # 服务管理
            ├── terminal/Index.vue       # SSH 终端（多标签）
            ├── groups/Index.vue         # 分组管理
            ├── presets/Index.vue        # 预设管理
            └── settings/Index.vue       # 设置（主密码/主题/并发数）
```

### 3.2 数据模型（SQLite）

表：`servers`、`groups`、`server_groups`（多对多）、`preset_commands`、`preset_paths`、`monitor_history`（监控历史）、`command_history`、`app_settings`。

- `servers`：id、name、host、port、username、password(密文)、status、created_at、updated_at
- `monitor_history`：id、server_id、cpu_usage、mem_used、mem_total、disk_used、disk_total、net_rx、net_tx、timestamp（用于图表回放）
- 密码加密：保留原 AES-256-CBC + PBKDF2 方案，用 Go 重写（`crypto/aes` + `golang.org/x/crypto/pbkdf2`）

---

## 4. 实施步骤

### 阶段 0：初始化 Wails 项目
1. 在 `prj/` 下执行 `wails init -n ServerManager2 -t vue`（生成 Vue 前端骨架 + Go 主程序）。
2. 配置 `wails.json`（应用名、图标、单一二进制目标）。
3. 安装前端依赖：`element-plus`、`@element-plus/icons-vue`、`pinia`、`vue-router`、`echarts`、`xterm`、`xterm-addon-fit`、`axios`（可选）。
4. 安装 Go 依赖：`golang.org/x/crypto/ssh`、`github.com/pkg/sftp`、`modernc.org/sqlite`、`github.com/google/uuid`。

### 阶段 1：基础层
1. `internal/models`：定义全部数据模型。
2. `internal/store`：SQLite 初始化 + 各表 DAO（CRUD）。
3. `internal/crypto`：主密码设置/验证、PBKDF2 派生、AES-256-CBC 加解密（对齐原算法，可迁移旧数据）。
4. `internal/sshx`：SSH 客户端封装
   - `Connect` / `TestConnection` / `ExecCommand`（含 sudo 支持）
   - `SFTP` 操作（上传/下载带进度回调、List、Mkdir、Rename、Remove、Chmod）
   - `InteractiveShell`（Channel 交互式 shell，供终端使用）
5. `internal/tasks`：批量任务调度器（并发信号量控制，默认 5），支持上传/命令任务进度回调。

### 阶段 2：服务层（`app/`，Wails 绑定方法）
1. `app/server.go`：服务器 CRUD、测试连接、设置状态。
2. `app/group.go`、`app/preset.go`：分组与预设 CRUD。
3. `app/auth.go`、`app/settings.go`：主密码、主题/并发设置。
4. `app/upload.go`、`app/command.go`：对接 tasks 调度器，通过 Wails `EventEmit` 推送进度/输出。
5. `app/filemanager.go`：SFTP 文件树浏览、上传/下载（带进度 EventEmit）、删除、重命名、权限修改。
6. `app/monitor.go`：定时轮询（SSH 采集 CPU/内存/磁盘/网络），写入 `monitor_history`，通过 `EventEmit` 推送实时数据。
7. `app/service.go`：封装 `systemctl` 命令（list/start/stop/restart/enable/disable/status）。
8. `app/terminal.go`：为每个终端标签维护 SSH 交互会话，用 `EventEmit` 双向转发输入/输出。

### 阶段 3：前端
1. 搭建 `Layout.vue` 侧边栏导航（Dashboard、服务器、监控、文件、批量上传、批量执行、服务管理、终端、分组、预设、设置）。
2. 服务器列表、详情、新增/编辑对话框（含测试连接）。
3. 批量上传页（多服务器选择、拖拽文件、进度条）。
4. 批量执行页（命令输入、多服务器输出面板、实时流向）。
5. 文件管理页（双栏：服务器树 + 本地文件，或远程目录树 + 文件列表，支持操作）。
6. 监控页（ECharts 折线图：CPU/内存/磁盘/网络，实时刷新）。
7. 服务管理页（服务表格 + 启动/停止/重启/自启按钮）。
8. SSH 终端页（多标签 xterm.js，输入输出通过 Wails Events 双向流）。
9. 分组、预设、设置页。
10. 主密码引导对话框（首次启动设置）。

### 阶段 4：联调与验证
1. `wails dev` 启动开发模式，逐页验证功能。
2. 验证 SSH 连接、批量上传/命令、SFTP 文件操作、实时监控、systemd 服务、交互式终端。
3. 验证主密码加密存取与配置持久化。
4. `wails build` 产出单一 exe，验证打包。

> 注：真实远端 Linux 服务器可用本地容器/虚拟机，或至少用 `TestConnection` + 手动输入进行验证。

---

## 5. 关键设计决策

| 决策点 | 选择 | 理由 |
|--------|------|------|
| UI 组件库 | Element Plus | 与现有项目一致，迁移省力，AI 生成代码质量高（用户已确认） |
| 数据存储 | SQLite（modernc.org/sqlite） | 纯 Go 无 CGO 依赖，适合监控历史/日志查询（用户已确认） |
| 实时通信 | Wails Event 机制 | 原生内建，替代 WebSocket，流式传输监控/终端数据 |
| 终端渲染 | xterm.js + xterm-addon-fit | 行业标准，ANSI/VT100 支持完善 |
| 监控图表 | ECharts | 折线图/仪表盘成熟，实时刷新性能好 |
| 密码加密 | 复用原 AES-256-CBC + PBKDF2 | 兼容旧数据，保证安全 |
| 并发控制 | Go channel 信号量（默认 5） | 对齐原项目并发数，避免过载 |
| sudo 支持 | SSH exec 包装 `echo pass \| sudo -S` | 沿用原项目逻辑，适配 rootless 场景 |

---

## 6. 假设与约定

- 密码加密存储须通过主密码派生密钥，主密码未设置时服务器密码无法持久化解密（与原文一致）。
- 监控轮询默认间隔：CPU/内存 5s、磁盘 60s、网络 10s（可在设置中调整）。
- 交互式终端与 SSII 会话一一对应，关闭标签释放连接。
- 单二进制分发，图标与资源内嵌于 Wails 构建产物。
- 远端服务器为 Linux（SSH/SFTP/systemd），不支持 Windows 服务器的本地管理。

---

## 7. 验证步骤

1. `cd prj && wails dev` 启动应用，确认 UI 正常渲染、无前端报错。
2. 添加/编辑/删除服务器与分组、预设，重启应用后数据仍在（SQLite 持久化）。
3. 对可达服务器执行测试连接、批量上传、批量命令，观察实时进度/输出。
4. 打开监控页，确认 5s 内图表刷新并有历史曲线。
5. 打开服务管理页，对 systemd 服务执行 start/stop/restart/status。
6. 打开终端页，多标签分别连接不同服务器并执行交互命令。
7. 设置主密码后重启，确认密码加密存储与解锁流程正常。
8. `wails build` 生成 exe，双击运行验证打包产物。

---

## 8. 交付物

- `prj/` 下完整可运行的 Wails + Vue 3 + Go 项目。
- 覆盖原功能 + 新增四大能力的全部页面与后端服务。
- 单一 exe 构建产物。