package main

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ServerManager2/internal/models"
	"ServerManager2/internal/sshx"
)

// ---- 运维工具 · 磁盘空间检测 ----
//
// 与 monitor.go 的任务表同构：一次检测 = 一个 taskID + 一个可取消的任务。
// 两段式采集：第 1 段「瞬时检查」秒级返回并立刻推送一次报告，第 2 段「体积段」
// 跑完后推送完整报告。前端订阅 ops:disk:* 事件逐段渲染，不用干等 du。
//
// 本模块只做分析，不执行任何处置动作（报告里的命令只是给用户复制的建议）。

// diskTask 一次磁盘检测任务
type diskTask struct {
	serverID string

	mu        sync.Mutex
	client    *sshx.Client
	cancelled bool
}

// attach 记录当前连接（取消时要靠它立刻掐断，让正在跑的 du 立即返回）
func (t *diskTask) attach(c *sshx.Client) {
	t.mu.Lock()
	t.client = c
	t.mu.Unlock()
}

func (t *diskTask) isCancelled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cancelled
}

// abort 立即中止：掐断 SSH 连接（正在执行的 du/find 会马上返回错误），并置取消标记
func (t *diskTask) abort() {
	t.mu.Lock()
	t.cancelled = true
	c := t.client
	t.client = nil
	t.mu.Unlock()
	if c != nil {
		c.Close()
	}
}

// ListServerDisks 取指定服务器的全部文件系统（供"磁盘空间检测"的盘下拉使用）
func (a *App) ListServerDisks(serverID string) ([]models.DiskInfo, error) {
	sv, err := a.getServerWithPassword(serverID)
	if err != nil {
		return nil, err
	}
	c := sshx.New(sv.Host, sv.Port, sv.Username, sv.Password, 10)
	if err := c.Connect(); err != nil {
		return nil, err
	}
	defer c.Close()
	disks := c.GetDisks()
	if disks == nil {
		disks = []models.DiskInfo{}
	}
	return disks, nil
}

// StartDiskAnalysis 启动一次磁盘检测（异步），返回 taskID。
// 进度与报告通过 ops:disk:progress|report|done:<taskID> 事件推送。
//
// taskID 由调用方（前端）先生成、**先订阅事件再调用本方法**：这样即使检测极快就失败
// （例如 SSH 连不上立刻报错），首个事件也不会漏掉，界面不会一直空转。
// 传空串时由后端生成（供脚本/测试调用）。
func (a *App) StartDiskAnalysis(serverID, mount string, useSudo bool, taskID string) (string, error) {
	sv, err := a.getServerWithPassword(serverID)
	if err != nil {
		return "", err
	}
	mount = strings.TrimSpace(mount)
	if mount == "" {
		mount = "/"
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		taskID = uuid.NewString()
	}

	a.opsMu.Lock()
	for _, t := range a.diskTasks {
		if t.serverID == serverID {
			a.opsMu.Unlock()
			return "", errors.New("该服务器已有一个检测正在进行，请等它结束或先取消")
		}
	}
	task := &diskTask{serverID: serverID}
	a.diskTasks[taskID] = task
	a.opsMu.Unlock()

	go a.runDiskAnalysis(task, taskID, sv, mount, useSudo)
	return taskID, nil
}

// CancelDiskAnalysis 取消一次检测（正在跑的 du 会被掐断）
func (a *App) CancelDiskAnalysis(taskID string) error {
	a.opsMu.Lock()
	t, ok := a.diskTasks[taskID]
	a.opsMu.Unlock()
	if !ok {
		return nil
	}
	t.abort()
	return nil
}

// closeAllDiskTasks 中止全部检测（主密码锁定时调用）
func (a *App) closeAllDiskTasks() {
	a.opsMu.Lock()
	tasks := make([]*diskTask, 0, len(a.diskTasks))
	for _, t := range a.diskTasks {
		tasks = append(tasks, t)
	}
	a.opsMu.Unlock()
	for _, t := range tasks {
		t.abort()
	}
}

