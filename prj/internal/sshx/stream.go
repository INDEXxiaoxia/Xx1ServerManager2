package sshx

import (
	"strings"
	"sync"
	"time"
)

type streamBuffer struct {
	mu   sync.Mutex
	text string
}

func (b *streamBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	b.text += string(p)
	if len(b.text) > 1024*1024 {
		b.text = "\n[日志过快，已省略较早内容]\n" + b.text[len(b.text)-512*1024:]
	}
	b.mu.Unlock()
	return len(p), nil
}
func (b *streamBuffer) take() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	v := b.text
	b.text = ""
	return v
}
func (c *Client) StreamCommand(command string, emit func(string)) error {
	tail := ""
	err := c.streamCommandOnce(command, func(s string) {
		tail += s
		if len(tail) > 8192 {
			tail = tail[len(tail)-8192:]
		}
		emit(s)
	})
	if err != nil && IsPermissionError(tail) && c.RequestRoot != nil && c.Username != "root" {
		pw, e := c.RequestRoot()
		if e != nil {
			return e
		}
		c.DefaultRoot = true
		c.SudoPassword = pw
		return c.streamCommandOnce(command, emit)
	}
	return err
}
func (c *Client) streamCommandOnce(command string, emit func(string)) error {
	sess, err := c.client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	b := &streamBuffer{}
	sess.Stdout = b
	sess.Stderr = b
	if err = sess.Start(c.commandFor(command)); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- sess.Wait() }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	flush := func() {
		if s := b.take(); strings.TrimSpace(s) != "" {
			emit(s)
		}
	}
	for {
		select {
		case err := <-done:
			flush()
			return err
		case <-ticker.C:
			flush()
		}
	}
}
