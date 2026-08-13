package main

import (
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ServerManager2/internal/sshx"
	"ServerManager2/internal/tasks"
)

// uploadTask 单个上传任务
type uploadTask struct {
	ServerID   string
	ServerName string
	LocalPath  string
	RemotePath string
}

func (a *App) emitUpload(sessionID string, t uploadTask, status string, progress float64, transferred, total int64, errMsg string) {
	runtime.EventsEmit(a.ctx, "upload:progress", map[string]any{
		"sessionId":   sessionID,
		"serverId":    t.ServerID,
		"serverName":  t.ServerName,
		"localPath":   t.LocalPath,
		"remotePath":  t.RemotePath,
		"status":      status,
		"progress":    progress,
		"transferred": transferred,
		"total":       total,
		"errorMsg":    errMsg,
		"taskId":      t.ServerID + "|" + t.LocalPath,
	})
}

// StartBatchUpload 批量上传文件
func (a *App) StartBatchUpload(serverIDs []string, localPaths []string, remoteDir string) (string, error) {
	sessionID := uuid.NewString()

	go func() {
		settings, _ := a.store.GetSettings()
		var tasksList []uploadTask
		for _, sid := range serverIDs {
			sv, err := a.getServerWithPassword(sid)
			if err != nil {
				continue
			}
			for _, lp := range localPaths {
				name := filepath.Base(lp)
				remotePath := strings.TrimSuffix(remoteDir, "/") + "/" + name
				tasksList = append(tasksList, uploadTask{
					ServerID: sid, ServerName: sv.Name, LocalPath: lp, RemotePath: remotePath,
				})
			}
		}
		tasks.RunConcurrent(tasksList, settings.MaxConcurrent, func(t uploadTask) {
			a.doUpload(sessionID, t)
		})
		runtime.EventsEmit(a.ctx, "upload:completed", sessionID)
	}()

	return sessionID, nil
}

func (a *App) doUpload(sessionID string, t uploadTask) {
	a.emitUpload(sessionID, t, "running", 0, 0, 0, "")
	sv, err := a.getServerWithPassword(t.ServerID)
	if err != nil {
		a.emitUpload(sessionID, t, "failed", 0, 0, 0, err.Error())
		return
	}
	c := sshx.New(sv.Host, sv.Port, sv.Username, sv.Password, 10)
	if err := c.Connect(); err != nil {
		a.emitUpload(sessionID, t, "failed", 0, 0, 0, err.Error())
		return
	}
	defer c.Close()

	var total int64
	err = c.Upload(t.LocalPath, t.RemotePath, func(transferred, tot int64) {
		total = tot
		pct := 0.0
		if tot > 0 {
			pct = float64(int(transferred*10000/tot)) / 100
		}
		a.emitUpload(sessionID, t, "running", pct, transferred, tot, "")
	})
	if err != nil {
		a.emitUpload(sessionID, t, "failed", 0, 0, total, err.Error())
		return
	}
	a.emitUpload(sessionID, t, "success", 100, total, total, "")
}
