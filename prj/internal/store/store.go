package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"ServerManager2/internal/models"
)

// Store 数据访问层
type Store struct {
	db      *sql.DB
	dbPath  string
	dataDir string
	mu      sync.RWMutex
}

// ConfigDir 返回配置文件（数据库）所在目录
func (s *Store) ConfigDir() string { return s.dataDir }

// DBPath 返回数据库文件完整路径
func (s *Store) DBPath() string { return s.dbPath }

// NewStore 创建并初始化数据库
func NewStore(dbPath string) (*Store, error) {
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir := filepath.Join(home, "ServerManager2")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		dbPath = filepath.Join(dir, "data.db")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite 单写入者

	s := &Store{db: db, dbPath: dbPath, dataDir: filepath.Dir(dbPath)}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) init() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS servers (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 22,
			username TEXT NOT NULL,
			password TEXT,
			group_ids TEXT,
			status TEXT NOT NULL DEFAULT 'disconnected',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			color TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS preset_commands (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			command TEXT NOT NULL,
			description TEXT,
			category TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS preset_paths (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			path TEXT NOT NULL,
			description TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS monitor_history (
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
			timestamp TEXT NOT NULL,
			disk_json TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS command_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id TEXT,
			server_name TEXT,
			command TEXT,
			status TEXT,
			exit_code INTEGER,
			stdout TEXT,
			stderr TEXT,
			duration INTEGER,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return s.migrate()
}

// migrate 幂等增量迁移：老库补 disk_json / source / ai_session_id 列与查询索引。
// 先查 PRAGMA 再决定是否 ALTER，重复启动不会报错；老库升级后历史数据照常可读。
func (s *Store) migrate() error {
	if !s.hasColumn("monitor_history", "disk_json") {
		if _, err := s.db.Exec(`ALTER TABLE monitor_history ADD COLUMN disk_json TEXT`); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}
	// command_history 记录命令来源：manual（用户手敲/批量）、batch、ai（AI 接管执行），
	// 让"AI 到底在服务器上做了什么"有据可查
	if !s.hasColumn("command_history", "source") {
		if _, err := s.db.Exec(`ALTER TABLE command_history ADD COLUMN source TEXT`); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}
	if !s.hasColumn("command_history", "ai_session_id") {
		if _, err := s.db.Exec(`ALTER TABLE command_history ADD COLUMN ai_session_id TEXT`); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_monitor_history_server_id ON monitor_history(server_id, id DESC)`)
	return nil
}

// hasColumn 判断表是否已有某列
func (s *Store) hasColumn(table, column string) bool {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false
		}
		if name == column {
			return true
		}
	}
	return false
}

func (s *Store) Close() error {
	return s.db.Close()
}

// ---- 服务器 ----

func (s *Store) GetServers() ([]models.Server, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT id,name,host,port,username,password,group_ids,status,created_at,updated_at FROM servers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	servers := []models.Server{}
	for rows.Next() {
		var sv models.Server
		var groupIDs string
		var status, created, updated string
		if err := rows.Scan(&sv.ID, &sv.Name, &sv.Host, &sv.Port, &sv.Username, &sv.Password, &groupIDs, &status, &created, &updated); err != nil {
			return nil, err
		}
		sv.GroupIDs = parseIDs(groupIDs)
		sv.Status = models.ServerStatus(status)
		sv.CreatedAt, _ = time.Parse(time.RFC3339, created)
		sv.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		servers = append(servers, sv)
	}
	return servers, rows.Err()
}

func (s *Store) GetServerByID(id string) (*models.Server, error) {
	servers, err := s.GetServers()
	if err != nil {
		return nil, err
	}
	for i := range servers {
		if servers[i].ID == id {
			return &servers[i], nil
		}
	}
	return nil, nil
}

func (s *Store) AddServer(sv *models.Server) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO servers (id,name,host,port,username,password,group_ids,status,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		sv.ID, sv.Name, sv.Host, sv.Port, sv.Username, sv.Password, joinIDs(sv.GroupIDs), string(sv.Status), sv.CreatedAt.Format(time.RFC3339), sv.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) UpdateServer(sv *models.Server) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`UPDATE servers SET name=?,host=?,port=?,username=?,password=?,group_ids=?,status=?,updated_at=? WHERE id=?`,
		sv.Name, sv.Host, sv.Port, sv.Username, sv.Password, joinIDs(sv.GroupIDs), string(sv.Status), sv.UpdatedAt.Format(time.RFC3339), sv.ID,
	)
	return err
}

func (s *Store) DeleteServer(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM servers WHERE id=?`, id)
	return err
}

// SetServerGroup 只写入服务器的分组列（与 updated_at）。
// 单独一条语句而不是复用 UpdateServer：批量分配分组不该重写密码/状态等无关列，
// 避免把并发修改的其他字段一起盖回旧值。
func (s *Store) SetServerGroup(serverID, groupID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`UPDATE servers SET group_ids=?, updated_at=? WHERE id=?`,
		joinIDs(groupIDsOf(groupID)), at.Format(time.RFC3339), serverID,
	)
	return err
}

// SetServerPassword 只写入密码密文列（主密码变更后重新加密用）
func (s *Store) SetServerPassword(serverID, cipher string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE servers SET password=? WHERE id=?`, cipher, serverID)
	return err
}

// ---- 分组 ----

func (s *Store) GetGroups() ([]models.Group, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT id,name,description,color,created_at FROM groups`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []models.Group
	for rows.Next() {
		var g models.Group
		var desc, color, created string
		if err := rows.Scan(&g.ID, &g.Name, &desc, &color, &created); err != nil {
			return nil, err
		}
		g.Description = desc
		g.Color = color
		g.CreatedAt, _ = time.Parse(time.RFC3339, created)
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *Store) AddGroup(g *models.Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO groups (id,name,description,color,created_at) VALUES (?,?,?,?,?)`,
		g.ID, g.Name, g.Description, g.Color, g.CreatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) UpdateGroup(g *models.Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`UPDATE groups SET name=?,description=?,color=? WHERE id=?`,
		g.Name, g.Description, g.Color, g.ID,
	)
	return err
}

func (s *Store) DeleteGroup(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM groups WHERE id=?`, id)
	return err
}

// ---- 预设 ----

func (s *Store) GetPresetCommands() ([]models.PresetCommand, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT id,name,command,description,category,created_at FROM preset_commands`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.PresetCommand{}
	for rows.Next() {
		var c models.PresetCommand
		var desc, cat, created string
		if err := rows.Scan(&c.ID, &c.Name, &c.Command, &desc, &cat, &created); err != nil {
			return nil, err
		}
		c.Description = desc
		c.Category = cat
		c.CreatedAt, _ = time.Parse(time.RFC3339, created)
		items = append(items, c)
	}
	return items, rows.Err()
}

func (s *Store) AddPresetCommand(c *models.PresetCommand) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO preset_commands (id,name,command,description,category,created_at) VALUES (?,?,?,?,?,?)`,
		c.ID, c.Name, c.Command, c.Description, c.Category, c.CreatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) DeletePresetCommand(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM preset_commands WHERE id=?`, id)
	return err
}

func (s *Store) UpdatePresetCommand(c *models.PresetCommand) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`UPDATE preset_commands SET name=?,command=?,description=?,category=? WHERE id=?`,
		c.Name, c.Command, c.Description, c.Category, c.ID,
	)
	return err
}

func (s *Store) GetPresetPaths() ([]models.PresetPath, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT id,name,path,description,created_at FROM preset_paths`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.PresetPath{}
	for rows.Next() {
		var p models.PresetPath
		var desc, created string
		if err := rows.Scan(&p.ID, &p.Name, &p.Path, &desc, &created); err != nil {
			return nil, err
		}
		p.Description = desc
		p.CreatedAt, _ = time.Parse(time.RFC3339, created)
		items = append(items, p)
	}
	return items, rows.Err()
}

func (s *Store) AddPresetPath(p *models.PresetPath) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO preset_paths (id,name,path,description,created_at) VALUES (?,?,?,?,?)`,
		p.ID, p.Name, p.Path, p.Description, p.CreatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) DeletePresetPath(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM preset_paths WHERE id=?`, id)
	return err
}

