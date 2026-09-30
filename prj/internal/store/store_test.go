package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"ServerManager2/internal/models"
)

// newLegacyDB 构造"迁移前"的库：monitor_history 没有 disk_json 列，并预置一行旧数据
func newLegacyDB(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("打开老库失败: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE monitor_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		server_id TEXT NOT NULL,
		cpu_usage REAL,
		mem_total INTEGER,
		mem_used INTEGER,
		mem_usage REAL,
		disk_total INTEGER,
		disk_used INTEGER,
		disk_usage REAL,
		net_rx INTEGER,
		net_tx INTEGER,
		timestamp TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("建老表失败: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO monitor_history (server_id,cpu_usage,mem_total,mem_used,mem_usage,disk_total,disk_used,disk_usage,net_rx,net_tx,timestamp) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		"s1", 10.5, 8000, 3200, 40.0, 100000, 88300, 88.3, 1, 2, time.Now().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("写入老数据失败: %v", err)
	}
}

// TestMigrationOnLegacyDB 老库升级：自动补 disk_json 列，且旧历史数据仍可读（回退成根分区一条）
func TestMigrationOnLegacyDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	newLegacyDB(t, dbPath)

	st, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("老库升级失败: %v", err)
	}
	defer st.Close()

	if !st.hasColumn("monitor_history", "disk_json") {
		t.Fatalf("迁移应补上 disk_json 列")
	}

	points, err := st.GetMonitorHistory("s1", 10)
	if err != nil {
		t.Fatalf("读取历史失败: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("应读到 1 个历史点，got %d", len(points))
	}
	d := points[0].Disks
	if len(d) != 1 || d[0].Mount != "/" {
		t.Fatalf("旧行应回退为一条根分区数据，got %+v", d)
	}
	if d[0].TotalMB != 100000 || d[0].UsedMB != 88300 || d[0].Usage != 88.3 || d[0].AvailMB != 11700 {
		t.Fatalf("回退数据与旧字段不一致: %+v", d[0])
	}
}

// TestMonitorPointDiskRoundTrip 多磁盘快照读写往返：disk_json 原样还原成 Disks
func TestMonitorPointDiskRoundTrip(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "new.db"))
	if err != nil {
		t.Fatalf("建库失败: %v", err)
	}
	defer st.Close()

	raw := `[{"mount":"/","device":"/dev/sda1","fstype":"ext4","totalMB":10000,"usedMB":6200,"availMB":3800,"usage":62,"isPseudo":false,"isReadOnly":false},` +
		`{"mount":"/data","device":"/dev/sdb1","fstype":"xfs","totalMB":102400,"usedMB":90419,"availMB":8743,"usage":88.3,"isPseudo":false,"isReadOnly":false}]`
	mp := &models.MonitorPoint{
		ServerID: "s1", CPUUsage: 12.4, MemTotal: 8000, MemUsed: 3200, MemUsage: 40,
		DiskTotal: 102400, DiskUsed: 90419, DiskUsage: 88.3,
		DiskJSON: raw, NetRx: 10, NetTx: 20, Timestamp: time.Now(),
	}
	if err := st.AddMonitorPoint(mp); err != nil {
		t.Fatalf("写入监控点失败: %v", err)
	}

	points, err := st.GetMonitorHistory("s1", 10)
	if err != nil {
		t.Fatalf("读取历史失败: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("应读到 1 个历史点，got %d", len(points))
	}
	disks := points[0].Disks
	if len(disks) != 2 {
		t.Fatalf("应还原出 2 个磁盘，got %+v", disks)
	}
	if disks[0].Mount != "/" || disks[0].Usage != 62 {
		t.Fatalf("根分区还原异常: %+v", disks[0])
	}
	if disks[1].Mount != "/data" || disks[1].Device != "/dev/sdb1" || disks[1].Fstype != "xfs" {
		t.Fatalf("数据盘还原异常: %+v", disks[1])
	}
	// 旧字段必须同时保留，旧界面照常显示
	if points[0].DiskUsage != 88.3 || points[0].DiskTotal != 102400 {
		t.Fatalf("旧字段不应丢失: %+v", points[0])
	}
}

