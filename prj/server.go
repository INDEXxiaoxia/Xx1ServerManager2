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
	if sv.Password != "" && a.crypto.HasMasterKey() {
		if dec, err := a.crypto.DecryptPassword(sv.Password); err == nil {
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
	if isNew {
		sv.ID = uuid.NewString()
		sv.CreatedAt = now
		sv.Status = models.StatusDisconnected
	} else {
		if sv.Password == "" {
			sv.Password = existing.Password // 保持原密码
		}
		sv.CreatedAt = existing.CreatedAt
	}
	sv.UpdatedAt = now
	if sv.Password != "" && a.crypto.HasMasterKey() {
		enc, err := a.crypto.EncryptPassword(sv.Password)
		if err != nil {
			return sv, err
		}
		sv.Password = enc
	}
	if isNew {
		return sv, a.store.AddServer(&sv)
	}
	return sv, a.store.UpdateServer(&sv)
}

// DeleteServer 删除服务器
func (a *App) DeleteServer(id string) error {
	return a.store.DeleteServer(id)
}

// DeleteServers 批量删除服务器
func (a *App) DeleteServers(ids []string) (int, error) {
	count := 0
	for _, id := range ids {
		if err := a.store.DeleteServer(id); err == nil {
			count++
		}
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
