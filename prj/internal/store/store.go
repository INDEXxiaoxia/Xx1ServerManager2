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
			timestamp TEXT NOT NULL
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
	return nil
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
		`INSERT INTO monitor_history (server_id,cpu_usage,mem_total,mem_used,mem_usage,disk_total,disk_used,disk_usage,net_rx,net_tx,timestamp) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		mp.ServerID, mp.CPUUsage, mp.MemTotal, mp.MemUsed, mp.MemUsage, mp.DiskTotal, mp.DiskUsed, mp.DiskUsage, mp.NetRx, mp.NetTx, mp.Timestamp.Format(time.RFC3339),
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
		`SELECT id,server_id,cpu_usage,mem_total,mem_used,mem_usage,disk_total,disk_used,disk_usage,net_rx,net_tx,timestamp FROM monitor_history WHERE server_id=? ORDER BY id DESC LIMIT ?`,
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
		if err := rows.Scan(&mp.ID, &mp.ServerID, &mp.CPUUsage, &mp.MemTotal, &mp.MemUsed, &mp.MemUsage, &mp.DiskTotal, &mp.DiskUsed, &mp.DiskUsage, &mp.NetRx, &mp.NetTx, &ts); err != nil {
			return nil, err
		}
		mp.Timestamp, _ = time.Parse(time.RFC3339, ts)
		points = append(points, mp)
	}
	// 升序返回
	for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
		points[i], points[j] = points[j], points[i]
	}
	return points, rows.Err()
}

// ---- 命令历史 ----

func (s *Store) AddCommandHistory(h *models.CommandHistory) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO command_history (server_id,server_name,command,status,exit_code,stdout,stderr,duration,created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		h.ServerID, h.ServerName, h.Command, string(h.Status), h.ExitCode, h.Stdout, h.Stderr, h.Duration, h.CreatedAt.Format(time.RFC3339),
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
		`SELECT id,server_id,server_name,command,status,exit_code,stdout,stderr,duration,created_at FROM command_history ORDER BY id DESC LIMIT ?`,
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
		if err := rows.Scan(&h.ID, &h.ServerID, &h.ServerName, &h.Command, &status, &h.ExitCode, &h.Stdout, &h.Stderr, &h.Duration, &created); err != nil {
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

func joinIDs(ids []string) string {
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
	return ids
}
