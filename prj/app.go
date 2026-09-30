package main

import (
	"context"
	"fmt"
	"sync"

	"ServerManager2/internal/crypto"
	"ServerManager2/internal/store"
)

// App 应用主结构，所有导出方法将被 Wails 绑定到前端
type App struct {
	ctx    context.Context
	store  *store.Store
	crypto *crypto.CryptoManager

	// locked 是否处于锁定态（右上角「锁定」按钮设置）。
	// 不能只看 crypto 有没有密钥：未设置主密码时启动就会预置本机密钥。
	locked bool

	terminalMu sync.Mutex
	terminals  map[string]*TerminalSession

	monitorMu sync.Mutex
	monitors  map[string]context.CancelFunc

	// 文件管理复用连接：目录浏览/大小计算/搜索都会产生大量 SFTP 请求，
	// 每次操作新建 SSH 连接会显著拖慢（计算目录大小可能需要上百次 stat）。
	fileMu    sync.Mutex
	fileConns map[string]*fileSession

	// 运维工具的磁盘检测任务：一次检测一条记录，取消时靠它掐断正在跑的 du
	opsMu     sync.Mutex
	diskTasks map[string]*diskTask
}

// NewApp 创建应用实例
func NewApp() *App {
	return &App{
		terminals: make(map[string]*TerminalSession),
		monitors:  make(map[string]context.CancelFunc),
		fileConns: make(map[string]*fileSession),
		diskTasks: make(map[string]*diskTask),
	}
}

// startup 应用启动时初始化
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	st, err := store.NewStore("")
	if err != nil {
		fmt.Println("初始化数据库失败:", err)
		return
	}
	a.store = st
	a.crypto = crypto.New()

	// 预置加密密钥：
	// - 已设置主密码：进入锁定态，等用户输入主密码解锁；
	// - 未设置主密码：用本机种子派生密钥，保证服务器密码能加密保存且重启后可读
	//   （这正是"没设主密码也不要把密码直接删掉"的要求）。
	if set, _ := a.IsMasterPasswordSet(); set {
		a.locked = true
	} else if err := a.ensureMachineKey(); err != nil {
		fmt.Println("初始化本机密钥失败:", err)
	}

	// 监控历史只保留最近 1 小时，启动时清一次并进入定时清理
	a.startHistoryPruner()
}

// Greet 保留的示例方法
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}
