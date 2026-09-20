package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ServerManager2/internal/models"
	"ServerManager2/internal/sshx"
)

const monitorInterval = 5 * time.Second

const (
	// monitorRetention 监控数据保留期：界面最多只看最近 60 个点，留一天足够回溯排查
	monitorRetention = 24 * time.Hour
	// historyPruneInterval 清理周期：与采集间隔解耦，不必每个采集周期都扫一遍表
	historyPruneInterval = 30 * time.Minute
)

// pruneHistory 清除保留期之外的监控历史（全局清理，覆盖所有服务器）
func (a *App) pruneHistory() {
	if a.store == nil {
		return
	}
	n, err := a.store.PruneMonitorHistory(time.Now().Add(-monitorRetention))
	if err != nil {
		fmt.Println("清理监控历史失败:", err)
		return
	}
	if n > 0 {
		fmt.Printf("已清除 %d 条超过 %s 的监控数据\n", n, monitorRetention)
	}
}

// startHistoryPruner 启动保留期清理：先立刻清一次（补上上次退出后积压的过期数据），之后定时清理
func (a *App) startHistoryPruner() {
	a.pruneHistory()
	go func() {
		t := time.NewTicker(historyPruneInterval)
		defer t.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-t.C:
				a.pruneHistory()
			}
		}
	}()
}

// fetchStats 获取服务器实时统计
func (a *App) fetchStats(serverID string) (map[string]any, error) {
	sv, err := a.getServerWithPassword(serverID)
	if err != nil {
		return nil, err
	}
	c := sshx.New(sv.Host, sv.Port, sv.Username, sv.Password, 10)
	if err := c.Connect(); err != nil {
		return nil, err
	}
	defer c.Close()
	return c.GetSystemStats(), nil
}

// GetServerStats 一次性获取服务器实时统计
func (a *App) GetServerStats(serverID string) (map[string]any, error) {
	stats, err := a.fetchStats(serverID)
	if err != nil {
		return nil, err
	}
	a.saveMonitorPoint(serverID, stats)
	return stats, nil
}

func (a *App) saveMonitorPoint(serverID string, stats map[string]any) {
	mp := &models.MonitorPoint{
		ServerID:  serverID,
		CPUUsage:  toF64(stats["cpuUsage"]),
		MemTotal:  toI64(stats["memTotal"]),
		MemUsed:   toI64(stats["memUsed"]),
		MemUsage:  toF64(stats["memUsage"]),
		DiskTotal: toI64(stats["diskTotal"]),
		DiskUsed:  toI64(stats["diskUsed"]),
		DiskUsage: toF64(stats["diskUsage"]),
		DiskJSON:  disksJSON(stats["disks"]),
		NetRx:     toI64(stats["netRx"]),
		NetTx:     toI64(stats["netTx"]),
		Timestamp: time.Now(),
	}
	_ = a.store.AddMonitorPoint(mp)
}

// disksJSON 把采集到的磁盘快照序列化为落库字符串；采集失败或为空时返回空串
func disksJSON(v any) string {
	disks, ok := v.([]models.DiskInfo)
	if !ok || len(disks) == 0 {
		return ""
	}
	b, err := json.Marshal(disks)
	if err != nil {
		return ""
	}
	return string(b)
}

// StartMonitor 启动服务器实时监控轮询
func (a *App) StartMonitor(serverID string) error {
	a.monitorMu.Lock()
	if _, ok := a.monitors[serverID]; ok {
		a.monitorMu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.monitors[serverID] = cancel
	a.monitorMu.Unlock()

	go func() {
		defer func() {
			a.monitorMu.Lock()
			delete(a.monitors, serverID)
			a.monitorMu.Unlock()
		}()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if stats, err := a.fetchStats(serverID); err == nil {
				a.saveMonitorPoint(serverID, stats)
				runtime.EventsEmit(a.ctx, "monitor:data:"+serverID, stats)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(monitorInterval):
			}
		}
	}()
	return nil
}

// StopMonitor 停止服务器实时监控
func (a *App) StopMonitor(serverID string) {
	a.monitorMu.Lock()
	defer a.monitorMu.Unlock()
	if cancel, ok := a.monitors[serverID]; ok {
		cancel()
		delete(a.monitors, serverID)
	}
}

// StopAllMonitors 停止所有监控
func (a *App) StopAllMonitors() {
	a.monitorMu.Lock()
	defer a.monitorMu.Unlock()
	for id, cancel := range a.monitors {
		cancel()
		delete(a.monitors, id)
	}
}

// GetMonitorHistory 获取监控历史数据
func (a *App) GetMonitorHistory(serverID string, limit int) ([]models.MonitorPoint, error) {
	return a.store.GetMonitorHistory(serverID, limit)
}

// GetTopProcesses 获取远程服务器内存占用 TOP 进程
func (a *App) GetTopProcesses(serverID string, limit int) ([]map[string]any, error) {
	sv, err := a.getServerWithPassword(serverID)
	if err != nil {
		return nil, err
	}
	c := sshx.New(sv.Host, sv.Port, sv.Username, sv.Password, 10)
	if err := c.Connect(); err != nil {
		return nil, err
	}
	defer c.Close()
	if limit <= 0 {
		limit = 10
	}
	_, out, _ := c.ExecCommand("ps aux --sort=-%mem | head -n "+itoa(limit+1)+" | tail -n "+itoa(limit), 15, false)
	var result []map[string]any
	for _, line := range splitLines(out) {
		f := fieldsN(line)
		if len(f) < 11 {
			continue
		}
		result = append(result, map[string]any{
			"user": f[0], "pid": f[1], "cpuPercent": safeF64(f[2]),
			"memPercent": safeF64(f[3]), "memMB": safeInt64(f[5]) / 1024,
			"command": joinFields(f[10:]), "time": time.Now().Format("15:04:05"),
		})
	}
	return result, nil
}

func toF64(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func toI64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case float64:
		return int64(t)
	case int:
		return int64(t)
	}
	return 0
}
