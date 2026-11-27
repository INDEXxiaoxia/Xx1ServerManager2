package main

import (
	"context"
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()
	geometry := loadWindowGeometry()

	// Create application with options
	err := wails.Run(&options.App{
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true},
		Title:       "Xx1ServerManager2",
		// 默认窗口：终端 + 右侧面板并排时 1440×950 比较舒展
		Width:  geometry.Width,
		Height: geometry.Height,
		// 低于 1100×700 时右侧面板与终端都要开始挤，直接给个下限
		MinWidth:  760,
		MinHeight: 480,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		WindowStartState: geometry.startState(),
		OnBeforeClose:    geometry.saveBeforeClose,
		OnStartup:        app.startup,
		OnShutdown: func(ctx context.Context) {
			// 释放文件管理复用的 SFTP 连接，避免退出时残留会话
			app.CloseFileClients()
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
