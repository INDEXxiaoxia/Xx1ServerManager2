package main

import (
	"time"

	"github.com/google/uuid"

	"ServerManager2/internal/models"
)

// ListGroups 获取分组列表
func (a *App) ListGroups() ([]models.Group, error) {
	return a.store.GetGroups()
}

// SaveGroup 创建或更新分组
func (a *App) SaveGroup(g models.Group) (models.Group, error) {
	if g.ID == "" {
		g.ID = uuid.NewString()
		g.CreatedAt = time.Now()
		return g, a.store.AddGroup(&g)
	}
	return g, a.store.UpdateGroup(&g)
}

// DeleteGroup 删除分组
func (a *App) DeleteGroup(id string) error {
	return a.store.DeleteGroup(id)
}

// ListPresetCommands 获取预设命令列表
func (a *App) ListPresetCommands() ([]models.PresetCommand, error) {
	return a.store.GetPresetCommands()
}

// SavePresetCommand 创建或更新预设命令
func (a *App) SavePresetCommand(c models.PresetCommand) (models.PresetCommand, error) {
	if c.ID == "" {
		c.ID = uuid.NewString()
		c.CreatedAt = time.Now()
		return c, a.store.AddPresetCommand(&c)
	}
	return c, a.store.UpdatePresetCommand(&c)
}

// DeletePresetCommand 删除预设命令
func (a *App) DeletePresetCommand(id string) error {
	return a.store.DeletePresetCommand(id)
}

// ListPresetPaths 获取预设路径列表
func (a *App) ListPresetPaths() ([]models.PresetPath, error) {
	return a.store.GetPresetPaths()
}

// SavePresetPath 创建或更新预设路径
func (a *App) SavePresetPath(p models.PresetPath) (models.PresetPath, error) {
	if p.ID == "" {
		p.ID = uuid.NewString()
		p.CreatedAt = time.Now()
		return p, a.store.AddPresetPath(&p)
	}
	return p, a.store.UpdatePresetPath(&p)
}

// DeletePresetPath 删除预设路径
func (a *App) DeletePresetPath(id string) error {
	return a.store.DeletePresetPath(id)
}

// GetSettings 获取应用设置
func (a *App) GetSettings() (models.Settings, error) {
	return a.store.GetSettings()
}

// SaveSettings 保存应用设置
func (a *App) SaveSettings(s models.Settings) error {
	return a.store.SaveSettings(s)
}