func (s *Store) UpdatePresetPath(p *models.PresetPath) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`UPDATE preset_paths SET name=?,path=?,description=? WHERE id=?`,
		p.Name, p.Path, p.Description, p.ID,
	)
	return err
}

// ---- 监控历史 ----

func (s *Store) AddMonitorPoint(mp *models.MonitorPoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO monitor_history (server_id,cpu_usage,mem_total,mem_used,mem_usage,disk_total,disk_used,disk_usage,net_rx,net_tx,timestamp,disk_json) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		mp.ServerID, mp.CPUUsage, mp.MemTotal, mp.MemUsed, mp.MemUsage, mp.DiskTotal, mp.DiskUsed, mp.DiskUsage, mp.NetRx, mp.NetTx, mp.Timestamp.Format(time.RFC3339), nullableDiskJSON(mp.DiskJSON),
	)
	return err
}

func (s *Store) GetMonitorHistory(serverID string, limit int) ([]models.MonitorPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id,server_id,cpu_usage,mem_total,mem_used,mem_usage,disk_total,disk_used,disk_usage,net_rx,net_tx,timestamp,disk_json FROM monitor_history WHERE server_id=? ORDER BY id DESC LIMIT ?`,
		serverID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var points []models.MonitorPoint
	for rows.Next() {
		var mp models.MonitorPoint
		var ts string
		var diskJSON sql.NullString
		if err := rows.Scan(&mp.ID, &mp.ServerID, &mp.CPUUsage, &mp.MemTotal, &mp.MemUsed, &mp.MemUsage, &mp.DiskTotal, &mp.DiskUsed, &mp.DiskUsage, &mp.NetRx, &mp.NetTx, &ts, &diskJSON); err != nil {
			return nil, err
		}
		mp.Timestamp, _ = time.Parse(time.RFC3339, ts)
		mp.DiskJSON = diskJSON.String
		mp.Disks = disksFromJSON(diskJSON.String, &mp)
		points = append(points, mp)
	}
	// 升序返回
	for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
		points[i], points[j] = points[j], points[i]
	}
	return points, rows.Err()
}

// PruneMonitorHistory 删除 cutoff 之前的历史监控点，返回删除行数。
// 时间比较走 julianday() 而不是按 RFC3339 字符串字面量比：跨时区/夏令时切换时
// 字面量顺序不等于真实时间顺序，交给 SQLite 解析时区后缀更稳妥。
func (s *Store) PruneMonitorHistory(cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(
		`DELETE FROM monitor_history WHERE julianday(timestamp) < julianday(?)`,
		cutoff.Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// nullableDiskJSON 没有磁盘快照时写 NULL，让新老数据走同一条回退逻辑
func nullableDiskJSON(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// disksFromJSON 还原历史点的多磁盘快照。
// disk_json 为空（迁移前的老数据）或损坏时，回退成一条由旧字段构成的根分区数据，历史曲线不断档。
func disksFromJSON(raw string, mp *models.MonitorPoint) []models.DiskInfo {
	if strings.TrimSpace(raw) != "" {
		var ds []models.DiskInfo
		if err := json.Unmarshal([]byte(raw), &ds); err == nil && len(ds) > 0 {
			return ds
		}
	}
	if mp.DiskTotal > 0 || mp.DiskUsed > 0 || mp.DiskUsage > 0 {
		avail := mp.DiskTotal - mp.DiskUsed
		if avail < 0 {
			avail = 0
		}
		return []models.DiskInfo{{
			Mount:   "/",
			TotalMB: mp.DiskTotal,
			UsedMB:  mp.DiskUsed,
			AvailMB: avail,
			Usage:   mp.DiskUsage,
		}}
	}
	return nil
}

// ---- 命令历史 ----

func (s *Store) AddCommandHistory(h *models.CommandHistory) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if h.Source == "" {
		h.Source = "manual"
	}
	_, err := s.db.Exec(
		`INSERT INTO command_history (server_id,server_name,command,status,exit_code,stdout,stderr,duration,created_at,source,ai_session_id) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		h.ServerID, h.ServerName, h.Command, string(h.Status), h.ExitCode, h.Stdout, h.Stderr, h.Duration, h.CreatedAt.Format(time.RFC3339), h.Source, h.AISessionID,
	)
	return err
}

