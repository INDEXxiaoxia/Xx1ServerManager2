package main

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ServerManager2/internal/models"
	"ServerManager2/internal/sshx"
)

// 在线编辑相关的取值上限
const (
	textPeekBytes     = 8192            // 编码嗅探采样长度
	textMaxEditBytes  = 2 * 1024 * 1024 // 在线编辑文件大小上限 2MB
	dirSizeBatchLimit = 500             // 单次批量计算目录大小的数量上限
)

// fileSession 复用的 SFTP 连接会话
type fileSession struct {
	client   *sshx.Client
	lastUsed time.Time
}

// dialServer 新建一次性连接（调用方负责 Close）。
// 适用于上传/下载等长耗时、长连接语义明确的操作；
// 目录浏览、大小计算、搜索等高频小请求请使用 acquireFileClient 复用连接。
func (a *App) dialServer(serverID string) (*models.Server, *sshx.Client, error) {
	sv, err := a.getServerWithPassword(serverID)
	if err != nil {
		return nil, nil, err
	}
	c := sshx.New(sv.Host, sv.Port, sv.Username, sv.Password, 10)
	if err := c.Connect(); err != nil {
		return sv, nil, err
	}
	return sv, c, nil
}

// ---- 连接复用 ----

// acquireFileClient 获取（或重建）服务器对应的 SFTP 客户端。
// 文件管理相关操作请求密集，复用连接可以避免每次操作都重新握手。
func (a *App) acquireFileClient(serverID string) (*sshx.Client, error) {
	a.fileMu.Lock()
	defer a.fileMu.Unlock()

	if a.fileConns == nil {
		a.fileConns = make(map[string]*fileSession)
	}
	if sess, ok := a.fileConns[serverID]; ok && sess.client != nil {
		// 轻量健康探测：连接被远端回收时快速失败并重建
		if _, err := sess.client.StatPath("/"); err == nil {
			sess.lastUsed = time.Now()
			return sess.client, nil
		}
		sess.client.Close()
		delete(a.fileConns, serverID)
	}

	_, c, err := a.dialServer(serverID)
	if err != nil {
		return nil, err
	}
	a.fileConns[serverID] = &fileSession{client: c, lastUsed: time.Now()}
	return c, nil
}

// CloseFileClients 关闭全部复用连接（应用退出时调用）
func (a *App) CloseFileClients() {
	a.fileMu.Lock()
	defer a.fileMu.Unlock()
	for id, sess := range a.fileConns {
		if sess != nil && sess.client != nil {
			sess.client.Close()
		}
		delete(a.fileConns, id)
	}
}

// invalidateFileClient 主动丢弃某服务器的复用连接（服务器配置变更/删除时使用）
func (a *App) invalidateFileClient(serverID string) {
	a.fileMu.Lock()
	defer a.fileMu.Unlock()
	if sess, ok := a.fileConns[serverID]; ok {
		if sess.client != nil {
			sess.client.Close()
		}
		delete(a.fileConns, serverID)
	}
}

// ---- 本地文件对话框 ----

// SelectLocalFile 打开本地文件选择对话框，返回所选路径
func (a *App) SelectLocalFile() (string, error) {
	opts := runtime.OpenDialogOptions{Title: "选择本地文件"}
	return runtime.OpenFileDialog(a.ctx, opts)
}

// SelectLocalSavePath 打开本地保存对话框，返回保存路径
func (a *App) SelectLocalSavePath(defaultName string) (string, error) {
	opts := runtime.SaveDialogOptions{Title: "保存到本地", DefaultFilename: defaultName}
	return runtime.SaveFileDialog(a.ctx, opts)
}

// ---- 目录浏览 ----

// ListRemoteDir 列出远程目录
func (a *App) ListRemoteDir(serverID, path string) ([]models.FileEntry, error) {
	c, err := a.acquireFileClient(serverID)
	if err != nil {
		return nil, err
	}
	dir := cleanRemotePath(path)
	entries, err := c.ListDir(dir)
	if err != nil {
		return nil, fmt.Errorf("%s", describePathError(dir, err))
	}
	return entries, nil
}

