package sshx

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/pkg/sftp"

	"ServerManager2/internal/models"
)

// Client 为语言冲突与 ssh 包区分，SSH 客户端封装
type Client struct {
	Host     string
	Port     int
	Username string
	Password string
	Timeout  time.Duration

	client *ssh.Client
	sftp   *sftp.Client
}

// New 创建 SSH 客户端
func New(host string, port int, username, password string, timeout int) *Client {
	if timeout <= 0 {
		timeout = 10
	}
	return &Client{Host: host, Port: port, Username: username, Password: password, Timeout: time.Duration(timeout) * time.Second}
}

func (c *Client) sshConfig() *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User:            c.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(c.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         c.Timeout,
	}
}

// Connect 建立连接
func (c *Client) Connect() error {
	addr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	client, err := ssh.Dial("tcp", addr, c.sshConfig())
	if err != nil {
		return err
	}
	c.client = client
	return nil
}

// Close 关闭连接
func (c *Client) Close() {
	if c.sftp != nil {
		c.sftp.Close()
		c.sftp = nil
	}
	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
}

// TestConnection 测试连接
func (c *Client) TestConnection() (bool, string) {
	if err := c.Connect(); err != nil {
		return false, errMessage(err)
	}
	c.Close()
	return true, "连接成功"
}

func errMessage(err error) string {
	switch {
	case strings.Contains(err.Error(), "unable to authenticate"):
		return "认证失败：用户名或密码错误"
	case strings.Contains(err.Error(), "timeout"):
		return "连接超时"
	default:
		return "连接失败：" + err.Error()
	}
}

func (c *Client) session(timeout int) (*ssh.Session, error) {
	if c.client == nil {
		return nil, errors.New("SSH未连接")
	}
	sess, err := c.client.NewSession()
	if err != nil {
		return nil, err
	}
	if timeout > 0 {
		sess.Setenv("LANG", "C")
	}
	return sess, nil
}

// ExecCommand 执行命令，返回(退出码, 标准输出, 错误输出)
func (c *Client) ExecCommand(command string, timeout int, useSudo bool) (int, string, string) {
	if c.client == nil {
		return -1, "", "SSH未连接"
	}
	sess, err := c.client.NewSession()
	if err != nil {
		return -1, "", err.Error()
	}
	defer sess.Close()

	cmd := command
	if useSudo && c.Username != "root" {
		cmd = buildSudoCommand(command, c.Password)
	}

	if timeout > 0 {
		done := make(chan struct{})
		go func() {
			select {
			case <-done:
			case <-time.After(time.Duration(timeout)*time.Second + 2*time.Second):
				sess.Close()
			}
		}()
		defer close(done)
	}

	var stdout, stderr strings.Builder
	sess.Stdout = &stdout
	sess.Stderr = &stderr
	if err := sess.Run(cmd); err != nil {
		exitErr, ok := err.(*ssh.ExitError)
		if ok {
			return exitErr.ExitStatus(), stdout.String(), cleanSudoErr(stderr.String(), c.Username)
		}
		return -1, stdout.String(), err.Error()
	}
	return 0, stdout.String(), cleanSudoErr(stderr.String(), c.Username)
}

func buildSudoCommand(command, password string) string {
	lines := strings.Split(command, "\n")
	var out []string
	for _, line := range lines {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "#") {
			if l != "" {
				out = append(out, l)
			}
			continue
		}
		if strings.HasPrefix(l, "sudo ") {
			out = append(out, fmt.Sprintf("echo '%s' | sudo -S %s", password, strings.TrimPrefix(l, "sudo ")))
		} else {
			out = append(out, fmt.Sprintf("echo '%s' | sudo -S %s", password, l))
		}
	}
	return strings.Join(out, "\n")
}

func cleanSudoErr(stderr, username string) string {
	var lines []string
	for _, l := range strings.Split(stderr, "\n") {
		ls := strings.TrimSpace(l)
		if strings.HasPrefix(ls, "[sudo]") || strings.Contains(strings.ToLower(ls), "password for "+username) {
			continue
		}
		lines = append(lines, l)
	}
	return strings.Join(lines, "\n")
}

