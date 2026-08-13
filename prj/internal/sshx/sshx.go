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
