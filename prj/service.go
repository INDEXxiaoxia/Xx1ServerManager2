package main

import (
	"ServerManager2/internal/models"
)

// ListServices 获取服务器 systemd 服务列表
func (a *App) ListServices(serverID string) ([]models.SystemService, error) {
	_, c, err := a.dialServer(serverID)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	services, err := c.GetServices()
	return services, err
}

// GetServicesWithEnabled 获取服务列表并补充自启状态
func (a *App) GetServicesWithEnabled(serverID string) ([]models.SystemService, error) {
	_, c, err := a.dialServer(serverID)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	services, err := c.GetServices()
	if err != nil {
		return nil, err
	}
	for i := range services {
		services[i].Enabled = c.GetServiceEnabled(services[i].Name)
	}
	return services, nil
}

// ServiceAction 执行服务操作（start/stop/restart/enable/disable）
func (a *App) ServiceAction(serverID, name, action string) (map[string]any, error) {
	_, c, err := a.dialServer(serverID)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	exitCode, stdout, stderr := c.ServiceAction(name, action)
	status := "success"
	if exitCode != 0 {
		status = "failed"
	}
	return map[string]any{
		"service": name, "action": action, "status": status,
		"exitCode": exitCode, "stdout": stdout, "stderr": stderr,
	}, nil
}

// GetServiceStatus 获取服务状态详情
func (a *App) GetServiceStatus(serverID, name string) (string, error) {
	_, c, err := a.dialServer(serverID)
	if err != nil {
		return "", err
	}
	defer c.Close()
	return c.GetServiceStatus(name), nil
}
