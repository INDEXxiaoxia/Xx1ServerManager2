package main

import (
	"errors"
	"fmt"
	"strings"

	"ServerManager2/internal/crypto"
)

// machineSeedKey 本机密钥种子在 app_settings 中的键名（未设置主密码时用它派生密钥）
const machineSeedKey = "machineKeySeed"

// 兼容启动时本机密钥派生失败的老库：显式拒绝写密码，避免明文落盘
var errNoEncryptionKey = errors.New("加密密钥不可用，请先解锁后再保存密码")

// IsMasterPasswordSet 主密码是否已设置
func (a *App) IsMasterPasswordSet() (bool, error) {
	hash, _ := a.store.GetSetting("masterPasswordHash")
	return hash != "", nil
}

// IsUnlocked 当前是否已解锁
func (a *App) IsUnlocked() bool {
	if a.locked {
		return false
	}
	return a.crypto.HasMasterKey()
}

// IsLocked 当前是否处于锁定态（前端据此显示解锁界面）
func (a *App) IsLocked() bool { return a.locked }

// Lock 锁定应用：清除内存密钥并进入锁定态，同时把"拿着凭据的会话"一起收掉。
// 已设置主密码时，解锁需要重新输入主密码；
// 未设置主密码时（只由本机密钥保护），解锁是一次点击，没有可验证的凭据。
// 注意：锁定不会删除任何已保存的密码——密文照旧留在库里，解锁后仍可解出。
func (a *App) Lock() error {
	a.locked = true
	if a.crypto != nil {
		a.crypto.ClearKey()
	}
	// 监控轮询、文件管理复用连接、交互式终端都持有已认证的 SSH 会话，
	// 锁定后内存里不该再有可解密的凭据，这些会话一并停掉/关闭。
	a.StopAllMonitors()
	a.CloseFileClients()
	a.closeAllTerminals()
	a.closeAllDiskTasks()
	a.closeAllAISessions()
	return nil
}

// UnlockWithoutPassword 不输入密码直接解锁：仅当"未设置主密码"时允许。
// 已设置主密码时必须走 VerifyMasterPassword，避免绕过主密码。
func (a *App) UnlockWithoutPassword() error {
	set, err := a.IsMasterPasswordSet()
	if err != nil {
		return err
	}
	if set {
		return fmt.Errorf("已设置主密码，需要输入主密码解锁")
	}
	if err := a.ensureMachineKey(); err != nil {
		return err
	}
	a.locked = false
	return nil
}

// ensureMachineKey 确保内存中有可用密钥：
// 优先用主密码派生；未设置主密码时，用本机种子派生，
// 保证"没设主密码也能把服务器密码加密保存下来"。
func (a *App) ensureMachineKey() error {
	if a.crypto == nil {
		return fmt.Errorf("加密模块未初始化")
	}
	if a.crypto.HasMasterKey() && !a.locked {
		return nil
	}
	seed, _ := a.store.GetSetting(machineSeedKey)
	if seed == "" {
		s, err := a.crypto.GenerateMachineSeed()
		if err != nil {
			return err
		}
		if err := a.store.SetSetting(machineSeedKey, s); err != nil {
			return err
		}
		seed = s
	}
	if err := a.crypto.DeriveMachineKey(seed); err != nil {
		return err
	}
	a.locked = false
	return nil
}

// SetMasterPassword 设置主密码
func (a *App) SetMasterPassword(password string) error {
	salt, err := a.crypto.GenerateSalt()
	if err != nil {
		return err
	}
	hash := a.crypto.HashPassword(password, salt)
	if err := a.store.SetSetting("masterPasswordHash", hash); err != nil {
		return err
	}
	if err := a.store.SetSetting("encryptionSalt", salt); err != nil {
		return err
	}
	if err := a.crypto.SetMasterPassword(password, salt); err != nil {
		return err
	}
	a.locked = false
	// 之前若用本机密钥加密过密码 / AI Key，这里改用主密钥重新加密，避免设置主密码后解不开
	a.reencryptProtectedSecrets()
	return nil
}

