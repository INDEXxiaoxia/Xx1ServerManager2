package main

import (
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ServerManager2/internal/models"
	"ServerManager2/internal/sshx"
)

func (a *App) dialServer(serverID string) (*models.Server, *sshx.Client, error) {
	sv, err := a.getServerWithPassword(serverID)
	if err != nil {
		return nil, nil, err
	}
	c := sshx.New(sv.Host, sv.Port, sv.Username, sv.Password, 10)
	if err := c.Connect(); err != nil {
		return sv, nil, err
	}
	return sv, c, nil
}

// SelectLocalFile 打开本地文件选择对话框，返回所选路径
func (a *App) SelectLocalFile() (string, error) {
	opts := runtime.OpenDialogOptions{Title: "选择本地文件"}
	return runtime.OpenFileDialog(a.ctx, opts)
}

// SelectLocalSavePath 打开本地保存对话框，返回保存路径
func (a *App) SelectLocalSavePath(defaultName string) (string, error) {
	opts := runtime.SaveDialogOptions{Title: "保存到本地", DefaultFilename: defaultName}
	return runtime.SaveFileDialog(a.ctx, opts)
}

// ListRemoteDir 列出远程目录
func (a *App) ListRemoteDir(serverID, path string) ([]models.FileEntry, error) {
	_, c, err := a.dialServer(serverID)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return c.ListDir(path)
}

// MakeRemoteDir 创建远程目录
func (a *App) MakeRemoteDir(serverID, path string) error {
	_, c, err := a.dialServer(serverID)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.MakeDirs(path)
}

// RenameRemote 重命名/移动远程文件
func (a *App) RenameRemote(serverID, oldPath, newPath string) error {
	_, c, err := a.dialServer(serverID)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Rename(oldPath, newPath)
}

// DeleteRemote 删除远程文件或目录
func (a *App) DeleteRemote(serverID, path string, isDir bool) error {
	_, c, err := a.dialServer(serverID)
	if err != nil {
		return err
	}
	defer c.Close()
	if isDir {
		return c.RemoveDir(path)
	}
	return c.Remove(path)
}

// ChmodRemote 修改远程文件权限
func (a *App) ChmodRemote(serverID, path string, mode uint32) error {
	_, c, err := a.dialServer(serverID)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Chmod(path, mode)
}

// UploadToServer 上传单个文件到服务器（带进度事件）
func (a *App) UploadToServer(serverID, localPath, remotePath string) error {
	_, c, err := a.dialServer(serverID)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Upload(localPath, remotePath, func(transferred, total int64) {
		pct := 0.0
		if total > 0 {
			pct = float64(int(transferred*10000/total)) / 100
		}
		runtime.EventsEmit(a.ctx, "file:progress", map[string]any{
			"serverId": serverID, "localPath": localPath, "remotePath": remotePath,
			"direction": "upload", "progress": pct, "transferred": transferred, "total": total,
		})
	})
}

// DownloadFromServer 从服务器下载单个文件（带进度事件）
func (a *App) DownloadFromServer(serverID, remotePath, localPath string) error {
	_, c, err := a.dialServer(serverID)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Download(remotePath, localPath, func(transferred, total int64) {
		pct := 0.0
		if total > 0 {
			pct = float64(int(transferred*10000/total)) / 100
		}
		runtime.EventsEmit(a.ctx, "file:progress", map[string]any{
			"serverId": serverID, "localPath": localPath, "remotePath": remotePath,
			"direction": "download", "progress": pct, "transferred": transferred, "total": total,
		})
	})
}

// GetRemoteFileInfo 获取远程文件信息（用于下载默认名）
func (a *App) GetRemoteFileInfo(serverID, remotePath string) (map[string]any, error) {
	_, c, err := a.dialServer(serverID)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	entries, err := c.ListDir(filepath.Dir(remotePath))
	if err != nil {
		return nil, err
	}
	name := filepath.Base(remotePath)
	for _, e := range entries {
		if e.Name == name {
			return map[string]any{"name": e.Name, "size": e.Size, "modTime": e.ModTime}, nil
		}
	}
	return map[string]any{"name": name, "size": int64(0), "modTime": ""}, nil
}
