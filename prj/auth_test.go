package main

import (
	"testing"

	"ServerManager2/internal/models"
)

// storedCipher 读取库里保存的密码列（密文）
func storedCipher(t *testing.T, a *App, serverID string) string {
	t.Helper()
	sv, err := a.store.GetServerByID(serverID)
	if err != nil || sv == nil {
		t.Fatalf("读取服务器失败: %v", err)
	}
	return sv.Password
}

// TestEditWithoutNewPasswordKeepsCipher 密码留空的编辑必须原样保留密文：
// 一旦把密文当成明文再加密一次，密码就会当场失效（改名就改坏密码）。
func TestEditWithoutNewPasswordKeepsCipher(t *testing.T) {
	a := newTestApp(t)
	if err := a.ensureMachineKey(); err != nil {
		t.Fatalf("初始化本机密钥失败: %v", err)
	}
	sv, err := a.SaveServer(models.Server{
		Name: "web-01", Host: "10.0.0.1", Port: 22, Username: "root", Password: "s3cret!",
	})
	if err != nil {
		t.Fatalf("保存服务器失败: %v", err)
	}
	before := storedCipher(t, a, sv.ID)

	if _, err := a.SaveServer(models.Server{
		ID: sv.ID, Name: "web-01-renamed", Host: "10.0.0.2", Port: 2222, Username: "root",
	}); err != nil {
		t.Fatalf("留空密码的编辑失败: %v", err)
	}
	if got := storedCipher(t, a, sv.ID); got != before {
		t.Fatal("密码留空时密文必须原样保留，不能被二次加密")
	}
	got, err := a.getServerWithPassword(sv.ID)
	if err != nil {
		t.Fatalf("读取服务器失败: %v", err)
	}
	if got.Password != "s3cret!" {
		t.Fatalf("改名后密码应仍可解出，got %q", got.Password)
	}
	if got.Name != "web-01-renamed" || got.Host != "10.0.0.2" || got.Port != 2222 {
		t.Fatalf("其余字段应正常更新: %+v", got)
	}
}

// TestPasswordPersistsWithoutMasterPassword 未设置主密码时密码也必须能持久化：
// 库里是密文（不是明文），锁定再解锁后仍能解出原文。
func TestPasswordPersistsWithoutMasterPassword(t *testing.T) {
	a := newTestApp(t)
	if err := a.ensureMachineKey(); err != nil {
		t.Fatalf("初始化本机密钥失败: %v", err)
	}

	sv, err := a.SaveServer(models.Server{
		Name: "web-01", Host: "10.0.0.1", Port: 22, Username: "root", Password: "s3cret!",
	})
	if err != nil {
		t.Fatalf("保存服务器失败: %v", err)
	}

	cipher := storedCipher(t, a, sv.ID)
	if cipher == "" {
		t.Fatal("未设主密码时密码也必须保存下来，不能为空")
	}
	if cipher == "s3cret!" {
		t.Fatal("密码不能明文落库")
	}

	// 锁定 → 密钥出内存
	if err := a.Lock(); err != nil {
		t.Fatalf("锁定失败: %v", err)
	}
	if !a.IsLocked() || a.IsUnlocked() {
		t.Fatal("锁定后应处于锁定态")
	}
	if got := storedCipher(t, a, sv.ID); got != cipher {
		t.Fatal("锁定不应改动已保存的密码")
	}

	// 一键解锁（未设主密码）→ 密码可解
	if err := a.UnlockWithoutPassword(); err != nil {
		t.Fatalf("一键解锁失败: %v", err)
	}
	got, err := a.getServerWithPassword(sv.ID)
	if err != nil {
		t.Fatalf("读取服务器失败: %v", err)
	}
	if got.Password != "s3cret!" {
		t.Fatalf("解锁后应取回原密码，got %q", got.Password)
	}
}

// TestUnlockWithoutPasswordRejectedWhenMasterSet 已设置主密码时不允许绕过主密码解锁
func TestUnlockWithoutPasswordRejectedWhenMasterSet(t *testing.T) {
	a := newTestApp(t)
	if err := a.SetMasterPassword("old123"); err != nil {
		t.Fatalf("设置主密码失败: %v", err)
	}
	if err := a.Lock(); err != nil {
		t.Fatalf("锁定失败: %v", err)
	}
	if err := a.UnlockWithoutPassword(); err == nil {
		t.Fatal("已设置主密码时必须走密码解锁")
	}
	if !a.IsLocked() {
		t.Fatal("拒绝后应仍处于锁定态")
	}
}

