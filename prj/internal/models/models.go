package models

import "time"

// ServerStatus 服务器连接状态
type ServerStatus string

const (
	StatusDisconnected ServerStatus = "disconnected"
	StatusConnecting   ServerStatus = "connecting"
	StatusConnected    ServerStatus = "connected"
	StatusError        ServerStatus = "error"
)

// TaskStatus 任务状态
type TaskStatus string

const (
	TaskWaiting  TaskStatus = "waiting"
	TaskRunning  TaskStatus = "running"
	TaskSuccess  TaskStatus = "success"
	TaskFailed   TaskStatus = "failed"
	TaskCanceled TaskStatus = "cancelled"
)

// Server 服务器配置
type Server struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Host      string       `json:"host"`
	Port      int          `json:"port"`
	Username  string       `json:"username"`
	Password  string       `json:"password,omitempty"` // 密文存储，不返回
	GroupIDs  []string     `json:"groupIds"`
	Status    ServerStatus `json:"status"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

// Group 分组
type Group struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Color       string    `json:"color"`
	CreatedAt   time.Time `json:"createdAt"`
}

// PresetCommand 预设命令
type PresetCommand struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Command     string    `json:"command"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	CreatedAt   time.Time `json:"createdAt"`
}

// PresetPath 预设路径
type PresetPath struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

// MonitorPoint 监控数据点
type MonitorPoint struct {
	ID         int       `json:"id"`
	ServerID   string    `json:"serverId"`
	CPUUsage   float64   `json:"cpuUsage"`
	MemTotal   int64     `json:"memTotal"`
	MemUsed    int64     `json:"memUsed"`
	MemUsage   float64   `json:"memUsage"`
	DiskTotal  int64     `json:"diskTotal"`
	DiskUsed   int64     `json:"diskUsed"`
	DiskUsage  float64   `json:"diskUsage"`
	NetRx      int64     `json:"netRx"`
	NetTx      int64     `json:"netTx"`
	Timestamp  time.Time `json:"timestamp"`
}

// CommandHistory 命令执行历史
type CommandHistory struct {
	ID        int64      `json:"id"`
	ServerID  string     `json:"serverId"`
	ServerName string    `json:"serverName"`
	Command   string     `json:"command"`
	Status    TaskStatus `json:"status"`
	ExitCode  int        `json:"exitCode"`
	Stdout    string     `json:"stdout"`
	Stderr    string     `json:"stderr"`
	Duration  int64      `json:"duration"` // milliseconds
	CreatedAt time.Time  `json:"createdAt"`
}

// Settings 应用设置
type Settings struct {
	MaxConcurrent  int    `json:"maxConcurrent"`
	UploadTimeout  int    `json:"uploadTimeout"`
	CommandTimeout int    `json:"commandTimeout"`
	Theme          string `json:"theme"`
}

// FileEntry 远程文件条目
type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"modTime"`
}

// SystemService systemd 服务
type SystemService struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	LoadState   string `json:"loadState"`
	ActiveState string `json:"activeState"`
	SubState    string `json:"subState"`
	Enabled     string `json:"enabled"`
}