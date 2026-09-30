# Xx1ServerManager2

基于原有 Xx1ServerManager 功能重构的 Windows 桌面端服务器管理工具。

## 技术栈

| 层 | 技术 |
|---|---|
| 桌面框架 | [Wails 2](https://wails.io/)（v2.11） |
| 后端 | Go 1.24（`internal/` 分层：models / store / crypto / sshx / tasks） |
| 前端 | Vue 3 + Vite 5 + Vue Router + Pinia |
| UI 组件 | Element Plus（zh-cn）、ECharts（监控图表）、xterm.js（SSH 终端） |
| 数据存储 | SQLite（[modernc.org/sqlite](https://modernc.org/sqlite)，纯 Go 无 CGO） |
| SSH 能力 | [golang.org/x/crypto/ssh](https://pkg.go.dev/golang.org/x/crypto/ssh) + [github.com/pkg/sftp](https://github.com/pkg/sftp) |
| 密码安全 | PBKDF2-HMAC-SHA256（100k 次）派生密钥 + AES-256-CBC + PKCS7，加密存储服务器密码 |

## 主要功能

- **服务器管理**：增删改查、测试连接；分组**单属**（一个服务器只属于一个分组，设置页/表单里单选）
- **总览**：模糊搜索（名称/主机）+ 分组下拉筛选，卡片按分组分区展示（有服务器的分组在前、未分组最后）；
  在线/离线按监控数据实时推导——取到数据即在线，连接或取数失败才是离线
- **实时监控**：CPU / 内存 / 磁盘 / 网络 的实时曲线与历史趋势（ECharts，事件流推送）
- **文件管理**：远程目录浏览、上传 / 下载（含进度）、新建 / 重命名 / 删除 / 改权限；目标目录无写权限时自动回退 sudo 上传
- **批量上传**：左侧多选服务器，支持预设路径，并发执行并回传进度
- **批量执行**：左侧多选服务器执行命令，实时回显输出与状态码，含历史记录
- **分组管理**：批量勾选把已有服务器归入某分组（勾选即归入，取消勾选即移出为未分组，后写覆盖先写）；
  删除分组会把成员释放为未分组，不留悬空分组 id
- **服务管理**：查看服务列表 / 状态 / 开机自启，启动、停止、重启 —— 页面暂未定型，**侧栏菜单入口已隐藏**（路由保留）
- **实时 SSH 终端**：xterm.js 交互式 shell，支持窗口缩放自适应
- **预设管理**：预设命令（可分类）、预设路径，供批量操作快速选用
- **安全**：主密码解锁才可查看服务器凭据；密码密文入库；右上角**一键锁定**（清内存密钥并关闭监控/终端/文件会话）。
  未设置主密码时密码仍以本机密钥加密持久保存，重启后照常可用，不会丢失

## 目录结构（`prj/` 为项目根）

```
prj/
├── main.go                     # Wails 入口（窗口标题 Xx1ServerManager2）
├── app.go                      # App 结构：事件、监控/终端会话管理
├── server.go / group.go / auth.go
├── upload.go / command.go / filemanager.go
├── monitor.go / service.go / terminal.go   # 各功能 Wails 绑定方法
├── internal/
│   ├── models/   # 数据模型
│   ├── store/    # SQLite 数据访问
│   ├── crypto/   # 主密码 + 加解密
│   ├── sshx/     # SSH/SFTP/交互 shell/系统状态/服务
│   └── tasks/    # 批量任务并发
└── frontend/
    ├── src/views/   # 功能页面（服务管理页已从菜单隐藏，路由保留）
    ├── src/api/     # Wails 绑定封装
    ├── src/utils/   # 分组单属规则与统一排序（groups.js）
    └── src/store/   # Pinia
```

## 环境要求

- Windows 10 / 11（需 Microsoft Edge WebView2 运行时，一般系统已自带）
- Go 1.24+
- Node.js 18+（npm）
- [Wails CLI](https://wails.io/docs/gettingstarted/installation) v2

## 启动 / 调试 / 打包

```bash
# 开发调试（热重载前端，窗口内 F12 可开 DevTools）
cd prj
wails dev

# 仅构建前端（可选，wails 命令会自动完成）
cd prj/frontend
npm install
npm run build

# 生产打包
cd prj
wails build

# 产物
prj/build/bin/prj.exe
```

## 数据存储

- SQLite 数据库：`%USERPROFILE%\ServerManager2\data.db`
- 服务器密码以 AES 密文存储：设置主密码后用主密码派生密钥，未设置主密码时用本机种子派生密钥
  （重置主密码会用旧密钥解出、新密钥重新加密，密码不会丢）