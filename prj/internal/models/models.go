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
	ID       string `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"` // 密文存储，不返回
	// GroupIDs 生效分组：**一个服务器只属于一个分组**，因此长度至多 1（首项生效）。
	// 保持数组形态是为了不改动已落库的 group_ids 列与前端既有字段名；读写都会收敛。
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

// DiskInfo 单个文件系统的容量信息（容量单位 MB，与既有 diskTotal 口径一致）
type DiskInfo struct {
	Mount      string  `json:"mount"`      // 挂载点，如 / 或 /data
	Device     string  `json:"device"`     // /dev/sdb1 或 server:/export
	Fstype     string  `json:"fstype"`     // ext4 / xfs / tmpfs（老系统 df 不带 -T 时为空）
	TotalMB    int64   `json:"totalMB"`    // 总容量
	UsedMB     int64   `json:"usedMB"`     // 已用容量
	AvailMB    int64   `json:"availMB"`    // 可用容量
	Usage      float64 `json:"usage"`      // 使用率 0-100
	IsPseudo   bool    `json:"isPseudo"`   // tmpfs / overlay 等伪文件系统
	IsReadOnly bool    `json:"isReadOnly"` // ro 挂载（不参与"快满"预警）
}

// MonitorPoint 监控数据点
type MonitorPoint struct {
	ID        int     `json:"id"`
	ServerID  string  `json:"serverId"`
	CPUUsage  float64 `json:"cpuUsage"`
	MemTotal  int64   `json:"memTotal"`
	MemUsed   int64   `json:"memUsed"`
	MemUsage  float64 `json:"memUsage"`
	DiskTotal int64   `json:"diskTotal"`
	DiskUsed  int64   `json:"diskUsed"`
	DiskUsage float64 `json:"diskUsage"`
	// DiskJSON 多磁盘快照原文（仅用于落库，不随桥接重复传输：前端读 Disks）
	DiskJSON string `json:"-"`
	// Disks 多磁盘快照；旧数据只有单组磁盘字段时，由 store 回退成一条根分区数据
	Disks     []DiskInfo `json:"disks,omitempty"`
	NetRx     int64      `json:"netRx"`
	NetTx     int64      `json:"netTx"`
	Timestamp time.Time  `json:"timestamp"`
}

// CommandHistory 命令执行历史
type CommandHistory struct {
	ID         int64      `json:"id"`
	ServerID   string     `json:"serverId"`
	ServerName string     `json:"serverName"`
	Command    string     `json:"command"`
	Status     TaskStatus `json:"status"`
	ExitCode   int        `json:"exitCode"`
	Stdout     string     `json:"stdout"`
	Stderr     string     `json:"stderr"`
	Duration   int64      `json:"duration"` // milliseconds
	CreatedAt  time.Time  `json:"createdAt"`
	// Source 命令来源：manual（用户手敲/单个执行）、batch（批量执行）、ai（AI 接管执行）
	Source string `json:"source"`
	// AISessionID AI 接管会话 id（source=ai 时有值），用于追溯"AI 这一轮都干了什么"
	AISessionID string `json:"aiSessionId"`
}

// ---- 终端 AI ----

// Settings 应用设置
type Settings struct {
	MaxConcurrent  int    `json:"maxConcurrent"`
	UploadTimeout  int    `json:"uploadTimeout"`
	CommandTimeout int    `json:"commandTimeout"`
	Theme          string `json:"theme"`
}

// AISettings AI 配置。
// API Key 不随查询回传（只回 apiKeySet 告诉前端"已经存过一把"），
// 保存时留空表示"不改动已存的 Key"。
type AISettings struct {
	BaseURL      string `json:"baseURL"`
	APIKey       string `json:"apiKey"`
	APIKeySet    bool   `json:"apiKeySet"`
	Model        string `json:"model"`
	MaxRounds    int    `json:"maxRounds"`
	Timeout      int    `json:"timeout"`
	ContextLines int    `json:"contextLines"`
}

// AITestResult 测试连通结果
type AITestResult struct {
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
	LatencyMs int64  `json:"latencyMs"`
	Model     string `json:"model"`
	Endpoint  string `json:"endpoint"`
}

