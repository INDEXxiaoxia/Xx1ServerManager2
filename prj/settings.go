package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// GetConfigDirInfo 返回配置文件（数据库）所在目录信息，供设置页展示与打开
func (a *App) GetConfigDirInfo() (map[string]any, error) {
	if a.store == nil {
		return nil, fmt.Errorf("存储尚未初始化")
	}
	dir := a.store.ConfigDir()
	info := map[string]any{
		"dir":    dir,
		"dbPath": a.store.DBPath(),
		"exists": false,
	}
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		info["exists"] = true
	}
	return info, nil
}

// OpenConfigDir 在系统文件管理器中打开配置文件目录
func (a *App) OpenConfigDir() (string, error) {
	if a.store == nil {
		return "", fmt.Errorf("存储尚未初始化")
	}
	dir := a.store.ConfigDir()
	if dir == "" {
		return "", fmt.Errorf("未获取到配置目录路径")
	}
	if st, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("配置目录不存在：%s", dir)
	} else if !st.IsDir() {
		return "", fmt.Errorf("配置路径不是目录：%s", dir)
	}
	if err := openInFileManager(dir); err != nil {
		return dir, err
	}
	return dir, nil
}

// openInFileManager 按平台拉起系统文件管理器
func openInFileManager(dir string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// 用 explorer 打开目录；不用 Wails 的 BrowserOpenURL，
		// 避免自定义 scheme 下被当成浏览器打开而触发窗口关闭
		cmd = exec.Command("explorer", filepath.Clean(dir))
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		// Linux 上按可用性依次尝试
		if path, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command(path, dir)
		} else if path, err := exec.LookPath("gio"); err == nil {
			cmd = exec.Command(path, "open", dir)
		} else {
			return fmt.Errorf("未找到可用的文件管理器命令（xdg-open / gio）")
		}
	}
	// explorer 成功时也可能返回非 0 退出码，这里不校验退出码，只保证能拉起
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开目录失败：%v", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}