package sshx

import (
	"bufio"
	"fmt"
	"github.com/pkg/sftp"
	"strings"
	"time"
)

func (c *Client) rootSFTP() (*sftp.Client, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return nil, err
	}
	input, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	output, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	ready := make(chan error, 1)
	go func() {
		r := bufio.NewScanner(stderr)
		var notes strings.Builder
		prompted := false
		for r.Scan() {
			line := r.Text()
			if strings.Contains(line, "XX1_PASSWORD") {
				if !prompted {
					fmt.Fprintln(input, c.SudoPassword)
					prompted = true
				} else {
					select {
					case ready <- fmt.Errorf("sudo 密码错误"):
					default:
					}
					return
				}
			} else if strings.Contains(line, "XX1_READY") {
				select {
				case ready <- nil:
				default:
				}
			} else {
				if notes.Len() < 4096 {
					notes.WriteString(line + "\n")
				}
			}
		}
		select {
		case ready <- fmt.Errorf("无法启动 root 文件服务: %s", notes.String()):
		default:
		}
	}()
	script := `printf 'XX1_READY\n' >&2; for p in /usr/lib/openssh/sftp-server /usr/lib/ssh/sftp-server /usr/libexec/openssh/sftp-server /usr/libexec/sftp-server; do if [ -x "$p" ]; then exec "$p"; fi; done; echo '未找到 sftp-server' >&2; exit 1`
	if err = sess.Start("sudo -S -p 'XX1_PASSWORD\n' -- sh -c " + shellQuote(script)); err != nil {
		sess.Close()
		return nil, err
	}
	select {
	case err = <-ready:
	case <-time.After(15 * time.Second):
		err = fmt.Errorf("sudo 文件服务启动超时")
	}
	if err != nil {
		sess.Close()
		return nil, err
	}
	sc, err := sftp.NewClientPipe(output, input)
	if err != nil {
		sess.Close()
		return nil, err
	}
	c.sftp = sc
	go func() { _ = sess.Wait() }()
	return sc, nil
}
