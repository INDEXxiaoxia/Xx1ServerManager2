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

// DirSizeItem 目录大小计算结果（每个目录独立成败，便于前端逐行渲染）
type DirSizeItem struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Method string `json:"method"` // du | walk
	Error  string `json:"error"`
}

// SearchResult 远程文件名搜索结果
type SearchResult struct {
	Root      string      `json:"root"`
	Keyword   string      `json:"keyword"`
	Recursive bool        `json:"recursive"`
	Items     []FileEntry `json:"items"`
	Truncated bool        `json:"truncated"` // 命中上限被截断
	Scanned   int         `json:"scanned"`   // 已扫描条目数
	ElapsedMs int64       `json:"elapsedMs"`
}

// TextFileMeta 文本文件元信息（编码/二进制/换行探测结果，不加载全文）
type TextFileMeta struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Encoding string `json:"encoding"`
	HasBOM   bool   `json:"hasBOM"`
	Binary   bool   `json:"binary"`
	EOL      string `json:"eol"`      // lf | crlf | cr
	EOLMixed bool   `json:"eolMixed"` // 同一文件混用换行
	Editable bool   `json:"editable"`
	Message  string `json:"message"` // 不可编辑时的原因说明
}

// TextFileContent 文本文件内容。
// Base64 承载原始字节（未解码），避免非 UTF-8 内容经 JSON 桥被替换为 U+FFFD。
type TextFileContent struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Base64   string `json:"base64"`
	Encoding string `json:"encoding"`
	HasBOM   bool   `json:"hasBOM"`
	EOL      string `json:"eol"`
	EOLMixed bool   `json:"eolMixed"`
	Truncated bool  `json:"truncated"`
}

// TextFileWriteRequest 文本文件保存请求
type TextFileWriteRequest struct {
	Text     string `json:"text"`
	Encoding string `json:"encoding"`
	KeepBOM  bool   `json:"keepBOM"` // 显式为 true 才会写入 UTF-8 BOM
	EOL      string `json:"eol"`     // lf | crlf | cr
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