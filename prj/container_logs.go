package main

import (
	"fmt"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) WatchContainerLogs(id, serverID, engine, containerID string, lines int) error {
	if _, err := containerShellCommand(engine, containerID); err != nil {
		return err
	}
	if lines != 200 && lines != 500 && lines != 1000 {
		return fmt.Errorf("日志行数无效")
	}
	t := &fileTransfer{}
	a.transferMu.Lock()
	if a.transfers == nil {
		a.transfers = map[string]*fileTransfer{}
	}
	if a.transfers[id] != nil {
		a.transferMu.Unlock()
		return fmt.Errorf("日志任务已存在")
	}
	a.transfers[id] = t
	a.transferMu.Unlock()
	defer func() { a.transferMu.Lock(); delete(a.transfers, id); a.transferMu.Unlock() }()
	sv, err := a.getServerWithPassword(serverID)
	if err != nil {
		return err
	}
	c := a.newServerClient(sv, 10)
	if err = c.Connect(); err != nil {
		return err
	}
	defer func() { t.mu.Lock(); t.client = nil; c.Close(); t.mu.Unlock() }()
	t.mu.Lock()
	t.client = c
	cancelled := t.cancelled
	t.mu.Unlock()
	if cancelled {
		return nil
	}
	return c.StreamCommand(fmt.Sprintf("%s logs --tail %d --timestamps --follow %s", engine, lines, containerID), func(s string) { runtime.EventsEmit(a.ctx, "container:logs:"+id, s) })
}
