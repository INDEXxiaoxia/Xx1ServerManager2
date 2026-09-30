package main

import (
	"path/filepath"
	"testing"

	"ServerManager2/internal/crypto"
	"ServerManager2/internal/models"
	"ServerManager2/internal/store"
)

// newTestApp 构造只带存储与加密模块的 App：不启动 Wails，事件发送不会被触发
func newTestApp(t *testing.T) *App {
	t.Helper()
	st, err := store.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("建库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	a := NewApp()
	a.store = st
	a.crypto = crypto.New()
	return a
}

// addTestServer 新建一台服务器，返回 id
func addTestServer(t *testing.T, a *App, name string, groupIDs ...string) string {
	t.Helper()
	sv, err := a.SaveServer(models.Server{
		Name: name, Host: "10.0.0.1", Port: 22, Username: "root", GroupIDs: groupIDs,
	})
	if err != nil {
		t.Fatalf("保存服务器 %s 失败: %v", name, err)
	}
	return sv.ID
}

// addTestGroup 新建一个分组，返回 id
func addTestGroup(t *testing.T, a *App, name string) string {
	t.Helper()
	g, err := a.SaveGroup(models.Group{Name: name, Color: "#5780A9"})
	if err != nil {
		t.Fatalf("保存分组 %s 失败: %v", name, err)
	}
	return g.ID
}

// groupOf 读取某台服务器当前的生效分组 id
func groupOf(t *testing.T, a *App, serverID string) string {
	t.Helper()
	sv, err := a.store.GetServerByID(serverID)
	if err != nil || sv == nil {
		t.Fatalf("读取服务器失败: %v", err)
	}
	return effectiveGroupID(sv.GroupIDs)
}

func intOf(t *testing.T, res map[string]any, key string) int {
	t.Helper()
	v, ok := res[key]
	if !ok {
		t.Fatalf("结果缺少字段 %s: %+v", key, res)
	}
	n, ok := v.(int)
	if !ok {
		t.Fatalf("字段 %s 不是 int: %#v", key, v)
	}
	return n
}

// TestAssignGroupServersEditorSemantics 编辑器语义：勾选集合就是该分组的成员集合，
// 后写覆盖先写（别组成员被转移过来），未勾选的本组成员释放为未分组。
func TestAssignGroupServersEditorSemantics(t *testing.T) {
	a := newTestApp(t)
	ga := addTestGroup(t, a, "生产环境")
	gb := addTestGroup(t, a, "测试环境")

	s1 := addTestServer(t, a, "web-01", ga)
	s2 := addTestServer(t, a, "web-02", gb)
	s3 := addTestServer(t, a, "web-03")
	s4 := addTestServer(t, a, "web-04", ga)

	res, err := a.AssignGroupServers(ga, []string{s1, s2})
	if err != nil {
		t.Fatalf("批量分配失败: %v", err)
	}
	// s1 原本就在 A（无非改动），s2 从 B 转移过来（1 处冲突），s4 未勾选 → 释放
	if got := intOf(t, res, "changed"); got != 2 {
		t.Fatalf("changed 应为 2，got %d（%+v）", got, res)
	}
	if got := intOf(t, res, "movedIn"); got != 1 {
		t.Fatalf("movedIn 应为 1，got %d（%+v）", got, res)
	}
	if got := intOf(t, res, "released"); got != 1 {
		t.Fatalf("released 应为 1，got %d（%+v）", got, res)
	}
	if got := intOf(t, res, "conflicts"); got != 1 {
		t.Fatalf("conflicts 应为 1，got %d（%+v）", got, res)
	}

	if g := groupOf(t, a, s1); g != ga {
		t.Fatalf("s1 应留在 A，got %q", g)
	}
	if g := groupOf(t, a, s2); g != ga {
		t.Fatalf("s2 应被转移到 A，got %q", g)
	}
	if g := groupOf(t, a, s3); g != "" {
		t.Fatalf("s3 与本次分配无关，应保持未分组，got %q", g)
	}
	if g := groupOf(t, a, s4); g != "" {
		t.Fatalf("s4 未勾选，应释放为未分组，got %q", g)
	}

	// 再保存一次同样的集合：幂等，不应产生新的变更
	res, err = a.AssignGroupServers(ga, []string{s1, s2})
	if err != nil {
		t.Fatalf("重复分配失败: %v", err)
	}
	if got := intOf(t, res, "changed"); got != 0 {
		t.Fatalf("重复保存应无变更，got %d（%+v）", got, res)
	}
}

// TestAssignGroupServersUngroupedMode 空分组 id = 移出分组（供"移出分组"按钮使用）
func TestAssignGroupServersUngroupedMode(t *testing.T) {
	a := newTestApp(t)
	ga := addTestGroup(t, a, "生产环境")
	s1 := addTestServer(t, a, "web-01", ga)
	s2 := addTestServer(t, a, "web-02", ga)

	res, err := a.AssignGroupServers("", []string{s1})
	if err != nil {
		t.Fatalf("移出分组失败: %v", err)
	}
	if got := intOf(t, res, "released"); got != 1 {
		t.Fatalf("released 应为 1，got %d（%+v）", got, res)
	}
	if g := groupOf(t, a, s1); g != "" {
		t.Fatalf("s1 应变为未分组，got %q", g)
	}
	if g := groupOf(t, a, s2); g != ga {
		t.Fatalf("s2 不应受影响，got %q", g)
	}
}

// TestAssignGroupServersBadInput 分组不存在要报错；已删除的服务器 id 静默忽略
func TestAssignGroupServersBadInput(t *testing.T) {
	a := newTestApp(t)
	ga := addTestGroup(t, a, "生产环境")
	s1 := addTestServer(t, a, "web-01")

	if _, err := a.AssignGroupServers("not-exist", []string{s1}); err == nil {
		t.Fatal("分组不存在时应报错")
	}
	if g := groupOf(t, a, s1); g != "" {
		t.Fatalf("报错时不应写入任何分组，got %q", g)
	}

	res, err := a.AssignGroupServers(ga, []string{s1, "already-deleted"})
	if err != nil {
		t.Fatalf("含未知 id 时不应报错: %v", err)
	}
	if got := intOf(t, res, "changed"); got != 1 {
		t.Fatalf("只应写入已知的 1 台，got %d（%+v）", got, res)
	}
}

// TestDeleteGroupReleasesMembers 删除分组要把成员释放为未分组，不能留下悬空分组 id
func TestDeleteGroupReleasesMembers(t *testing.T) {
	a := newTestApp(t)
	ga := addTestGroup(t, a, "生产环境")
	gb := addTestGroup(t, a, "测试环境")
	s1 := addTestServer(t, a, "web-01", ga)
	s2 := addTestServer(t, a, "web-02", gb)

	released, err := a.DeleteGroup(ga)
	if err != nil {
		t.Fatalf("删除分组失败: %v", err)
	}
	if released != 1 {
		t.Fatalf("应释放 1 台，got %d", released)
	}
	if g := groupOf(t, a, s1); g != "" {
		t.Fatalf("s1 应变为未分组，got %q", g)
	}
	if g := groupOf(t, a, s2); g != gb {
		t.Fatalf("其他分组成员不应受影响，got %q", g)
	}
	groups, _ := a.ListGroups()
	if len(groups) != 1 || groups[0].ID != gb {
		t.Fatalf("分组应只剩一个，got %+v", groups)
	}
	if _, err := a.DeleteGroup("  "); err == nil {
		t.Fatal("空分组 id 应报错")
	}
}