// DrillDiskDir 对某个目录下钻一层（点击目录排行某一行时调用）。
// 返回的 Partial 表示 du 被超时中断、Items 只是统计到的部分，前端要如实提示。
func (a *App) DrillDiskDir(serverID, mount, path string, useSudo bool) (models.DrillResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = strings.TrimSpace(mount)
	}
	if path == "" {
		path = "/"
	}
	sv, err := a.getServerWithPassword(serverID)
	if err != nil {
		return models.DrillResult{}, err
	}
	c := sshx.New(sv.Host, sv.Port, sv.Username, sv.Password, 10)
	if err := c.Connect(); err != nil {
		return models.DrillResult{}, err
	}
	defer c.Close()
	res, err := c.DiskDrill(path, useSudo)
	if res.Items == nil {
		res.Items = []models.DiskDirItem{}
	}
	return res, err
}

// runDiskAnalysis 检测主体：连接 → 瞬时段 → 体积段 → 完成
func (a *App) runDiskAnalysis(task *diskTask, taskID string, sv *models.Server, mount string, useSudo bool) {
	start := time.Now()
	defer func() {
		a.opsMu.Lock()
		delete(a.diskTasks, taskID)
		a.opsMu.Unlock()
	}()
	elapsed := func() int64 { return time.Since(start).Milliseconds() }

	c := sshx.New(sv.Host, sv.Port, sv.Username, sv.Password, 10)
	if err := c.Connect(); err != nil {
		a.emitDiskProgress(taskID, "error", "连接服务器失败："+err.Error())
		a.emitDiskDone(taskID, map[string]any{"ok": false, "message": "连接服务器失败：" + err.Error()})
		return
	}
	task.attach(c)
	defer func() {
		task.attach(nil)
		c.Close()
	}()

	// 第 1 段：瞬时检查
	a.emitDiskProgress(taskID, "quickscan", "正在检查容量、inode、系统日志与已删除句柄…")
	quick, err := c.DiskQuickscan(mount, useSudo)
	if err != nil {
		a.emitDiskProgress(taskID, "error", "检查失败："+err.Error())
		a.emitDiskDone(taskID, map[string]any{"ok": false, "message": "检查失败：" + err.Error()})
		return
	}
	if task.isCancelled() {
		a.emitDiskDone(taskID, map[string]any{"ok": false, "cancelled": true, "message": "已取消"})
		return
	}
	a.emitDiskReport(taskID, sshx.BuildDiskReport(taskID, sv.ID, sv.Name, mount, quick, nil, elapsed()))

	// 第 2 段：体积段（du + 大文件扫描）
	a.emitDiskProgress(taskID, "volume", "正在扫描目录占用（大目录可能需要十几秒）…")
	vol, _ := c.DiskVolumeScan(mount, useSudo)
	if task.isCancelled() {
		// 取消时也把已有的瞬时段结论再推一次，避免前端停在"进行中"
		a.emitDiskReport(taskID, sshx.BuildDiskReport(taskID, sv.ID, sv.Name, mount, quick, nil, elapsed()))
		a.emitDiskDone(taskID, map[string]any{"ok": false, "cancelled": true, "message": "已取消目录扫描"})
		return
	}
	rep := sshx.BuildDiskReport(taskID, sv.ID, sv.Name, mount, quick, vol, elapsed())
	a.emitDiskReport(taskID, rep)
	a.emitDiskProgress(taskID, "done", "分析完成")
	a.emitDiskDone(taskID, map[string]any{
		"ok":        true,
		"elapsedMs": rep.ElapsedMs,
		"partial":   rep.Partial,
	})
}

func (a *App) emitDiskProgress(taskID, stage, message string) {
	runtime.EventsEmit(a.ctx, "ops:disk:progress:"+taskID, map[string]any{
		"taskId":  taskID,
		"stage":   stage,
		"message": message,
		"at":      time.Now().UnixMilli(),
	})
}

func (a *App) emitDiskReport(taskID string, rep models.DiskReport) {
	runtime.EventsEmit(a.ctx, "ops:disk:report:"+taskID, rep)
}

func (a *App) emitDiskDone(taskID string, payload map[string]any) {
	payload["taskId"] = taskID
	runtime.EventsEmit(a.ctx, "ops:disk:done:"+taskID, payload)
}
