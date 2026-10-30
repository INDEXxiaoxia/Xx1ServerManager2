# Xx1ServerManager2

Windows 桌面端 Linux 服务器管理工具。基于 [Wails 2](https://wails.io/) + Go + Vue 3，把 SSH 终端、文件管理、实时监控和批量运维收进同一个本地应用。

> **安全提示**：服务器密码、主密码、AI API Key 等只保存在本机，不会上传到任何云端。请勿把 `%USERPROFILE%\ServerManager2\` 目录或含密钥的配置提交到公开仓库。

## 下载

到 [Releases](../../releases) 下载最新安装包：

| 文件 | 说明 |
|---|---|
| `ServerManager2-amd64-installer_2.0.exe` | NSIS 安装包（推荐） |
| `ServerManager2_2.0.exe` | 绿色免安装可执行文件 |

系统要求：Windows 10 / 11，已安装 [WebView2](https://developer.microsoft.com/microsoft-edge/webview2/)（多数系统自带）。

## 主要功能

- **服务器 / 分组**：增删改查、测试连接；一台服务器只属于一个分组
- **总览卡片**：按名称 / 主机搜索，按分组筛选；在线状态跟随监控实时更新
- **实时监控**：CPU、内存、磁盘、网络曲线与历史趋势
- **文件管理**：浏览、上传 / 下载（含进度）、新建 / 重命名 / 删除 / 改权限；无写权限时自动回退 sudo 上传
- **批量运维**：多选服务器批量上传或执行命令，实时回显
- **SSH 终端**：xterm.js 交互式终端，支持多标签、同步输入、快捷命令面板
- **终端 AI（实验）**：建议命令 / 直接执行 / 完全接管；需自行配置 OpenAI 兼容接口
- **安全**：主密码锁定、凭据 AES 加密入库、一键锁定清空内存会话

## 技术栈

| 层 | 技术 |
|---|---|
| 桌面框架 | Wails 2 |
| 后端 | Go（`internal/`：models / store / crypto / sshx / tasks） |
| 前端 | Vue 3 + Vite + Vue Router + Pinia + Element Plus |
| 终端 / 图表 | xterm.js / ECharts |
| 存储 | SQLite（`modernc.org/sqlite`，纯 Go） |
| SSH | `golang.org/x/crypto/ssh` + `github.com/pkg/sftp` |
| 密码 | PBKDF2-HMAC-SHA256 + AES-256-CBC |

## 目录结构

```
prj/                         # Wails 工程根目录
├── main.go / app.go         # 入口与会话管理
├── *.go                     # 各功能绑定方法
├── internal/
│   ├── models/ store/ crypto/ sshx/ tasks/
└── frontend/                # Vue 前端
build.bat / build_installer.bat
```

## 本地开发

```bash
# 依赖：Go 1.24+、Node.js 18+、Wails CLI v2
cd prj
wails doctor          # 检查环境
wails dev             # 热重载开发（窗口内 F12 开 DevTools）

# 生产构建
wails build
# 产物：prj/build/bin/
```

安装包可参考根目录 `build_installer.bat`（NSIS）。

## 数据存储（本机）

- 数据库：`%USERPROFILE%\ServerManager2\data.db`
- 服务器密码以 AES 密文保存；设置主密码后用主密码派生密钥，未设置时用本机种子派生
- AI 接口地址、API Key、模型名保存在本地设置中，**不会随本仓库分发**

## 许可

[MIT](./LICENSE)
