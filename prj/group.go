package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"ServerManager2/internal/models"
)

// effectiveGroupID 取服务器的生效分组 id（分组单属：列表首项生效，空即未分组）
func effectiveGroupID(ids []string) string {
	for _, id := range ids {
		if s := strings.TrimSpace(id); s != "" {
			return s
		}
	}
	return ""
}

// groupIDsFor 把生效分组 id 组装成存储用的数组形态（长度 0 或 1）
func groupIDsFor(groupID string) []string {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil
	}
	return []string{groupID}
}

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

// DeleteGroup 删除分组，并把该分组下的服务器释放为未分组。
// 不释放的话服务器会继续指向一个已不存在的分组 id，界面上就只能显示裸 id。
// 返回被释放的服务器台数，供前端如实提示。
func (a *App) DeleteGroup(id string) (int, error) {
	if strings.TrimSpace(id) == "" {
		return 0, fmt.Errorf("分组 id 不能为空")
	}
	servers, err := a.store.GetServers()
	if err != nil {
		return 0, err
	}
	now := time.Now()
	released := 0
	for i := range servers {
		if effectiveGroupID(servers[i].GroupIDs) != id {
			continue
		}
		if err := a.store.SetServerGroup(servers[i].ID, "", now); err != nil {
			return released, err
		}
		released++
	}
	if err := a.store.DeleteGroup(id); err != nil {
		return released, err
	}
	return released, nil
}

// AssignGroupServers 批量分配分组，语义是"写入这个分组的成员集合"（编辑器语义，后写覆盖先写）。
// 服务器只能属于一个分组，因此：
//   - groupID 非空：serverIDs 里的服务器全部归入该分组（原本属于别的分组会被转移过来）；原本属于
//     该分组、但本次没有被勾选的服务器释放为未分组；无关服务器不动。
//   - groupID 为空：serverIDs 里的服务器全部移出分组（供"移出分组"使用）。
//
// 已不存在的服务器 id 静默忽略（可能在别处刚被删除）。返回变更计数，其中 conflicts 是
// "归入本组但原本属于其他分组"的台数，用于在保存前/后如实提示冲突。
func (a *App) AssignGroupServers(groupID string, serverIDs []string) (map[string]any, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID != "" {
		groups, err := a.store.GetGroups()
		if err != nil {
			return nil, err
		}
		found := false
		for i := range groups {
			if groups[i].ID == groupID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("分组不存在")
		}
	}

	servers, err := a.store.GetServers()
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(servers))
	for i := range servers {
		known[servers[i].ID] = true
	}
	target := make(map[string]bool, len(serverIDs))
	for _, id := range serverIDs {
		if known[id] {
			target[id] = true
		}
	}

	now := time.Now()
	changed, movedIn, released, conflicts := 0, 0, 0, 0
	for i := range servers {
		sv := servers[i]
		cur := effectiveGroupID(sv.GroupIDs)
		checked := target[sv.ID]
		switch {
		case checked && cur != groupID:
			if err := a.store.SetServerGroup(sv.ID, groupID, now); err != nil {
				return assignResult(changed, movedIn, released, conflicts), err
			}
			changed++
			if groupID != "" {
				movedIn++
			} else {
				released++
			}
			if groupID != "" && cur != "" {
				conflicts++
			}
		case !checked && groupID != "" && cur == groupID:
			if err := a.store.SetServerGroup(sv.ID, "", now); err != nil {
				return assignResult(changed, movedIn, released, conflicts), err
			}
			changed++
			released++
		}
	}
	return assignResult(changed, movedIn, released, conflicts), nil
}

func assignResult(changed, movedIn, released, conflicts int) map[string]any {
	return map[string]any{
		"changed": changed, "movedIn": movedIn, "released": released, "conflicts": conflicts,
	}
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