// TestDisksFromJSONFallback 快照损坏时回退旧字段，不应报错或丢点
func TestDisksFromJSONFallback(t *testing.T) {
	mp := &models.MonitorPoint{DiskTotal: 100, DiskUsed: 40, DiskUsage: 40}
	got := disksFromJSON("{不是合法 JSON", mp)
	if len(got) != 1 || got[0].Mount != "/" || got[0].TotalMB != 100 {
		t.Fatalf("损坏快照应回退旧字段，got %+v", got)
	}
	// 空快照 + 全零旧字段：返回空列表，前端显示"暂无磁盘数据"而不是假的 0%
	if got := disksFromJSON("", &models.MonitorPoint{}); got != nil {
		t.Fatalf("无任何数据时应返回空，got %+v", got)
	}
}

// TestGroupIDsCollapseToSingle 分组单属：写入多个分组只保留首个（含空串过滤）
func TestGroupIDsCollapseToSingle(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "groups.db"))
	if err != nil {
		t.Fatalf("建库失败: %v", err)
	}
	defer st.Close()

	now := time.Now()
	sv := &models.Server{
		ID: "s1", Name: "web-01", Host: "10.0.0.1", Port: 22, Username: "root",
		GroupIDs: []string{"g1", "g2", "g3"}, CreatedAt: now, UpdatedAt: now,
	}
	if err := st.AddServer(sv); err != nil {
		t.Fatalf("写入服务器失败: %v", err)
	}

	got, err := st.GetServerByID("s1")
	if err != nil || got == nil {
		t.Fatalf("读取服务器失败: %v", err)
	}
	if len(got.GroupIDs) != 1 || got.GroupIDs[0] != "g1" {
		t.Fatalf("多分组应收敛为首项 g1，got %+v", got.GroupIDs)
	}

	// 空串/重复不再产生多值
	if ids := parseIDs(`["","g9","g9"]`); len(ids) != 1 || ids[0] != "g9" {
		t.Fatalf("parseIDs 应收敛为 [g9]，got %+v", ids)
	}
	if s := joinIDs([]string{"", ""}); s != "" {
		t.Fatalf("全空分组应序列化为空串，got %q", s)
	}
}

