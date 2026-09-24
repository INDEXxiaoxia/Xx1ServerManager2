package main

import (
	"testing"
	"time"
)

// TestMonitorRetentionWindow 监控历史只保留一小时：界面最多看最近 60 个点（5 分钟），
// 一小时的保留窗口足够回溯排查，同时把磁盘占用压到最小。
func TestMonitorRetentionWindow(t *testing.T) {
	if monitorRetention != time.Hour {
		t.Fatalf("监控数据保留期应为 1 小时，got %v", monitorRetention)
	}
	if historyPruneInterval != 5*time.Minute {
		t.Fatalf("清理周期应为 5 分钟，got %v", historyPruneInterval)
	}
	// 清理周期必须明显小于保留期，否则过期数据会长时间滞留
	if historyPruneInterval >= monitorRetention {
		t.Fatalf("清理周期(%v)应远小于保留期(%v)", historyPruneInterval, monitorRetention)
	}
	// 5 秒采集一次，一小时约 720 个点
	pointsPerHour := int(monitorRetention / monitorInterval)
	if pointsPerHour < 600 || pointsPerHour > 800 {
		t.Fatalf("每小时监控点数异常: %d", pointsPerHour)
	}
}