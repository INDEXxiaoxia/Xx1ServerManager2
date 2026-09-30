package main

import (
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"

	"ServerManager2/internal/sshx"
)

// TerminalSession 交互式 SSH 终端会话
type TerminalSession struct {
	mu       sync.Mutex
	ID       string
	ServerID string
	client   *sshx.Client
	sess     *ssh.Session
	stdin    io.Writer
	out      io.ReadCloser
	closed   bool
}

// TerminalOpen 打开新的 SSH 终端会话
func (a *App) TerminalOpen(serverID string, cols, rows int) (string, error) {
	sv, err := a.getServerWithPassword(serverID)
	if err != nil {
		return "", err
	}
	c := sshx.New(sv.Host, sv.Port, sv.Username, sv.Password, 10)
	if err := c.Connect(); err != nil {
		return "", err
	}
	sess, stdin, out, err := c.OpenShell(cols, rows)
	if err != nil {
		c.Close()
		return "", err
	}

	id := uuid.NewString()
	ts := &TerminalSession{
		ID: id, ServerID: serverID, client: c, sess: sess, stdin: stdin, out: out,
	}

	a.terminalMu.Lock()
	a.terminals[id] = ts
	a.terminalMu.Unlock()

	// 读取输出并推送到前端
	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := out.Read(buf)
			if n > 0 {
				runtime.EventsEmit(a.ctx, "terminal:data:"+id, string(buf[:n]))
			}
			if rerr != nil {
				break
			}
		}
		runtime.EventsEmit(a.ctx, "terminal:close:"+id, map[string]any{"id": id})
	}()

	return id, nil
}

// TerminalWrite 向终端写入数据。
// 会话已退出（EOF）时不算错误：标记会话关闭并返回 nil，
// 否则前端每次按键都会拿到一个 PEM 拒绝，被全局兜底弹成红色错误条。
func (a *App) TerminalWrite(id, data string) error {
	a.terminalMu.Lock()
	ts, ok := a.terminals[id]
	a.terminalMu.Unlock()
	if !ok {
		return nil
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.closed {
		return nil
	}
	if _, err := ts.stdin.Write([]byte(data)); err != nil {
		if sshx.IsSessionGone(err) {
			ts.closed = true
			return nil
		}
		return err
	}
	return nil
}

// TerminalResize 调整终端尺寸
func (a *App) TerminalResize(id string, cols, rows int) error {
	a.terminalMu.Lock()
	ts, _ := a.terminals[id]
	a.terminalMu.Unlock()
	if ts == nil {
		return nil
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.closed {
		return nil
	}
	if err := ts.sess.WindowChange(rows, cols); err != nil {
		// 会话已结束时不报错（展开/收起侧栏等操作会触发 resize）
		if sshx.IsSessionGone(err) {
			ts.closed = true
			return nil
		}
		return err
	}
	return nil
}

// TerminalWriteMany 一次向多个终端写入相同数据（多终端同步操作）。
// 单个会话写入失败时重试一次（短暂阻塞/背压的常见情况），仍失败则跳过该会话，
// 不阻塞其余终端；返回实际写入成功的会话数与首个错误，便于前端如实提示。
func (a *App) TerminalWriteMany(ids []string, data string) (int, error) {
	written := 0
	var firstErr error
	for _, id := range ids {
		a.terminalMu.Lock()
		ts, ok := a.terminals[id]
		a.terminalMu.Unlock()
		if !ok {
			continue
		}
		if err := a.writeTerminalOnce(ts, data); err != nil {
			// 重试一次：同步键入是逐键调用，偶发的一次失败不该让某个窗口少一个字符
			if err2 := a.writeTerminalOnce(ts, data); err2 != nil {
				if firstErr == nil {
					firstErr = err2
				}
				continue
			}
		}
		written++
	}
	return written, firstErr
}

func (a *App) writeTerminalOnce(ts *TerminalSession, data string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.closed || ts.stdin == nil {
		return errors.New("会话已关闭")
	}
	_, err := ts.stdin.Write([]byte(data))
	return err
}

// RunRemoteCommand 在指定服务器上执行一次性命令并返回标准输出。
// 注意：它使用独立的一次性 SSH 连接，不占用交互式终端会话的通道，
// 因此不会污染终端屏幕、也不会与正在输入的 shell 抢占。
func (a *App) RunRemoteCommand(serverID, command string, timeout int) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", errors.New("命令不能为空")
	}
	sv, err := a.getServerWithPassword(serverID)
	if err != nil {
		return "", err
	}
	if timeout <= 0 {
		timeout = 10
	}
	c := sshx.New(sv.Host, sv.Port, sv.Username, sv.Password, 10)
	if err := c.Connect(); err != nil {
		return "", err
	}
	defer c.Close()
	code, stdout, stderr := c.ExecCommand(command, timeout, false)
	if code != 0 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = strings.TrimSpace(stdout)
		}
		if msg == "" {
			msg = "命令执行失败"
		}
		return "", errors.New(msg)
	}
	return strings.TrimSpace(stdout), nil
}

// TerminalClose 关闭终端会话
func (a *App) TerminalClose(id string) error {
	a.terminalMu.Lock()
	ts, ok := a.terminals[id]
	delete(a.terminals, id)
	a.terminalMu.Unlock()
	if !ok {
		return nil
	}
	ts.mu.Lock()
	ts.closed = true
	ts.mu.Unlock()
	if ts.out != nil {
		ts.out.Close()
	}
	ts.sess.Close()
	ts.client.Close()
	return nil
}

// closeAllTerminals 关闭全部交互式终端会话（锁定时调用，不依赖前端是否来得及发关闭请求）
func (a *App) closeAllTerminals() {
	a.terminalMu.Lock()
	ids := make([]string, 0, len(a.terminals))
	for id := range a.terminals {
		ids = append(ids, id)
	}
	a.terminalMu.Unlock()
	for _, id := range ids {
		_ = a.TerminalClose(id)
	}
}