// TestLockedSaveRejectsPlaintext 锁定后没有可用密钥：保存密码必须报错，绝不明文落库
func TestLockedSaveRejectsPlaintext(t *testing.T) {
	a := newTestApp(t)
	if err := a.ensureMachineKey(); err != nil {
		t.Fatalf("初始化本机密钥失败: %v", err)
	}
	sv, err := a.SaveServer(models.Server{
		Name: "web-01", Host: "10.0.0.1", Port: 22, Username: "root", Password: "first-pass",
	})
	if err != nil {
		t.Fatalf("保存服务器失败: %v", err)
	}
	before := storedCipher(t, a, sv.ID)

	if err := a.Lock(); err != nil {
		t.Fatalf("锁定失败: %v", err)
	}

	// 带新密码保存 → 明确报错，库里保持原密文
	_, err = a.SaveServer(models.Server{
		ID: sv.ID, Name: "web-01", Host: "10.0.0.1", Port: 22, Username: "root", Password: "second-pass",
	})
	if err != errNoEncryptionKey {
		t.Fatalf("锁定后写密码应返回 errNoEncryptionKey，got %v", err)
	}
	if got := storedCipher(t, a, sv.ID); got != before {
		t.Fatal("报错时不应改动已保存的密码")
	}

	// 密码留空的编辑仍然允许（沿用原密文），不会把已有密码删掉
	if _, err := a.SaveServer(models.Server{
		ID: sv.ID, Name: "web-01-renamed", Host: "10.0.0.1", Port: 22, Username: "root",
	}); err != nil {
		t.Fatalf("留空密码的编辑不应失败: %v", err)
	}
	if got := storedCipher(t, a, sv.ID); got != before {
		t.Fatal("留空密码时必须沿用原密文")
	}
}

// TestChangeMasterPasswordReencrypts 重置主密码要把已存密码重新加密，不能把密码弄丢
func TestChangeMasterPasswordReencrypts(t *testing.T) {
	a := newTestApp(t)
	if err := a.SetMasterPassword("old123"); err != nil {
		t.Fatalf("设置主密码失败: %v", err)
	}
	sv, err := a.SaveServer(models.Server{
		Name: "web-01", Host: "10.0.0.1", Port: 22, Username: "root", Password: "s3cret!",
	})
	if err != nil {
		t.Fatalf("保存服务器失败: %v", err)
	}
	before := storedCipher(t, a, sv.ID)

	// 旧密码不对：拒绝且不改动任何东西
	if _, err := a.ChangeMasterPassword("wrong", "new123"); err == nil {
		t.Fatal("当前主密码不正确时应报错")
	}
	if got := storedCipher(t, a, sv.ID); got != before {
		t.Fatal("校验失败时不应改动密码密文")
	}

	res, err := a.ChangeMasterPassword("old123", "new123")
	if err != nil {
		t.Fatalf("重置主密码失败: %v", err)
	}
	if got := intOf(t, res, "reencrypted"); got != 1 {
		t.Fatalf("应有 1 条重新加密，got %d（%+v）", got, res)
	}
	if got := intOf(t, res, "failed"); got != 0 {
		t.Fatalf("不应有失败条目，got %d（%+v）", got, res)
	}
	if got := storedCipher(t, a, sv.ID); got == before || got == "s3cret!" {
		t.Fatal("密文应已用新密钥重写")
	}

	// 新密码可解锁，且密码原文仍可解出；旧密码失效
	if err := a.Lock(); err != nil {
		t.Fatalf("锁定失败: %v", err)
	}
	if ok, _ := a.VerifyMasterPassword("old123"); ok {
		t.Fatal("旧主密码应失效")
	}
	ok, err := a.VerifyMasterPassword("new123")
	if err != nil || !ok {
		t.Fatalf("新主密码应可解锁: ok=%v err=%v", ok, err)
	}
	got, err := a.getServerWithPassword(sv.ID)
	if err != nil {
		t.Fatalf("读取服务器失败: %v", err)
	}
	if got.Password != "s3cret!" {
		t.Fatalf("重置主密码后密码不应丢失，got %q", got.Password)
	}

	// 未设置主密码时不允许走"重置"
	b := newTestApp(t)
	if _, err := b.ChangeMasterPassword("x", "y"); err == nil {
		t.Fatal("尚未设置主密码时应提示改用设置主密码")
	}
}

// TestSetMasterPasswordMigratesMachineKeyCipher 先无主密码保存、后设置主密码：
// 设置时会用主密钥重新加密，之后重启锁定也能解出原密码
func TestSetMasterPasswordMigratesMachineKeyCipher(t *testing.T) {
	a := newTestApp(t)
	if err := a.ensureMachineKey(); err != nil {
		t.Fatalf("初始化本机密钥失败: %v", err)
	}
	sv, err := a.SaveServer(models.Server{
		Name: "web-01", Host: "10.0.0.1", Port: 22, Username: "root", Password: "s3cret!",
	})
	if err != nil {
		t.Fatalf("保存服务器失败: %v", err)
	}
	machineCipher := storedCipher(t, a, sv.ID)

	if err := a.SetMasterPassword("master123"); err != nil {
		t.Fatalf("设置主密码失败: %v", err)
	}
	if got := storedCipher(t, a, sv.ID); got == machineCipher {
		t.Fatal("设置主密码后应改用主密钥重新加密")
	}
	if err := a.Lock(); err != nil {
		t.Fatalf("锁定失败: %v", err)
	}
	if ok, _ := a.VerifyMasterPassword("master123"); !ok {
		t.Fatal("主密码应可解锁")
	}
	got, err := a.getServerWithPassword(sv.ID)
	if err != nil {
		t.Fatalf("读取服务器失败: %v", err)
	}
	if got.Password != "s3cret!" {
		t.Fatalf("设置主密码后密码不应丢失，got %q", got.Password)
	}
}