// ResolveRemotePath 把用户输入解析为规范化绝对路径。
// 支持 ~（当前用户 home）、~user（指定用户 home）、相对路径（以 home 为基准）。
func (a *App) ResolveRemotePath(serverID, rawPath string) (string, error) {
	c, err := a.acquireFileClient(serverID)
	if err != nil {
		return "", err
	}
	raw := strings.TrimSpace(rawPath)
	if raw == "" {
		return "", fmt.Errorf("请输入目录路径")
	}
	raw = strings.ReplaceAll(raw, "\\", "/")

	home := strings.TrimSpace(a.remoteBase(c))
	if home == "" {
		home = "/"
	}

	switch {
	case raw == "~":
		return cleanRemotePath(home), nil
	case strings.HasPrefix(raw, "~/"):
		return remoteAbs(home, strings.TrimPrefix(raw, "~/")), nil
	case strings.HasPrefix(raw, "~"):
		// ~user：通过远端 shell 展开（SFTP 不识别 ~）
		user := raw[1:]
		if idx := strings.Index(user, "/"); idx >= 0 {
			user = user[:idx]
		}
		rest := strings.TrimPrefix(raw[1+len(user):], "/")
		code, out, stderr := c.ExecCommand("LC_ALL=C cd ~"+shellSafeWord(user)+" 2>/dev/null && pwd", 15, false)
		dir := strings.TrimSpace(out)
		if code != 0 || dir == "" || !strings.HasPrefix(dir, "/") {
			return "", fmt.Errorf("无法解析用户主目录 ~%s：%s", user, strings.TrimSpace(stderr))
		}
		return remoteAbs(dir, rest), nil
	case strings.HasPrefix(raw, "/"):
		return cleanRemotePath(raw), nil
	default:
		return remoteAbs(home, raw), nil
	}
}

// ValidateRemoteDir 校验路径是否为可访问目录，供顶部目录栏跳转前检查。
// 路径不存在、是文件、无权限都会返回明确错误，前端据此提示且不改变当前目录。
func (a *App) ValidateRemoteDir(serverID, remotePath string) (map[string]any, error) {
	c, err := a.acquireFileClient(serverID)
	if err != nil {
		return nil, err
	}
	dir := cleanRemotePath(remotePath)
	info, err := c.StatPath(dir)
	if err != nil {
		return nil, fmt.Errorf("%s", describePathError(dir, err))
	}
	isSymlink := c.IsSymlink(dir)
	if !info.IsDir() {
		if isSymlink {
			return nil, fmt.Errorf("该路径是符号链接指向的文件，不是目录：%s", dir)
		}
		return nil, fmt.Errorf("该路径是文件，不是目录：%s", dir)
	}
	result := map[string]any{
		"ok":        true,
		"path":      dir,
		"isDir":     true,
		"isSymlink": isSymlink,
		"mode":      info.Mode().String(),
		"name":      baseNameRemote(dir),
		"modTime":   info.ModTime().Format("2006-01-02 15:04:05"),
	}
	if isSymlink {
		result["message"] = "该路径是符号链接指向的目录"
	}
	// 顺带返回父目录，方便前端实现「上级目录」
	result["parent"] = dirNameRemote(dir)
	return result, nil
}

// MakeRemoteDir 创建远程目录
func (a *App) MakeRemoteDir(serverID, path string) error {
	c, err := a.acquireFileClient(serverID)
	if err != nil {
		return err
	}
	return c.MakeDirs(cleanRemotePath(path))
}

// RenameRemote 重命名/移动远程文件
func (a *App) RenameRemote(serverID, oldPath, newPath string) error {
	c, err := a.acquireFileClient(serverID)
	if err != nil {
		return err
	}
	return c.Rename(cleanRemotePath(oldPath), cleanRemotePath(newPath))
}

