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

	terminalMu sync.Mutex
	terminals  map[string]*TerminalSession

	monitorMu sync.Mutex
	monitors  map[string]context.CancelFunc
}

// NewApp 创建应用实例
func NewApp() *App {
	return &App{
		terminals: make(map[string]*TerminalSession),
		monitors:  make(map[string]context.CancelFunc),
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
}

// Greet 保留的示例方法
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}