func (s *Store) GetCommandHistory(limit int) ([]models.CommandHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id,server_id,server_name,command,status,exit_code,stdout,stderr,duration,created_at,COALESCE(source,''),COALESCE(ai_session_id,'') FROM command_history ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.CommandHistory{}
	for rows.Next() {
		var h models.CommandHistory
		var status, created string
		if err := rows.Scan(&h.ID, &h.ServerID, &h.ServerName, &h.Command, &status, &h.ExitCode, &h.Stdout, &h.Stderr, &h.Duration, &created, &h.Source, &h.AISessionID); err != nil {
			return nil, err
		}
		h.Status = models.TaskStatus(status)
		h.CreatedAt, _ = time.Parse(time.RFC3339, created)
		items = append(items, h)
	}
	return items, rows.Err()
}

// ---- 设置 ----

func (s *Store) GetSettings() (models.Settings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	def := models.Settings{MaxConcurrent: 5, UploadTimeout: 300, CommandTimeout: 30, Theme: "light"}
	rows, err := s.db.Query(`SELECT key,value FROM app_settings`)
	if err != nil {
		return def, err
	}
	defer rows.Close()
	kv := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return def, err
		}
		kv[k] = v
	}
	if v, ok := kv["maxConcurrent"]; ok {
		fmt.Sscanf(v, "%d", &def.MaxConcurrent)
	}
	if v, ok := kv["uploadTimeout"]; ok {
		fmt.Sscanf(v, "%d", &def.UploadTimeout)
	}
	if v, ok := kv["commandTimeout"]; ok {
		fmt.Sscanf(v, "%d", &def.CommandTimeout)
	}
	if v, ok := kv["theme"]; ok {
		def.Theme = v
	}
	return def, nil
}