// DeleteRemote 删除远程文件或目录
func (a *App) DeleteRemote(serverID, path string, isDir bool) error {
	c, err := a.acquireFileClient(serverID)
	if err != nil {
		return err
	}
	p := cleanRemotePath(path)
	if isDir {
		return c.RemoveDir(p)
	}
	return c.Remove(p)
}

// ChmodRemote 修改远程文件权限
func (a *App) ChmodRemote(serverID, path string, mode uint32) error {
	c, err := a.acquireFileClient(serverID)
	if err != nil {
		return err
	}
	return c.Chmod(cleanRemotePath(path), mode)
}

// ---- 目录大小计算 ----

// GetRemoteDirSizes 批量计算目录大小。
// 每个目录独立成败（单条失败不影响其他条），并在计算过程中逐条推送 file:dirsize 事件，
// 前端据此实现「点击计算 → 转圈 → 显示大小」的逐行反馈与总进度。
func (a *App) GetRemoteDirSizes(serverID string, paths []string) ([]models.DirSizeItem, error) {
	if len(paths) == 0 {
		return []models.DirSizeItem{}, nil
	}
	if len(paths) > dirSizeBatchLimit {
		paths = paths[:dirSizeBatchLimit]
	}
	c, err := a.acquireFileClient(serverID)
	if err != nil {
		return nil, err
	}

	results := make([]models.DirSizeItem, 0, len(paths))
	for i, raw := range paths {
		dir := cleanRemotePath(raw)
		item := models.DirSizeItem{Path: dir}
		size, method, err := c.DirSize(dir)
		if err != nil {
			item.Error = err.Error()
		} else {
			item.Size = size
			item.Method = method
		}
		results = append(results, item)
		runtime.EventsEmit(a.ctx, "file:dirsize", map[string]any{
			"serverId": serverID, "path": dir, "size": item.Size, "method": item.Method,
			"error": item.Error, "done": i + 1, "total": len(paths),
		})
	}
	return results, nil
}

// ---- 文件搜索 ----

// SearchRemoteDir 在当前目录（或其子目录）按文件名模糊搜索。
// recursive=true 时递归搜索子目录，受深度 8 层 / 500 条结果 / 30s 上限保护。
func (a *App) SearchRemoteDir(serverID, root, keyword string, recursive bool) (models.SearchResult, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return models.SearchResult{}, fmt.Errorf("请输入搜索关键字")
	}
	c, err := a.acquireFileClient(serverID)
	if err != nil {
		return models.SearchResult{}, err
	}
	dir := cleanRemotePath(root)
	start := time.Now()
	items, truncated, scanned, err := c.SearchDir(dir, keyword, recursive, 0, 0)
	if err != nil {
		return models.SearchResult{}, fmt.Errorf("%s", describePathError(dir, err))
	}
	if items == nil {
		items = []models.FileEntry{}
	}
	return models.SearchResult{
		Root:      dir,
		Keyword:   keyword,
		Recursive: recursive,
		Items:     items,
		Truncated: truncated,
		Scanned:   scanned,
		ElapsedMs: time.Since(start).Milliseconds(),
	}, nil
}

// ---- 文本文件在线编辑 ----