// ---- SFTP ----

func (c *Client) sftpClient() (*sftp.Client, error) {
	if c.client == nil {
		return nil, errors.New("SSH未连接")
	}
	if c.sftp == nil {
		sc, err := sftp.NewClient(c.client)
		if err != nil {
			return nil, err
		}
		c.sftp = sc
	}
	return c.sftp, nil
}

// ListDir 列出远程目录
func (c *Client) ListDir(path string) ([]models.FileEntry, error) {
	sc, err := c.sftpClient()
	if err != nil {
		return nil, err
	}
	infos, err := sc.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var entries []models.FileEntry
	for _, info := range infos {
		entries = append(entries, models.FileEntry{
			Name:    info.Name(),
			Path:    strings.TrimSuffix(path, "/") + "/" + info.Name(),
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}
	return entries, nil
}

// HomeDir 获取远端 home 目录（优先使用 SFTP 的 realpath 语义，失败则回落 shell）
func (c *Client) HomeDir() (string, error) {
	if sc, err := c.sftpClient(); err == nil {
		if dir, err := sc.Getwd(); err == nil && strings.HasPrefix(dir, "/") {
			return dir, nil
		}
	}
	code, out, stderr := c.ExecCommand("LC_ALL=C cd ~ && pwd", 15, false)
	if code != 0 {
		return "", errors.New(firstLine(stderr))
	}
	dir := strings.TrimSpace(out)
	if !strings.HasPrefix(dir, "/") {
		return "", errors.New("无法获取远端 home 目录")
	}
	return dir, nil
}

// StatPath 获取远程路径信息（不存在或不可访问时返回错误）
func (c *Client) StatPath(path string) (os.FileInfo, error) {
	sc, err := c.sftpClient()
	if err != nil {
		return nil, err
	}
	return sc.Stat(path)
}

// IsSymlink 判断路径是否为符号链接（用于避免目录栏把软链接误判为普通目录/文件）
func (c *Client) IsSymlink(path string) bool {
	sc, err := c.sftpClient()
	if err != nil {
		return false
	}
	info, err := sc.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

// Peek 读取文件头部最多 maxBytes 字节（用于编码/二进制嗅探，不加载整个大文件）
func (c *Client) Peek(path string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = 8192
	}
	sc, err := c.sftpClient()
	if err != nil {
		return nil, err
	}
	rf, err := sc.Open(path)
	if err != nil {
		return nil, err
	}
	defer rf.Close()
	buf := make([]byte, maxBytes)
	n, err := io.ReadFull(rf, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buf[:n], nil
}

// ReadAll 一次性读取整个文件，超过 maxBytes 返回错误（避免把巨型文件读进内存）
// sizeHint 为远端 Stat 报告的大小，用于第一次读取前的快速拒绝
func (c *Client) ReadAll(path string, sizeHint, maxBytes int64) ([]byte, error) {
	if maxBytes > 0 && sizeHint > maxBytes {
		return nil, fmt.Errorf("文件过大（%.1f MB），超过在线编辑上限 %.1f MB", float64(sizeHint)/1048576, float64(maxBytes)/1048576)
	}
	sc, err := c.sftpClient()
	if err != nil {
		return nil, err
	}
	rf, err := sc.Open(path)
	if err != nil {
		return nil, err
	}
	defer rf.Close()

	limit := maxBytes
	if limit <= 0 {
		limit = sizeHint + 1024
	}
	if limit <= 0 {
		limit = 4 * 1024 * 1024
	}
	data, err := io.ReadAll(io.LimitReader(rf, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		if maxBytes > 0 {
			return nil, fmt.Errorf("文件过大（超过 %.1f MB），已取消在线编辑", float64(maxBytes)/1048576)
		}
		return nil, errors.New("文件在读取过程中变大，已取消操作")
	}
	return data, nil
}

// WriteFileBytes 覆盖写入远程文件，并保持原有权限位。
// 使用 OpenFile(WRONLY|CREATE|TRUNC) 而不是 Create()：
// Create() 依赖远端默认 umask（可能产生 0666），且会丢失权限位。
func (c *Client) WriteFileBytes(path string, data []byte, mode os.FileMode) error {
	sc, err := c.sftpClient()
	if err != nil {
		return err
	}
	f, err := sc.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	// 显式 Flush 后关闭，确保数据落盘再改权限
	if err := f.Close(); err != nil {
		return err
	}
	if mode != 0 {
		if err := sc.Chmod(path, mode.Perm()); err != nil {
			return fmt.Errorf("文件已写入，但恢复权限 %s 失败: %w", mode.Perm().String(), err)
		}
	}
	return nil
}

// DirSizeMethod 目录大小计算方式
const (
	DirSizeMethodDu   = "du"
	DirSizeMethodWalk = "walk"
)

// dirSizeLimits SFTP 回退遍历的限制，防止对超大目录（如 /）无限扫描
const (
	dirSizeMaxEntries = 200000
	dirSizeMaxElapsed = 60 * time.Second

	// 搜索上限：深度 8 层、结果 500 条、耗时 30s
	searchMaxDepth   = 8
	searchMaxResults = 500
	searchMaxElapsed = 30 * time.Second
)

// DirSize 计算目录占用的显式大小（apparent size，与文件 Stat().Size() 口径一致）。
// 优先使用远端 du（快），不可用时回退为 SFTP 递归遍历并由调用方感知 method=walk。
func (c *Client) DirSize(path string) (int64, string, error) {
	if size, err := c.dirSizeByDu(path); err == nil {
		return size, DirSizeMethodDu, nil
	} else {
		walkSize, walkErr := c.dirSizeByWalk(path)
		if walkErr == nil {
			return walkSize, DirSizeMethodWalk, nil
		}
		return 0, "", fmt.Errorf("du 与遍历方式均失败：%v；%v", err, walkErr)
	}
}

// dirSizeByDu 使用远端 du 计算目录大小。
// -b/--bytes 为 GNU 扩展；老版本 coreutils 与 BusyBox 参数不兼容时逐个回退。
func (c *Client) dirSizeByDu(path string) (int64, error) {
	variants := [][]string{
		{"du", "-sbP", path},
		{"du", "-s", "--block-size=1", "-P", path},
		{"du", "-sk", path}, // 最后退化为 KB 精度（BusyBox）
	}
	var lastErr error
	for i, argv := range variants {
		cmd := fmt.Sprintf("LC_ALL=C %s 2>&1", shellJoin(argv))
		code, out, _ := c.ExecCommand(cmd, 60, false)
		if code != 0 {
			lastErr = fmt.Errorf("du 执行失败: %s", firstLine(out))
			continue
		}
		size, ok := parseDuSize(out)
		if !ok {
			lastErr = fmt.Errorf("du 输出无法解析: %s", firstLine(out))
			continue
		}
		if i == 2 {
			size *= 1024 // -k 为 KB 精度
		}
		return size, nil
	}
	if lastErr == nil {
		lastErr = errors.New("du 不可用")
	}
	return 0, lastErr
}

// parseDuSize 解析 du 输出，取第一个字段为字节数。
// 不使用空格切分整行：路径可能包含空格、中文甚至换行，只信任首个字段。
func parseDuSize(out string) (int64, bool) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimLeft(line, " \t")
		if line == "" {
			continue
		}
		field := line
		if idx := strings.IndexAny(line, " \t"); idx >= 0 {
			field = line[:idx]
		}
		if v, err := strconv.ParseInt(field, 10, 64); err == nil && v >= 0 {
			return v, true
		}
	}
	return 0, false
}

// dirSizeByWalk SFTP 递归遍历兜底方案：
// 跳过符号链接（防止循环与越权），并对条目数与耗时设上限。
func (c *Client) dirSizeByWalk(root string) (int64, error) {
	sc, err := c.sftpClient()
	if err != nil {
		return 0, err
	}
	start := time.Now()
	var total int64
	var count int
	reachedLimit := errors.New("目录过大，遍历已达上限")

	w := sc.Walk(root)
	for w.Step() {
		// 单个子目录无权限时跳过，但根目录本身不可读需要报错
		if err := w.Err(); err != nil {
			if w.Path() == root {
				return 0, err
			}
		} else if info := w.Stat(); info != nil && !info.IsDir() {
			count++
			if count > dirSizeMaxEntries {
				return 0, reachedLimit
			}
			total += info.Size()
		}
		if time.Since(start) > dirSizeMaxElapsed {
			return 0, errors.New("目录过大，遍历超时（60s）")
		}
	}
	return total, nil
}

// SearchDir 按文件名在远端搜索。
// recursive 为假时只扫描 root 这一层（目录很大也很快）；
// 为真时递归子目录，并受 maxDepth / maxResults / 30s 超时三重限制。
// 返回的 truncated 表示因达到上限而结果不完整。
func (c *Client) SearchDir(root, keyword string, recursive bool, maxDepth, maxResults int) ([]models.FileEntry, bool, int, error) {
	sc, err := c.sftpClient()
	if err != nil {
		return nil, false, 0, err
	}
	if maxDepth <= 0 {
		maxDepth = searchMaxDepth
	}
	if maxResults <= 0 {
		maxResults = searchMaxResults
	}
	start := time.Now()
	var out []models.FileEntry
	truncated := false
	scanned := 0

	appendEntry := func(path string, info os.FileInfo) bool {
		scanned++
		if MatchFileName(keyword, info.Name()) {
			out = append(out, models.FileEntry{
				Name:    info.Name(),
				Path:    path,
				IsDir:   info.IsDir(),
				Size:    info.Size(),
				Mode:    info.Mode().String(),
				ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			})
			if len(out) >= maxResults {
				truncated = true
				return false
			}
		}
		return true
	}

	if !recursive {
		infos, err := sc.ReadDir(root)
		if err != nil {
			return nil, false, 0, err
		}
		for _, info := range infos {
			if !appendEntry(joinRemote(root, info.Name()), info) {
				break
			}
		}
		return out, truncated, scanned, nil
	}

	rootDepth := strings.Count(strings.Trim(root, "/"), "/")
	w := sc.Walk(root)
	for w.Step() {
		if err := w.Err(); err != nil {
			// 单个子目录无权限时跳过，但根目录本身不可读需要报错
			if w.Path() == root {
				return nil, false, scanned, err
			}
			continue
		}
		// kr/fs 的 Walker 基于 Lstat，不会跟随符号链接；
		// 且 Stat() 取的是目录项本身而非目标，因此不会走进链接目录。
		info := w.Stat()
		if info == nil {
			continue
		}
		if info.IsDir() {
			depth := strings.Count(strings.Trim(w.Path(), "/"), "/") - rootDepth
			if depth >= maxDepth {
				w.SkipDir()
			}
			continue
		}
		if !appendEntry(w.Path(), info) {
			break
		}
		if time.Since(start) > searchMaxElapsed {
			truncated = true
			break
		}
	}
	return out, truncated, scanned, nil
}

// MatchFileName 文件名模糊匹配：大小写不敏感的子串匹配，
// 关键字中的 * 与 ? 按通配符处理，[] 一律按字面量处理（避免字符类注入）。
func MatchFileName(keyword, name string) bool {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return false
	}
	lowName := strings.ToLower(name)
	lowKw := strings.ToLower(keyword)
	if strings.Contains(lowName, lowKw) {
		return true
	}
	if !strings.ContainsAny(keyword, "*?") {
		return false
	}
	// 先转义字符类元字符，再交给 filepath.Match
	pattern := strings.NewReplacer("[", "[[]", "]", "[]]").Replace(lowKw)
	if ok, err := filepath.Match("*"+pattern+"*", lowName); err == nil && ok {
		return true
	}
	return false
}

// joinRemote 拼接远程路径，避免出现 // 与丢失根目录
func joinRemote(dir, name string) string {
	if strings.HasSuffix(dir, "/") || dir == "" {
		return dir + name
	}
	return dir + "/" + name
}

// ---- 远端 shell ----

// shellQuote 单引号包裹参数，兼容空格、中文、$ 与换行
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellJoin 将 argv 拼接为安全的命令行
func shellJoin(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, a := range argv {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// MakeDirs 递归创建远程目录
func (c *Client) MakeDirs(path string) error {
	sc, err := c.sftpClient()
	if err != nil {
		return err
	}
	return sc.MkdirAll(path)
}

// Rename 重命名/移动
func (c *Client) Rename(oldPath, newPath string) error {
	sc, err := c.sftpClient()
	if err != nil {
		return err
	}
	return sc.Rename(oldPath, newPath)
}

// Remove 删除文件或空目录
func (c *Client) Remove(path string) error {
	sc, err := c.sftpClient()
	if err != nil {
		return err
	}
	return sc.Remove(path)
}

// RemoveDir 删除目录（包含内容时使用 Recursive）
func (c *Client) RemoveDir(path string) error {
	sc, err := c.sftpClient()
	if err != nil {
		return err
	}
	return sc.RemoveDirectory(path)
}

// Chmod 修改权限
func (c *Client) Chmod(path string, mode uint32) error {
	sc, err := c.sftpClient()
	if err != nil {
		return err
	}
	return sc.Chmod(path, os.FileMode(mode))
}

// uploadAll 将本地文件写入远程 writer，带进度回调
func uploadAll(w io.Writer, lf *os.File, total int64, progress func(transferred, total int64)) error {
	buf := make([]byte, 32*1024)
	var transferred int64
	for {
		n, rerr := lf.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			transferred += int64(n)
			if progress != nil {
				progress(transferred, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	return nil
}

// uploadWithSudo 目标目录无写权限时回退为 sudo 上传（同初版逻辑）：
// 先传到 /tmp，再用 sudo mkdir -p + mv + chmod 移动到目标位置
func (c *Client) uploadWithSudo(sc *sftp.Client, lf *os.File, total int64, remotePath string, progress func(transferred, total int64)) error {
	basename := filepath.Base(remotePath)
	tmp := fmt.Sprintf("/tmp/%s.%d", basename, time.Now().UnixNano())

	rf, err := sc.Create(tmp)
	if err != nil {
		return fmt.Errorf("上传临时文件失败: %w", err)
	}
	if _, err := lf.Seek(0, io.SeekStart); err != nil {
		rf.Close()
		_ = sc.Remove(tmp)
		return err
	}
	werr := uploadAll(rf, lf, total, progress)
	rf.Close()
	if werr != nil {
		_ = sc.Remove(tmp)
		return werr
	}

	dir := filepath.ToSlash(filepath.Dir(remotePath))
	cmds := []string{
		fmt.Sprintf("mkdir -p '%s'", dir),
		fmt.Sprintf("mv '%s' '%s'", tmp, remotePath),
		fmt.Sprintf("chmod 644 '%s'", remotePath),
	}
	for _, cmd := range cmds {
		code, _, stderr := c.ExecCommand(cmd, 60, true)
		if code != 0 {
			_, _, _ = c.ExecCommand(fmt.Sprintf("rm -f '%s'", tmp), 30, true)
			return fmt.Errorf("%s 失败: %s", cmd, strings.TrimSpace(stderr))
		}
	}
	return nil
}

// Upload 上传文件，带进度回调(transferred, total)。
// 目标目录不可写（如 /opt 下需要 root）时自动回退 sudo 上传，
// 避免直接 sc.Create 报 permission denied。
func (c *Client) Upload(localPath, remotePath string, progress func(transferred, total int64)) error {
	sc, err := c.sftpClient()
	if err != nil {
		return err
	}
	lf, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer lf.Close()
	stat, err := lf.Stat()
	if err != nil {
		return err
	}
	total := stat.Size()
	dir := filepath.ToSlash(filepath.Dir(remotePath))
	if dir != "." && dir != "/" && dir != "" {
		_ = sc.MkdirAll(dir)
	}

	// 先尝试直接上传
	rf, err := sc.Create(remotePath)
	if err == nil {
		werr := uploadAll(rf, lf, total, progress)
		rf.Close()
		if werr == nil {
			return nil
		}
		_ = sc.Remove(remotePath)
	}

	// 失败（常见 permission denied），非 root 用户回退 sudo 上传
	if c.Username == "root" {
		return err
	}
	return c.uploadWithSudo(sc, lf, total, remotePath, progress)
}

// Download 下载文件，带进度回调
func (c *Client) Download(remotePath, localPath string, progress func(transferred, total int64)) error {
	sc, err := c.sftpClient()
	if err != nil {
		return err
	}
	rf, err := sc.Open(remotePath)
	if err != nil {
		return err
	}
	defer rf.Close()
	stat, err := rf.Stat()
	if err != nil {
		return err
	}
	total := stat.Size()
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	lf, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer lf.Close()
	if progress != nil {
		buf := make([]byte, 32*1024)
		var transferred int64
		for {
			n, rerr := rf.Read(buf)
			if n > 0 {
				if _, werr := lf.Write(buf[:n]); werr != nil {
					return werr
				}
				transferred += int64(n)
				progress(transferred, total)
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				return rerr
			}
		}
	} else {
		if _, err := io.Copy(lf, rf); err != nil {
			return err
		}
	}
	return nil
}

// ---- 监控 ----

// GetSystemStats 获取系统资源统计
func (c *Client) GetSystemStats() map[string]any {
	stats := map[string]any{
		"cpuUsage": 0.0, "memTotal": int64(0), "memUsed": int64(0), "memUsage": 0.0,
		"diskTotal": int64(0), "diskUsed": int64(0), "diskUsage": 0.0,
		"netRx": int64(0), "netTx": int64(0),
	}

	// CPU 使用率（两次采样）
	_, cpu1, _ := c.ExecCommand("cat /proc/stat | grep '^cpu '", 10, false)
	time.Sleep(150 * time.Millisecond)
	_, cpu2, _ := c.ExecCommand("cat /proc/stat | grep '^cpu '", 10, false)
	if v, ok := calcCPU(cpu1, cpu2); ok {
		stats["cpuUsage"] = v
	}

	// 内存
	_, memOut, _ := c.ExecCommand("free -m | awk 'NR==2{print $2,$3}'", 10, false)
	if total, used, ok := parseTwoInts(memOut); ok && total > 0 {
		stats["memTotal"] = total
		stats["memUsed"] = used
		stats["memUsage"] = round2(float64(used) / float64(total) * 100)
	}

	// 磁盘
	_, diskOut, _ := c.ExecCommand("df -m / | tail -1 | awk '{print $2,$3,$5}'", 10, false)
	if total, used, pct, ok := parseDisk(diskOut); ok {
		stats["diskTotal"] = total
		stats["diskUsed"] = used
		stats["diskUsage"] = pct
	}

	// 网络流量（每秒增量）
	_, net1, _ := c.ExecCommand("cat /proc/net/dev | grep ':' | grep -v 'lo:' | awk '{print $2,$10}'", 10, false)
	time.Sleep(1000 * time.Millisecond)
	_, net2, _ := c.ExecCommand("cat /proc/net/dev | grep ':' | grep -v 'lo:' | awk '{print $2,$10}'", 10, false)
	if rx, tx := calcNet(net1, net2); rx >= 0 && tx >= 0 {
		stats["netRx"] = rx
		stats["netTx"] = tx
	}

	return stats
}

func calcCPU(a, b string) (float64, bool) {
	pa := procFields(a)
	pb := procFields(b)
	if len(pa) < 8 || len(pb) < 8 {
		return 0, false
	}
	// parax: user nice system idle iowait
	ax := pai(pa, 1) + pai(pa, 2) + pai(pa, 3) + pai(pa, 5) + pai(pa, 6) + pai(pa, 7)
	ai := pai(pa, 4)
	bx := pai(pb, 1) + pai(pb, 2) + pai(pb, 3) + pai(pb, 5) + pai(pb, 6) + pai(pb, 7)
	bi := pai(pb, 4)
	dtot := float64((bx + bi) - (ax + ai))
	didle := float64(bi - ai)
	if dtot <= 0 {
		return 0, false
	}
	return round2((dtot - didle) / dtot * 100), true
}

func procFields(s string) []string {
	return strings.Fields(s)
}

func pai(f []string, i int) int64 {
	if i < 0 || i >= len(f) {
		return 0
	}
	v, _ := strconv.ParseInt(f[i], 10, 64)
	return v
}

func parseTwoInts(s string) (int64, int64, bool) {
	f := strings.Fields(s)
	if len(f) < 2 {
		return 0, 0, false
	}
	a, e1 := strconv.ParseInt(f[0], 10, 64)
	b, e2 := strconv.ParseInt(f[1], 10, 64)
	if e1 != nil || e2 != nil {
		return 0, 0, false
	}
	return a, b, true
}

func parseDisk(s string) (int64, int64, float64, bool) {
	f := strings.Fields(s)
	if len(f) < 3 {
		return 0, 0, 0, false
	}
	t, e1 := strconv.ParseInt(f[0], 10, 64)
	u, e2 := strconv.ParseInt(f[1], 10, 64)
	pctStr := strings.TrimSuffix(f[2], "%")
	pct, e3 := strconv.ParseFloat(pctStr, 64)
	if e1 != nil || e2 != nil || e3 != nil {
		return 0, 0, 0, false
	}
	return t, u, pct, true
}

func calcNet(a, b string) (int64, int64) {
	rx1, tx1 := netSums(a)
	rx2, tx2 := netSums(b)
	return rx2 - rx1, tx2 - tx1
}

func netSums(s string) (int64, int64) {
	var rx, tx int64
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 {
			r, e1 := strconv.ParseInt(f[0], 10, 64)
			t, e2 := strconv.ParseInt(f[1], 10, 64)
			if e1 == nil && e2 == nil {
				rx += r
				tx += t
			}
		}
	}
	return rx, tx
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

// ---- 服务管理 ----

// GetServices 获取 systemd 服务列表
func (c *Client) GetServices() ([]models.SystemService, error) {
	cmd := "systemctl list-units --type=service --all --no-legend --no-pager | head -n 200"
	_, out, _ := c.ExecCommand(cmd, 30, false)
	var services []models.SystemService
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		name := strings.TrimSuffix(f[0], ".service")
		services = append(services, models.SystemService{
			Name:        name,
			LoadState:   f[1],
			ActiveState: f[2],
			SubState:    f[3],
			Description: strings.Join(f[4:], " "),
		})
	}
	return services, nil
}

// GetServiceEnabled 查询服务是否开机自启
func (c *Client) GetServiceEnabled(name string) string {
	_, out, _ := c.ExecCommand("systemctl is-enabled "+name+" 2>/dev/null", 10, false)
	return strings.TrimSpace(out)
}

// ServiceAction 执行服务操作
func (c *Client) ServiceAction(name, action string) (int, string, string) {
	return c.ExecCommand("systemctl "+action+" "+name, 30, true)
}

// GetServiceStatus 获取服务状态信息
func (c *Client) GetServiceStatus(name string) string {
	_, out, _ := c.ExecCommand("systemctl status "+name+" --no-pager 2>/dev/null | head -n 20", 15, false)
	return out
}

// OpenShell 打开交互式 shell 会话，返回会话、标准输入写入端、合并输出读取端
func (c *Client) OpenShell(cols, rows int) (*ssh.Session, io.WriteCloser, io.ReadCloser, error) {
	if c.client == nil {
		return nil, nil, nil, errors.New("SSH未连接")
	}
	sess, err := c.client.NewSession()
	if err != nil {
		return nil, nil, nil, err
	}
	pr, pw := io.Pipe()
	sess.Stdout = pw
	sess.Stderr = pw
	stdin, err := sess.StdinPipe()
	if err != nil {
		pw.Close()
		sess.Close()
		return nil, nil, nil, err
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		pw.Close()
		sess.Close()
		return nil, nil, nil, err
	}
	if err := sess.Shell(); err != nil {
		pw.Close()
		sess.Close()
		return nil, nil, nil, err
	}
	return sess, stdin, pr, nil
}