// AIContext 提供给 AI 的终端内容快照（UI 上要透明展示"到底把什么发出去了"）
type AIContext struct {
	TermID    string `json:"termId"`
	Mode      string `json:"mode"`
	Text      string `json:"text"`
	Lines     int    `json:"lines"`
	Bytes     int    `json:"bytes"`
	Truncated bool   `json:"truncated"` // 超出读取上限，头尾保留、中间省略
	Dropped   bool   `json:"dropped"`   // 起点已被环形缓冲丢弃（内容太久远）
	Elided    bool   `json:"elided"`    // 重复行折叠过
	SeqFrom   int64  `json:"seqFrom"`
	SeqTo     int64  `json:"seqTo"`
	Empty     bool   `json:"empty"`
}

// AICommand AI 给出的一条命令
type AICommand struct {
	Command string `json:"command"`
	Risk    string `json:"risk"`   // low | medium | high
	Reason  string `json:"reason"` // 风险说明（给用户看的）
}

// AIRequest 一次 AI 请求
type AIRequest struct {
	// SessionID 由前端生成：前端先订阅事件、再发起请求，避免极快失败时漏掉首个事件
	SessionID string `json:"sessionId"`
	TermID    string `json:"termId"`
	ServerID  string `json:"serverId"`
	Text      string `json:"text"`
	Mode      string `json:"mode"`    // suggest | exec | takeover
	Context   string `json:"context"` // selection | last50 | last200 | sinceLast | all
	Lines     int    `json:"lines"`
	Selection string `json:"selection"`
	MaxRounds int    `json:"maxRounds"`
	// ContinueSession 为真表示"在停下的任务上追加新要求"（复用已有上下文）
	ContinueSession bool `json:"continueSession"`
}

// AIResponse 建议 / 直接执行模式的一次回复
type AIResponse struct {
	SessionID string      `json:"sessionId"`
	Text      string      `json:"text"`
	Commands  []AICommand `json:"commands"`
	Context   AIContext   `json:"context"`
	Model     string      `json:"model"`
	ElapsedMs int64       `json:"elapsedMs"`
	Truncated bool        `json:"truncated"`
}

// AIExecResult 一条命令在终端里执行的结果
type AIExecResult struct {
	Command   string `json:"command"`
	ExitCode  int    `json:"exitCode"`
	Output    string `json:"output"`
	Truncated bool   `json:"truncated"`
	// Status done | blocked | timeout | interrupted | cancelled | error
	Status    string `json:"status"`
	Message   string `json:"message"`
	ElapsedMs int64  `json:"elapsedMs"`
	Risk      string `json:"risk"`
}

// AIStep 接管模式中的一轮（后端推事件、前端渲染成命令卡 + 结果卡）
type AIStep struct {
	Round   int           `json:"round"`
	Thought string        `json:"thought"`
	Command string        `json:"command"`
	Risk    string        `json:"risk"`
	State   string        `json:"state"` // running | done | failed | blocked
	Result  *AIExecResult `json:"result"`
}

// AIState 接管会话状态
type AIState struct {
	SessionID string `json:"sessionId"`
	TermID    string `json:"termId"`
	// State running | paused | stopped | done | error | maxrounds
	State string `json:"state"`
	// Kind 暂停原因分类，前端据此决定"要不要把键盘交还给用户"：
	//   input     等待输入（密码 / y-n 确认）→ 交还键盘与粘贴
	//   altscreen 进了全屏交互程序（vim/top）→ 交还键盘，让用户退出程序
	//   stall     长时间没有输出 → 保持锁定（命令还在跑，乱敲会搞乱）
	Kind      string `json:"kind"`
	Round     int    `json:"round"`
	MaxRounds int    `json:"maxRounds"`
	Message   string `json:"message"`
	At        int64  `json:"at"`
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
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Base64    string `json:"base64"`
	Encoding  string `json:"encoding"`
	HasBOM    bool   `json:"hasBOM"`
	EOL       string `json:"eol"`
	EOLMixed  bool   `json:"eolMixed"`
	Truncated bool   `json:"truncated"`
}

