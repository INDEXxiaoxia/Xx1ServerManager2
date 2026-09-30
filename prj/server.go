package main

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"ServerManager2/internal/models"
	"ServerManager2/internal/sshx"
)

// getServerWithPassword 获取服务器并解密密码
func (a *App) getServerWithPassword(id string) (*models.Server, error) {
	sv, err := a.store.GetServerByID(id)
	if err != nil {
		return nil, err
	}
	if sv == nil {
		return nil, fmt.Errorf("服务器不存在")
	}
	if sv.Password != "" {
		if dec, err := a.crypto.DecryptPassword(sv.Password); err == nil {
			sv.Password = dec
		} else if dec, ok := a.decryptWithMachineKey(sv.Password); ok {
			// 主密码设置之前用本机密钥加密的历史行：当前密钥解不开时回退本机密钥，
			// 保证"没设主密码时保存的密码"在设置主密码后也不会变成不可用。
			sv.Password = dec
		}
	}
	return sv, nil
}

// ListServers 获取服务器列表（不返回密码）
func (a *App) ListServers() ([]models.Server, error) {
	servers, err := a.store.GetServers()
	if err != nil {
		return nil, err
	}
	for i := range servers {
		servers[i].Password = ""
	}
	return servers, nil
}

// GetServer 获取单个服务器（不返回密码）
func (a *App) GetServer(id string) (models.Server, error) {
	sv, err := a.store.GetServerByID(id)
	if err != nil {
		return models.Server{}, err
	}
	if sv == nil {
		return models.Server{}, fmt.Errorf("服务器不存在")
	}
	sv.Password = ""
	return *sv, nil
}

// SaveServer 创建或更新服务器
func (a *App) SaveServer(sv models.Server) (models.Server, error) {
	existing, _ := a.store.GetServerByID(sv.ID)
	now := time.Now()
	isNew := existing == nil
	// plain 是本次提交的**新明文密码**，空表示"不改密码"。
	// 必须先在这里分流：早先把原密文塞回 sv.Password 再加密，会让"改名不改密码"的
	// 编辑把密文二次加密，密码当场失效。
	plain := sv.Password
	if isNew {
		sv.ID = uuid.NewString()
		sv.CreatedAt = now
		sv.Status = models.StatusDisconnected
	} else {
		sv.CreatedAt = existing.CreatedAt
	}
	// 分组单属：前端只提交一个分组，这里再兜一层，避免多值落库
	sv.GroupIDs = groupIDsFor(effectiveGroupID(sv.GroupIDs))
	sv.UpdatedAt = now

	switch {
	case plain != "":
		// 没有可用密钥时宁可报错，也绝不把明文密码写进数据库
		if a.crypto == nil || !a.crypto.HasMasterKey() {
			return sv, errNoEncryptionKey
		}
		enc, err := a.crypto.EncryptPassword(plain)
		if err != nil {
			return sv, err
		}
		sv.Password = enc
	case !isNew:
		sv.Password = existing.Password // 沿用原密文，原样写回
	default:
		sv.Password = ""
	}

	if isNew {
		return sv, a.store.AddServer(&sv)
	}
	if err := a.store.UpdateServer(&sv); err != nil {
		return sv, err
	}
	// 主机/端口/账号/密码可能已变更，旧连接不能继续复用
	a.invalidateFileClient(sv.ID)
	return sv, nil
}

// DeleteServer 删除服务器
func (a *App) DeleteServer(id string) error {
	err := a.store.DeleteServer(id)
	// 连接配置已失效，丢弃文件管理的复用连接
	a.invalidateFileClient(id)
	return err
}

// DeleteServers 批量删除服务器
func (a *App) DeleteServers(ids []string) (int, error) {
	count := 0
	for _, id := range ids {
		if err := a.store.DeleteServer(id); err == nil {
			count++
		}
		a.invalidateFileClient(id)
	}
	return count, nil
}

// TestServerConnection 测试连接（直接输入配置）
func (a *App) TestServerConnection(host string, port int, username, password string) map[string]any {
	ok, msg := sshx.New(host, port, username, password, 10).TestConnection()
	return map[string]any{"connected": ok, "message": msg}
}

// TestStoredServer 测试已存储服务器连接
func (a *App) TestStoredServer(id string) (map[string]any, error) {
	sv, err := a.getServerWithPassword(id)
	if err != nil {
		return nil, err
	}
	ok, msg := sshx.New(sv.Host, sv.Port, sv.Username, sv.Password, 10).TestConnection()
	status := models.StatusConnected
	if !ok {
		status = models.StatusError
	}
	sv.Status = status
	sv.UpdatedAt = time.Now()
	_ = a.store.UpdateServer(sv)
	return map[string]any{"connected": ok, "message": msg}, nil
}

// SetServerStatus 设置服务器状态（供前端刷新）
func (a *App) SetServerStatus(id string, status string) error {
	sv, err := a.store.GetServerByID(id)
	if err != nil || sv == nil {
		return fmt.Errorf("服务器不存在")
	}
	sv.Status = models.ServerStatus(status)
	sv.UpdatedAt = time.Now()
	return a.store.UpdateServer(sv)
}
