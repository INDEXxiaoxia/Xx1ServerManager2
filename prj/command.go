package main

import (
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ServerManager2/internal/models"
	"ServerManager2/internal/sshx"
	"ServerManager2/internal/tasks"
)

type commandTask struct {
	ServerID   string
	ServerName string
}

func (a *App) emitCommand(sessionID string, t commandTask, command string, status string, exitCode int, stdout, stderr string, duration int64, errMsg string) {
	runtime.EventsEmit(a.ctx, "command:output", map[string]any{
		"sessionId":  sessionID,
		"serverId":   t.ServerID,
		"serverName": t.ServerName,
		"command":    command,
		"status":     status,
		"exitCode":   exitCode,
		"stdout":     stdout,
		"stderr":     stderr,
		"duration":   duration,
		"errorMsg":   errMsg,
		"taskId":     t.ServerID,
	})
}

// StartBatchCommand 批量执行命令
func (a *App) StartBatchCommand(serverIDs []string, command string, timeout int) (string, error) {
	sessionID := uuid.NewString()
	if timeout <= 0 {
		timeout = 30
	}

	go func() {
		settings, _ := a.store.GetSettings()
		var tasksList []commandTask
		for _, sid := range serverIDs {
			sv, err := a.getServerWithPassword(sid)
			if err != nil {
				continue
			}
			tasksList = append(tasksList, commandTask{ServerID: sid, ServerName: sv.Name})
		}
		tasks.RunConcurrent(tasksList, settings.MaxConcurrent, func(t commandTask) {
			a.doCommand(sessionID, t, command, timeout)
		})
		runtime.EventsEmit(a.ctx, "command:completed", sessionID)
	}()

	return sessionID, nil
}

func (a *App) doCommand(sessionID string, t commandTask, command string, timeout int) {
	a.emitCommand(sessionID, t, command, "running", 0, "正在执行...", "", 0, "")
	sv, err := a.getServerWithPassword(t.ServerID)
	if err != nil {
		a.emitCommand(sessionID, t, command, "failed", -1, "", "", 0, err.Error())
		return
	}
	c := sshx.New(sv.Host, sv.Port, sv.Username, sv.Password, 10)
	if err := c.Connect(); err != nil {
		a.emitCommand(sessionID, t, command, "failed", -1, "", "", 0, err.Error())
		return
	}
	defer c.Close()

	start := time.Now()
	exitCode, stdout, stderr := c.ExecCommand(command, timeout, true)
	duration := time.Since(start).Milliseconds()
	status := models.TaskSuccess
	if exitCode != 0 {
		status = models.TaskFailed
	}

	// 保存历史
	_ = a.store.AddCommandHistory(&models.CommandHistory{
		ServerID: t.ServerID, ServerName: t.ServerName, Command: command,
		Status: status, ExitCode: exitCode, Stdout: stdout, Stderr: stderr,
		Duration: duration, CreatedAt: time.Now(),
	})

	a.emitCommand(sessionID, t, command, string(status), exitCode, stdout, stderr, duration, "")
}

// GetCommandHistory 获取命令执行历史
func (a *App) GetCommandHistory(limit int) ([]models.CommandHistory, error) {
	return a.store.GetCommandHistory(limit)
}