// VerifyMasterPassword 验证主密码，成功则解锁
func (a *App) VerifyMasterPassword(password string) (bool, error) {
	hash, _ := a.store.GetSetting("masterPasswordHash")
	salt, _ := a.store.GetSetting("encryptionSalt")
	if hash == "" || salt == "" {
		return false, nil
	}
	if a.crypto.VerifyMasterPassword(password, hash, salt) {
		_ = a.crypto.SetMasterPassword(password, salt)
		a.locked = false
		// 解锁后顺手把仍是本机密钥加密的密码 / AI Key 迁移到主密钥
		a.reencryptProtectedSecrets()
		return true, nil
	}
	return false, nil
}

// ChangeMasterPassword 重置主密码：要求当前主密码正确，并把已保存的服务器密码
// 与 AI API Key 从旧密钥重新加密到新密钥下。
// 不做这一步的话，任何一条用旧主密钥加密的密文都会永久解不开（等于把密码弄丢），
// 所以这里先解出明文、再换新密钥写回；单条解不开的保持原样不动（绝不删除）。
// 返回 {"reencrypted":n,"failed":m,"aiKeyReencrypted":bool,"aiKeyFailed":bool}。
func (a *App) ChangeMasterPassword(oldPassword, newPassword string) (map[string]any, error) {
	hash, _ := a.store.GetSetting("masterPasswordHash")
	salt, _ := a.store.GetSetting("encryptionSalt")
	if hash == "" || salt == "" {
		return nil, fmt.Errorf("尚未设置主密码，请使用「设置主密码」")
	}
	if !a.crypto.VerifyMasterPassword(oldPassword, hash, salt) {
		return nil, fmt.Errorf("当前主密码不正确")
	}

	// 1) 用当前（旧）密钥把能解开的密码先解出来
	servers, err := a.store.GetServers()
	if err != nil {
		return nil, err
	}
	type pending struct {
		id    string
		plain string
	}
	var plains []pending
	failed := 0
	for i := range servers {
		sv := servers[i]
		if sv.Password == "" {
			continue
		}
		plain, err := a.crypto.DecryptPassword(sv.Password)
		if err != nil {
			if p, ok := a.decryptWithMachineKey(sv.Password); ok {
				plain = p
			} else {
				failed++
				continue
			}
		}
		plains = append(plains, pending{id: sv.ID, plain: plain})
	}

	// 1b) AI API Key（与服务器密码同一套密钥，重置时必须一并迁移）
	aiPlain := ""
	aiPending := false
	aiKeyFailed := false
	if enc, _ := a.store.GetSetting(aiKeyAPIKey); strings.TrimSpace(enc) != "" {
		if p, err := a.crypto.DecryptPassword(enc); err == nil {
			aiPlain = p
			aiPending = true
		} else if p, ok := a.decryptWithMachineKey(enc); ok {
			aiPlain = p
			aiPending = true
		} else {
			aiKeyFailed = true
		}
	}

	// 2) 换新盐、新哈希、新密钥
	newSalt, err := a.crypto.GenerateSalt()
	if err != nil {
		return nil, err
	}
	if err := a.store.SetSetting("masterPasswordHash", a.crypto.HashPassword(newPassword, newSalt)); err != nil {
		return nil, err
	}
	if err := a.store.SetSetting("encryptionSalt", newSalt); err != nil {
		return nil, err
	}
	if err := a.crypto.SetMasterPassword(newPassword, newSalt); err != nil {
		return nil, err
	}
	a.locked = false

	// 3) 用新密钥重新加密写回
	reencrypted := 0
	for _, p := range plains {
		enc, err := a.crypto.EncryptPassword(p.plain)
		if err != nil {
			failed++
			continue
		}
		if err := a.store.SetServerPassword(p.id, enc); err != nil {
			failed++
			continue
		}
		reencrypted++
	}

	aiKeyReencrypted := false
	if aiPending {
		enc, err := a.crypto.EncryptPassword(aiPlain)
		if err != nil {
			aiKeyFailed = true
		} else if err := a.store.SetSetting(aiKeyAPIKey, enc); err != nil {
			aiKeyFailed = true
		} else {
			aiKeyReencrypted = true
		}
	}

	return map[string]any{
		"reencrypted":       reencrypted,
		"failed":            failed,
		"aiKeyReencrypted":  aiKeyReencrypted,
		"aiKeyFailed":       aiKeyFailed,
	}, nil
}

