package main

// IsMasterPasswordSet 主密码是否已设置
func (a *App) IsMasterPasswordSet() (bool, error) {
	hash, _ := a.store.GetSetting("masterPasswordHash")
	return hash != "", nil
}

// IsUnlocked 当前是否已解锁（主密钥已就绪）
func (a *App) IsUnlocked() bool {
	return a.crypto.HasMasterKey()
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
	return a.crypto.SetMasterPassword(password, salt)
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
		return true, nil
	}
	return false, nil
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
		"unlocked":          a.crypto.HasMasterKey(),
		"settings":          settings,
	}, nil
}
