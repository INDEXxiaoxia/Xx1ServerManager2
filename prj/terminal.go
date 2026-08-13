package main

import (
	"io"
	"sync"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"

	"ServerManager2/internal/sshx"
)

// TerminalSession 交互式 SSH 终端会话
type TerminalSession struct {
	mu     sync.Mutex
	ID     string
	ServerID string
	client *sshx.Client
	sess   *ssh.Session
	stdin  io.Writer
	out    io.ReadCloser
	closed bool
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

// TerminalWrite 向终端写入数据
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
	_, err := ts.stdin.Write([]byte(data))
	return err
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
	return ts.sess.WindowChange(rows, cols)
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