// TextFileWriteRequest 文本文件保存请求
type TextFileWriteRequest struct {
	Text     string `json:"text"`
	Encoding string `json:"encoding"`
	KeepBOM  bool   `json:"keepBOM"` // 显式为 true 才会写入 UTF-8 BOM
	EOL      string `json:"eol"`     // lf | crlf | cr
}

// ---- 运维工具 · 磁盘空间检测 ----
//
// 说明：本功能只做分析，不执行任何处置动作。DiskFinding.Command 是给用户复制到
// 终端里自行执行的建议命令，后端绝不代跑。

// DiskFinding 一条检测结论
type DiskFinding struct {
	Level    string `json:"level"`    // critical | warning | info | unavailable
	Kind     string `json:"kind"`     // inode | deleted-open | gap | journal | docker | bigfile | permission | pseudo | reserved
	Title    string `json:"title"`    // 一句话结论
	Detail   string `json:"detail"`   // 为什么、要注意什么
	Evidence string `json:"evidence"` // 原始输出片段（前端折叠展示）
	Command  string `json:"command"`  // 建议命令（只复制，不执行）
	// CommandNote 这条命令做什么、跑完会发生什么、有没有副作用。
	// 用户看不懂命令时，"复制走自己跑"是有风险的，所以凡是给了 Command 就必须给 CommandNote。
	CommandNote string `json:"commandNote"`
	SizeMB      int64  `json:"sizeMB"` // 相关体积，无则 0
}

// DiskDirItem 目录占用条目（含下钻结果）
type DiskDirItem struct {
	Path        string  `json:"path"`
	SizeMB      int64   `json:"sizeMB"`
	Percent     float64 `json:"percent"`     // 占该盘已用容量的百分比
	HasChildren bool    `json:"hasChildren"` // du -d1 的每一项都是目录，恒为 true
}

// DrillResult 一次目录下钻的结果。
// Partial 为真表示 du 在超时上限处被中断，Items 只是「已统计到的部分」——
// 必须让用户看到这一点，否则会把不完整的列表当成该目录的全部子项。
type DrillResult struct {
	Items   []DiskDirItem `json:"items"`
	Partial bool          `json:"partial"`
	Note    string        `json:"note"`
}

// DiskReport 一次磁盘检测的完整报告
type DiskReport struct {
	TaskID     string `json:"taskId"`
	ServerID   string `json:"serverId"`
	ServerName string `json:"serverName"`
	Mount      string `json:"mount"`
	Device     string `json:"device"`
	Fstype     string `json:"fstype"`

	TotalMB int64   `json:"totalMB"`
	UsedMB  int64   `json:"usedMB"`
	AvailMB int64   `json:"availMB"`
	Usage   float64 `json:"usage"`

	InodeTotal int64   `json:"inodeTotal"`
	InodeUsed  int64   `json:"inodeUsed"`
	InodeUsage float64 `json:"inodeUsage"`

	Findings []DiskFinding `json:"findings"`
	TopDirs  []DiskDirItem `json:"topDirs"`
	// BigFiles 专项 / 全盘大文件
	BigFiles []DiskDirItem `json:"bigFiles"`

	// ScannedMB 一级目录扫描合计；GapMB = UsedMB - ScannedMB，即"没被目录扫描解释掉"的部分。
	// 两个口径本来就不同（du 是 apparent/块占用），因此报告里不做"加起来等于已用"的暗示，
	// 差额本身反而是最有价值的诊断信号。
	ScannedMB int64 `json:"scannedMB"`
	GapMB     int64 `json:"gapMB"`

	// ReclaimableMB 报告中 critical/warning 项标注的体积合计（粗略口径：
	// docker 项只计可回收量，容器日志按大文件计一次，不会与 docker 项重复计算）
	ReclaimableMB int64 `json:"reclaimableMB"`

	// Partial 体积段未完成（超时 / 被取消 / du 不可用）
	Partial bool `json:"partial"`
	// Stage 当前阶段：quickscan | volume | done
	Stage     string   `json:"stage"`
	Notes     []string `json:"notes"`
	ElapsedMs int64    `json:"elapsedMs"`
	ScannedAt string   `json:"scannedAt"`
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