// TestLegacyMultiGroupCollapse 老库直插的多分组行读取时也收敛为首项
func TestLegacyMultiGroupCollapse(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy_groups.db")
	st, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("建库失败: %v", err)
	}
	now := time.Now().Format(time.RFC3339)
	if _, err := st.db.Exec(
		`INSERT INTO servers (id,name,host,port,username,password,group_ids,status,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		"s1", "db-01", "10.0.0.2", 22, "root", "", `["gb","ga"]`, "disconnected", now, now,
	); err != nil {
		t.Fatalf("直插老数据失败: %v", err)
	}
	got, err := st.GetServers()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(got) != 1 || len(got[0].GroupIDs) != 1 || got[0].GroupIDs[0] != "gb" {
		t.Fatalf("老数据应收敛为首项 gb，got %+v", got)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
}

// TestSetServerGroupRoundTrip 只改分组列：其他字段（密码密文等）不受影响
func TestSetServerGroupRoundTrip(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "setgroup.db"))
	if err != nil {
		t.Fatalf("建库失败: %v", err)
	}
	defer st.Close()

	now := time.Now()
	sv := &models.Server{
		ID: "s1", Name: "web-01", Host: "10.0.0.1", Port: 22, Username: "root",
		Password: "CIPHERTEXT", GroupIDs: []string{"g1"}, Status: models.StatusConnected,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := st.AddServer(sv); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	at := now.Add(time.Minute)
	if err := st.SetServerGroup("s1", "g2", at); err != nil {
		t.Fatalf("设置分组失败: %v", err)
	}
	got, _ := st.GetServerByID("s1")
	if len(got.GroupIDs) != 1 || got.GroupIDs[0] != "g2" {
		t.Fatalf("分组应变为 g2，got %+v", got.GroupIDs)
	}
	if got.Password != "CIPHERTEXT" || got.Name != "web-01" {
		t.Fatalf("无关列不应被改写: %+v", got)
	}
	if !got.UpdatedAt.Equal(at.Truncate(time.Second)) && got.UpdatedAt.Before(at.Add(-time.Second)) {
		t.Fatalf("updated_at 应被更新，got %v", got.UpdatedAt)
	}

	// 空分组 id = 移出分组
	if err := st.SetServerGroup("s1", "", at); err != nil {
		t.Fatalf("移出分组失败: %v", err)
	}
	got, _ = st.GetServerByID("s1")
	if len(got.GroupIDs) != 0 {
		t.Fatalf("移出后不应有分组，got %+v", got.GroupIDs)
	}
}

// TestMigrationIdempotent 重复打开同一库不应因重复迁移报错
func TestMigrationIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "repeat.db")
	for i := 0; i < 3; i++ {
		st, err := NewStore(dbPath)
		if err != nil {
			t.Fatalf("第 %d 次打开失败: %v", i+1, err)
		}
		if !st.hasColumn("monitor_history", "disk_json") {
			t.Fatalf("第 %d 次打开后缺列", i+1)
		}
		if err := st.Close(); err != nil {
			t.Fatalf("关闭失败: %v", err)
		}
	}
}

// TestPruneMonitorHistory 保留期清理：只删截止时间之前的点，保留期内的点一条不动
func TestPruneMonitorHistory(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "prune.db"))
	if err != nil {
		t.Fatalf("建库失败: %v", err)
	}
	defer st.Close()

	now := time.Now()
	// 保留期已改为 1 小时：前两条超期（90min / 3h 前），后三条在保留期内
	stamps := []time.Time{
		now.Add(-3 * time.Hour),
		now.Add(-90 * time.Minute),
		now.Add(-50 * time.Minute),
		now.Add(-10 * time.Minute),
		now,
	}
	for i, ts := range stamps {
		if err := st.AddMonitorPoint(&models.MonitorPoint{ServerID: "s1", CPUUsage: float64(i), Timestamp: ts}); err != nil {
			t.Fatalf("写入监控点失败: %v", err)
		}
	}
	// 另一台服务器的数据同样受保留期约束（清理是全局的）
	if err := st.AddMonitorPoint(&models.MonitorPoint{ServerID: "s2", Timestamp: now.Add(-2 * time.Hour)}); err != nil {
		t.Fatalf("写入 s2 监控点失败: %v", err)
	}

	n, err := st.PruneMonitorHistory(now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if n != 3 {
		t.Fatalf("应删除 3 行（s1 两条 + s2 一条），got %d", n)
	}

	points, err := st.GetMonitorHistory("s1", 100)
	if err != nil {
		t.Fatalf("读取历史失败: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("保留期内应剩 3 条，got %d", len(points))
	}
	if points[0].Timestamp.Before(now.Add(-time.Hour)) {
		t.Fatalf("超期数据未被清除: %+v", points[0])
	}
	if left, _ := st.GetMonitorHistory("s2", 100); len(left) != 0 {
		t.Fatalf("s2 超期数据应被清除，got %d", len(left))
	}

	// 同一个截止时间再清一次：没有可删的，且不应报错
	if n, err := st.PruneMonitorHistory(now.Add(-time.Hour)); err != nil || n != 0 {
		t.Fatalf("重复清理应删除 0 行，got %d err=%v", n, err)
	}

	// 截止时间推到未来：全部清空
	if n, err := st.PruneMonitorHistory(now.Add(time.Hour)); err != nil || n != 3 {
		t.Fatalf("应清空剩余 3 条，got %d err=%v", n, err)
	}
	if points, _ := st.GetMonitorHistory("s1", 100); len(points) != 0 {
		t.Fatalf("清空后不应有数据，got %d", len(points))
	}

	// 空表清理不报错
	if n, err := st.PruneMonitorHistory(now); err != nil || n != 0 {
		t.Fatalf("空表清理异常: n=%d err=%v", n, err)
	}
}