// DetectRemoteEncoding 探测文本文件编码/二进制/换行风格（只读取文件头部，不加载全文）
func (a *App) DetectRemoteEncoding(serverID, remotePath string) (models.TextFileMeta, error) {
	c, err := a.acquireFileClient(serverID)
	if err != nil {
		return models.TextFileMeta{}, err
	}
	p := cleanRemotePath(remotePath)
	info, err := c.StatPath(p)
	if err != nil {
		return models.TextFileMeta{}, fmt.Errorf("%s", describePathError(p, err))
	}
	if info.IsDir() {
		return models.TextFileMeta{}, fmt.Errorf("目录不支持在线编辑：%s", p)
	}

	head, err := c.Peek(p, textPeekBytes)
	if err != nil {
		return models.TextFileMeta{}, fmt.Errorf("读取文件失败：%v", err)
	}
	cand := detectTextCandidate(head)
	eol, mixed := detectEOL(head)

	meta := models.TextFileMeta{
		Path:     p,
		Name:     baseNameRemote(p),
		Size:     info.Size(),
		Encoding: cand.Encoding,
		HasBOM:   cand.HasBOM,
		Binary:   cand.Binary,
		EOL:      eol,
		EOLMixed: mixed,
		Editable: true,
	}
	if cand.Binary {
		meta.Editable = false
		meta.Message = cand.Reason
		return meta, nil
	}
	if cand.Reason != "" {
		meta.Editable = false
		meta.Message = cand.Reason
		return meta, nil
	}
	if info.Size() > textMaxEditBytes {
		meta.Editable = false
		meta.Message = fmt.Sprintf("文件大小 %.1f MB 超过在线编辑上限 %.0f MB，请下载到本地编辑后上传",
			float64(info.Size())/1048576, float64(textMaxEditBytes)/1048576)
		return meta, nil
	}
	return meta, nil
}

// ReadRemoteTextFile 读取文本文件内容。
// 返回原文 base64：Go 的 string 经 JSON 桥传输非 UTF-8 字节会被替换为 U+FFFD，
// 直接返回解码后的文本会损坏 GB18030 等编码的文件。
func (a *App) ReadRemoteTextFile(serverID, remotePath, encName string) (models.TextFileContent, error) {
	c, err := a.acquireFileClient(serverID)
	if err != nil {
		return models.TextFileContent{}, err
	}
	p := cleanRemotePath(remotePath)
	info, err := c.StatPath(p)
	if err != nil {
		return models.TextFileContent{}, fmt.Errorf("%s", describePathError(p, err))
	}
	if info.IsDir() {
		return models.TextFileContent{}, fmt.Errorf("目录不支持在线编辑：%s", p)
	}

	data, err := c.ReadAll(p, info.Size(), textMaxEditBytes)
	if err != nil {
		return models.TextFileContent{}, err
	}

	enc := normalizedEncoding(encName)
	// 未显式指定编码时按内容自动识别（双保险：前端 DetectRemoteEncoding 已给出建议值）
	if strings.TrimSpace(encName) == "" {
		cand := detectTextCandidate(data)
		if cand.Binary {
			return models.TextFileContent{}, fmt.Errorf("%s", cand.Reason)
		}
		enc = cand.Encoding
	}
	hasBOM := detectHasBOM(enc, data)
	// base64 一律返回不含 BOM 的正文，BOM 通过 hasBOM 字段显式传递，
	// 避免前端解码时把 BOM 当成正文首字符（也便于用户选择移除 BOM）。
	body := stripBOM(data)

	eol, mixed := detectEOL(body)
	return models.TextFileContent{
		Path:      p,
		Size:      info.Size(),
		Base64:    base64.StdEncoding.EncodeToString(body),
		Encoding:  enc,
		HasBOM:    hasBOM,
		EOL:       eol,
		EOLMixed:  mixed,
		Truncated: false,
	}, nil
}

// WriteRemoteTextFile 保存文本文件。
// 关键约束：只有 keepBOM 显式为 true 时才会写入 UTF-8 BOM，
// 未带 BOM 的文件保存后绝不会凭空多出 windows BOM。
func (a *App) WriteRemoteTextFile(serverID, remotePath string, req models.TextFileWriteRequest) error {
	c, err := a.acquireFileClient(serverID)
	if err != nil {
		return err
	}
	p := cleanRemotePath(remotePath)
	info, err := c.StatPath(p)
	if err != nil {
		return fmt.Errorf("%s", describePathError(p, err))
	}
	if info.IsDir() {
		return fmt.Errorf("目录不支持写入：%s", p)
	}

	enc := normalizedEncoding(req.Encoding)
	eol := strings.ToLower(strings.TrimSpace(req.EOL))
	switch eol {
	case eolLF, eolCRLF, eolCR:
	default:
		eol = eolLF
	}

	data, err := buildTextBytes(req.Text, enc, req.KeepBOM, eol)
	if err != nil {
		return err
	}
	if err := c.WriteFileBytes(p, data, info.Mode()); err != nil {
		return fmt.Errorf("保存失败：%v", err)
	}
	return nil
}

