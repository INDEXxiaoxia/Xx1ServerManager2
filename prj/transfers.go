package main

import (
	"ServerManager2/internal/sshx"
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"sync"
	"sync/atomic"
	"time"
)

type fileTransfer struct {
	mu        sync.Mutex
	client    *sshx.Client
	cancelled bool
}

func (a *App) CancelFileTransfer(id string) error {

	a.transferMu.Lock()
	if a.transfers == nil {
		a.transfers = make(map[string]*fileTransfer)
	}
	t := a.transfers[id]
	if t == nil {
		// IPC 调用可能先收到取消，再收到上传；保留一个短期取消标记。
		t = &fileTransfer{cancelled: true}
		a.transfers[id] = t
		a.transferMu.Unlock()
		time.AfterFunc(time.Minute, func() {
			a.transferMu.Lock()
			if a.transfers[id] == t {
				delete(a.transfers, id)
			}
			a.transferMu.Unlock()
		})
		return nil
	}
	a.transferMu.Unlock()
	t.mu.Lock()
	t.cancelled = true
	if t.client != nil {
		t.client.InterruptTransport()
	}
	t.mu.Unlock()
	return nil
}
func (a *App) UploadFileTask(id, serverID, localPath, remotePath string) error {
	if id == "" {
		return errors.New("任务 ID 不能为空")
	}
	t := &fileTransfer{}
	a.transferMu.Lock()
	if a.transfers == nil {
		a.transfers = make(map[string]*fileTransfer)
	}
	if previous := a.transfers[id]; previous != nil {
		previous.mu.Lock()
		cancelled := previous.cancelled && previous.client == nil
		previous.mu.Unlock()
		if cancelled {
			delete(a.transfers, id)
			a.transferMu.Unlock()
			return errors.New("已取消")
		}
		a.transferMu.Unlock()
		return errors.New("任务正在运行")
	}
	a.transfers[id] = t
	a.transferMu.Unlock()
	defer func() { a.transferMu.Lock(); delete(a.transfers, id); a.transferMu.Unlock() }()
	sv, err := a.getServerWithPassword(serverID)
	if err != nil {
		return err
	}
	c := sshx.New(sv.Host, sv.Port, sv.Username, sv.Password, 15)
	if err = c.Connect(); err != nil {
		return err
	}
	defer func() { t.mu.Lock(); t.client = nil; c.Close(); t.mu.Unlock() }()
	t.mu.Lock()
	t.client = c
	cancelled := t.cancelled
	t.mu.Unlock()
	if cancelled {
		return errors.New("已取消")
	}
	var last atomic.Int64
	var stalled atomic.Bool
	last.Store(time.Now().UnixNano())
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, last.Load())) > 90*time.Second {
					stalled.Store(true)
					t.mu.Lock()
					if t.client != nil {
						t.client.InterruptTransport()
					}
					t.mu.Unlock()
					return
				}
			}
		}
	}()
	start := time.Now()
	err = c.Upload(localPath, remotePath, func(n, total int64) {
		last.Store(time.Now().UnixNano())
		runtime.EventsEmit(a.ctx, "transfer:progress", map[string]any{"id": id, "transferred": n, "total": total, "speed": float64(n) / time.Since(start).Seconds()})
	})
	if stalled.Load() {
		return errors.New("上传超过 90 秒无进展，连接已关闭，请检查网络后重试")
	}
	return err
}
