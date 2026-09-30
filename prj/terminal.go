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

	// tail 最近若干 KB 的原始输出（含 ANSI），供 AI 读取终端内容使用。
	// 输出本身仍然实时推给前端 xterm，这里只是额外留一份可回看的副本。
	tail *terminalTail
	// aiSeq 最近一次由 AI 注入命令时的输出偏移（「自上次 AI 命令以来」这一档要用）
	aiSeq int64
}

// terminalTailSize 每个终端会话保留的原始输出上限。
// 512KB 足够放下一次长安装的完整输出，又不至于让多标签会话把内存吃掉。
const terminalTailSize = 512 * 1024

// terminalTail 环形输出缓冲：只保留最近 terminalTailSize 字节，
// 但保留"全局偏移"（seq/base），因此可以精确地取"从某次命令之后到现在"的片段。
type terminalTail struct {
	mu   sync.Mutex
	data []byte
	base int64 // data[0] 对应的全局偏移
	seq  int64 // 累计写入的字节数
}

func newTerminalTail() *terminalTail {
	return &terminalTail{data: make([]byte, 0, terminalTailSize)}
}

// append 追加输出（超出上限时从头部丢弃）
func (t *terminalTail) append(p []byte) {
	if len(p) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq += int64(len(p))
	if len(p) >= terminalTailSize {
		// 单次就超过整个缓冲：只留最后一段
		t.data = append(t.data[:0], p[len(p)-terminalTailSize:]...)
		t.base = t.seq - int64(len(t.data))
		return
	}
	t.data = append(t.data, p...)
	if len(t.data) > terminalTailSize {
		drop := len(t.data) - terminalTailSize
		t.data = append(t.data[:0], t.data[drop:]...)
		t.base += int64(drop)
	}
}

// snapshot 取 [from, now) 的原始字节；from 早于缓冲区起点时 truncated=true（说明这段已被丢弃）
func (t *terminalTail) snapshot(from int64) (data []byte, fromActual, to int64, truncated bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	to = t.seq
	if from < t.base {
		from = t.base
		truncated = true
	}
	if from > to {
		from = to
	}
	start := int(from - t.base)
	out := make([]byte, len(t.data)-start)
	copy(out, t.data[start:])
	return out, from, to, truncated
}

// end 当前输出偏移
func (t *terminalTail) end() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.seq
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
		tail: newTerminalTail(),
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
				// 先留副本再推送：两边都拿到同一份原始字节（含 ANSI）
				ts.tail.append(buf[:n])
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
	// 终端关了，挂在它上面的 AI 会话也一并收掉（对话上下文只活在内存里）
	a.dropAISessionsForTerm(id)
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