// ---- 上传/下载 ----

// UploadToServer 上传单个文件到服务器（带进度事件）
func (a *App) UploadToServer(serverID, localPath, remotePath string) error {
	_, c, err := a.dialServer(serverID)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Upload(localPath, remotePath, func(transferred, total int64) {
		pct := 0.0
		if total > 0 {
			pct = float64(int(transferred*10000/total)) / 100
		}
		runtime.EventsEmit(a.ctx, "file:progress", map[string]any{
			"serverId": serverID, "localPath": localPath, "remotePath": remotePath,
			"direction": "upload", "progress": pct, "transferred": transferred, "total": total,
		})
	})
}

// DownloadFromServer 从服务器下载单个文件（带进度事件）
func (a *App) DownloadFromServer(serverID, remotePath, localPath string) error {
	_, c, err := a.dialServer(serverID)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Download(remotePath, localPath, func(transferred, total int64) {
		pct := 0.0
		if total > 0 {
			pct = float64(int(transferred*10000/total)) / 100
		}
		runtime.EventsEmit(a.ctx, "file:progress", map[string]any{
			"serverId": serverID, "localPath": localPath, "remotePath": remotePath,
			"direction": "download", "progress": pct, "transferred": transferred, "total": total,
		})
	})
}

// GetRemoteFileInfo 获取远程文件信息（用于下载默认名）
func (a *App) GetRemoteFileInfo(serverID, remotePath string) (map[string]any, error) {
	c, err := a.acquireFileClient(serverID)
	if err != nil {
		return nil, err
	}
	p := cleanRemotePath(remotePath)
	parent := dirNameRemote(p)
	name := baseNameRemote(p)
	entries, err := c.ListDir(parent)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Name == name {
			return map[string]any{"name": e.Name, "size": e.Size, "modTime": e.ModTime}, nil
		}
	}
	return map[string]any{"name": name, "size": int64(0), "modTime": ""}, nil
}

// ---- 内部辅助 ----

// remoteBase 获取远端会话的基准目录。
// 优先用 SFTP 的 Getwd（即会话工作目录，相对路径正是相对它解析），
// 拿不到时再回落 shell 的 home 目录。FileZilla 等客户端也采用同样口径。
func (a *App) remoteBase(c *sshx.Client) string {
	if c == nil {
		return "/"
	}
	if dir, err := c.HomeDir(); err == nil && strings.HasPrefix(dir, "/") {
		return dir
	}
	return "/"
}

// detectHasBOM 判断数据是否以对应编码的 BOM 开头
func detectHasBOM(enc string, data []byte) bool {
	switch enc {
	case encUTF8:
		return len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF
	case encUTF16LE:
		return len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE
	case encUTF16BE:
		return len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF
	}
	return false
}

// stripBOM 去掉数据开头的 BOM
func stripBOM(data []byte) []byte {
	switch {
	case len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF:
		return data[3:]
	case len(data) >= 2 && (data[0] == 0xFF && data[1] == 0xFE || data[0] == 0xFE && data[1] == 0xFF):
		return data[2:]
	}
	return data
}

// shellSafeWord 仅保留安全字符，用于 ~user 这类必须拼接的片段
func shellSafeWord(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// 保持 os 包引用（WriteFileBytes 依赖 os.FileMode，权限位来自远端 Stat）