// decryptWithMachineKey 用本机种子派生密钥尝试解密。
// 用途：主密码设置/重置之前用本机密钥加密的历史密文，在当前密钥解不开时兜底，
// 避免"没设主密码时保存的密码"变成不可用。密钥强度说明见 DeriveMachineKey 注释。
func (a *App) decryptWithMachineKey(enc string) (string, bool) {
	seed, _ := a.store.GetSetting(machineSeedKey)
	if strings.TrimSpace(seed) == "" || strings.TrimSpace(enc) == "" {
		return "", false
	}
	tmp := crypto.New()
	if err := tmp.DeriveMachineKey(seed); err != nil {
		return "", false
	}
	plain, err := tmp.DecryptPassword(enc)
	if err != nil {
		return "", false
	}
	return plain, true
}

// reencryptProtectedSecrets 把仍用本机密钥加密的敏感项迁移到当前主密钥下。
// 覆盖：服务器登录密码、AI API Key。每次设置主密码 / 解锁都会跑一遍，
// 因此即便中途失败的条目，下次解锁也能补上，不会丢。
func (a *App) reencryptProtectedSecrets() {
	a.reencryptServerPasswords()
	a.reencryptAIAPIKey()
}

// reencryptServerPasswords 把"用本机密钥加密"的服务器密码改用当前主密钥重新加密。
// 做法：先用本机密钥解出明文，再用主密钥加密。
func (a *App) reencryptServerPasswords() {
	seed, _ := a.store.GetSetting(machineSeedKey)
	if seed == "" {
		return
	}
	servers, err := a.store.GetServers()
	if err != nil {
		return
	}
	for i := range servers {
		sv := servers[i]
		if sv.Password == "" {
			continue
		}
		// 解不开说明本就是主密钥加密的，跳过
		if _, err := a.crypto.DecryptPassword(sv.Password); err == nil {
			continue
		}
		// 用临时管理器按本机种子解开
		tmp := crypto.New()
		if err := tmp.DeriveMachineKey(seed); err != nil {
			return
		}
		plain, err := tmp.DecryptPassword(sv.Password)
		if err != nil {
			continue
		}
		enc, err := a.crypto.EncryptPassword(plain)
		if err != nil {
			continue
		}
		sv.Password = enc
		_ = a.store.UpdateServer(&sv)
	}
}

// reencryptAIAPIKey 把仍用本机密钥加密的 AI API Key 改用当前主密钥重新加密。
// 与服务器密码同一套迁移逻辑，避免"先存 AI Key、后设主密码"后 Key 解不开。
func (a *App) reencryptAIAPIKey() {
	if a.store == nil || a.crypto == nil || !a.crypto.HasMasterKey() {
		return
	}
	enc, _ := a.store.GetSetting(aiKeyAPIKey)
	if strings.TrimSpace(enc) == "" {
		return
	}
	if _, err := a.crypto.DecryptPassword(enc); err == nil {
		return
	}
	plain, ok := a.decryptWithMachineKey(enc)
	if !ok {
		return
	}
	newEnc, err := a.crypto.EncryptPassword(plain)
	if err != nil {
		return
	}
	_ = a.store.SetSetting(aiKeyAPIKey, newEnc)
}

// GetAppInfo 返回应用基础信息（未解锁状态）
func (a *App) GetAppInfo() (map[string]any, error) {
	set, err := a.IsMasterPasswordSet()
	if err != nil {
		return nil, err
	}
	settings, _ := a.store.GetSettings()
	return map[string]any{
		"masterPasswordSet": set,
		"unlocked":          a.IsUnlocked(),
		"locked":            a.locked,
		"settings":          settings,
	}, nil
}
