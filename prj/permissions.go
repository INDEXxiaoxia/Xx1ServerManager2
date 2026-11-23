package main

import (
	"ServerManager2/internal/models"
	"ServerManager2/internal/sshx"
	"crypto/sha256"
	"fmt"
	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type permissionAnswer struct {
	Password string
	Allowed  bool
}

func (a *App) ResolveRootRequest(id, password string, allowed bool) {
	a.permissionMu.Lock()
	ch := a.permissionRequests[id]
	a.permissionMu.Unlock()
	if ch != nil {
		select {
		case ch <- permissionAnswer{password, allowed}:
		default:
		}
	}
}
func (a *App) requestRoot(sv *models.Server) (string, error) {
	a.permissionMu.Lock()
	if a.rootRequestLocks == nil {
		a.rootRequestLocks = map[string]*sync.Mutex{}
	}
	lock := a.rootRequestLocks[sv.ID]
	if lock == nil {
		lock = &sync.Mutex{}
		a.rootRequestLocks[sv.ID] = lock
	}
	a.permissionMu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	if pw, ok := a.cachedRoot(sv, time.Now()); ok {
		return pw, nil
	}

	id := uuid.NewString()
	ch := make(chan permissionAnswer, 1)
	a.permissionMu.Lock()
	if a.permissionRequests == nil {
		a.permissionRequests = map[string]chan permissionAnswer{}
	}
	a.permissionRequests[id] = ch
	a.permissionMu.Unlock()
	defer func() { a.permissionMu.Lock(); delete(a.permissionRequests, id); a.permissionMu.Unlock() }()
	runtime.EventsEmit(a.ctx, "server:root-request", map[string]string{"id": id, "server": sv.Name})
	select {
	case answer := <-ch:
		if !answer.Allowed {
			return "", fmt.Errorf("用户取消了提权")
		}
		c := a.newServerClient(sv, 10)
		c.RefreshRoot = nil
		c.RequestRoot = nil
		c.DefaultRoot = true
		c.SudoPassword = answer.Password
		if err := c.Connect(); err != nil {
			return "", err
		}
		defer c.Close()
		code, out, msg := c.ExecCommand("id -u", 10, false)
		if code != 0 || strings.TrimSpace(out) != "0" {
			return "", fmt.Errorf("提权验证失败: %s", msg)
		}
		a.permissionMu.Lock()
		if a.locked {
			a.permissionMu.Unlock()
			return "", fmt.Errorf("应用已锁定")
		}
		if a.elevated == nil {
			a.elevated = map[string]rootGrant{}
		}
		a.elevated[sv.ID] = rootGrant{answer.Password, time.Now(), rootIdentity(sv)}
		a.permissionMu.Unlock()
		return answer.Password, nil
	case <-time.After(3 * time.Minute):
		return "", fmt.Errorf("提权授权已超时")
	}
}
func (a *App) newServerClient(sv *models.Server, timeout int) *sshx.Client {
	c := sshx.New(sv.Host, sv.Port, sv.Username, sv.Password, timeout)
	c.AuthType = sv.AuthType
	c.PrivateKey = sv.PrivateKey
	c.KeyPassphrase = sv.KeyPassphrase
	c.DefaultRoot = sv.DefaultRoot
	c.SudoPassword = sv.SudoPassword
	if c.SudoPassword == "" {
		c.SudoPassword = sv.Password
	}
	basePassword := c.SudoPassword
	c.RefreshRoot = func() (bool, string) {
		if pw, ok := a.cachedRoot(sv, time.Now()); ok {
			return true, pw
		}
		if sv.DefaultRoot {
			return true, basePassword
		}
		return false, basePassword
	}
	c.DefaultRoot, c.SudoPassword = c.RefreshRoot()
	c.RequestRoot = func() (string, error) {
		if c.DefaultRoot {
			a.permissionMu.Lock()
			delete(a.elevated, sv.ID)
			a.permissionMu.Unlock()
		}
		return a.requestRoot(sv)
	}
	return c
}
func (a *App) RequestServerRoot(id string) error {
	sv, err := a.getServerWithPassword(id)
	if err != nil {
		return err
	}
	_, err = a.requestRoot(sv)
	if err != nil {
		return err
	}
	a.invalidateFileClient(id)
	return nil
}

type rootGrant struct {
	Password  string
	GrantedAt time.Time
	Identity  string
}

func rootIdentity(s *models.Server) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s|%s|%s|%s|%s|%s|%t", s.Host, s.Port, s.Username, s.AuthType, s.Password, s.PrivateKey, s.KeyPassphrase, s.SudoPassword, s.DefaultRoot))))
}
func (a *App) GetRootCacheMinutes() (int, error) {
	v, e := a.store.GetSetting("rootCacheMinutes")
	if e != nil {
		return 0, e
	}
	n, _ := strconv.Atoi(v)
	if n != 10 && n != 60 {
		return 0, nil
	}
	return n, nil
}
func (a *App) SetRootCacheMinutes(n int) error {
	if n != 0 && n != 10 && n != 60 {
		return fmt.Errorf("缓存时间只能是退出软件前、10 分钟或 60 分钟")
	}
	return a.store.SetSetting("rootCacheMinutes", strconv.Itoa(n))
}
func (a *App) cachedRoot(sv *models.Server, now time.Time) (string, bool) {
	minutes, _ := a.GetRootCacheMinutes()
	a.permissionMu.Lock()
	defer a.permissionMu.Unlock()
	g, ok := a.elevated[sv.ID]
	if !ok {
		return "", false
	}
	if g.Identity != rootIdentity(sv) || (minutes > 0 && !now.Before(g.GrantedAt.Add(time.Duration(minutes)*time.Minute))) {
		delete(a.elevated, sv.ID)
		return "", false
	}
	return g.Password, true
}

// 普通交互式 SSH 始终使用登录用户，不继承软件默认提权或临时授权。
// 容器终端仍可使用操作授权，因为启动 docker exec 可能需要访问受限的 socket。
func (a *App) newTerminalClient(sv *models.Server, command string) *sshx.Client {
	c := a.newServerClient(sv, 10)
	if command == "" {
		c.RefreshRoot = func() (bool, string) { return false, "" }
		c.DefaultRoot, c.SudoPassword = c.RefreshRoot()
		c.RequestRoot = nil
	}
	return c
}