func (s *Store) SaveSettings(st models.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	vals := map[string]string{
		"maxConcurrent":  fmt.Sprintf("%d", st.MaxConcurrent),
		"uploadTimeout":  fmt.Sprintf("%d", st.UploadTimeout),
		"commandTimeout": fmt.Sprintf("%d", st.CommandTimeout),
		"theme":          st.Theme,
	}
	for k, v := range vals {
		_, err := s.db.Exec(`INSERT INTO app_settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, k, v)
		if err != nil {
			return err
		}
	}
	return nil
}

// ---- 主密码相关 ----
// 主密码哈希与盐值存于 app_settings

func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *Store) SetSetting(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO app_settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// ---- 辅助 ----
// 分组单属约束：一个服务器只属于一个分组，group_ids 里至多保留一个 id（首项生效）。
// 历史数据里若存了多个 id，读取与写入都会收敛为首项，任一次编辑即把收敛结果落库。

// groupIDsOf 把"生效分组 id"包成存储用的数组形态（空 id 即未分组）
func groupIDsOf(groupID string) []string {
	if groupID == "" {
		return nil
	}
	return []string{groupID}
}

func joinIDs(ids []string) string {
	ids = normalizeGroupIDs(ids)
	if len(ids) == 0 {
		return ""
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

func parseIDs(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(s), &ids); err != nil {
		return nil
	}
	return normalizeGroupIDs(ids)
}

// normalizeGroupIDs 去空串、去重，并只保留第一个 id
func normalizeGroupIDs(ids []string) []string {
	out := make([]string, 0, 1)
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if len(out) == 0 {
			out = append(out, id)
		}
	}
	return out
}
