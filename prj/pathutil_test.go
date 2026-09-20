package main

import "testing"

// TestCleanRemotePath 路径规范化：折叠斜杠、去尾斜杠、解析 . 与 ..
func TestCleanRemotePath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/", "/"},
		{"", "/"},
		{"/etc", "/etc"},
		{"/etc/", "/etc"},
		{"/etc//ssh/", "/etc/ssh"},
		{"/var/../etc", "/etc"},
		{"/a/b/./c", "/a/b/c"},
		{"  /tmp  ", "/tmp"},
		{"/opt\\myapp", "/opt/myapp"},
		{".", "/"},
	}
	for _, c := range cases {
		if got := cleanRemotePath(c.in); got != c.want {
			t.Fatalf("cleanRemotePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestRemoteAb 相对路径以基准目录解析
func TestRemoteAb(t *testing.T) {
	cases := []struct{ base, in, want string }{
		{"/home/xx1", "logs", "/home/xx1/logs"},
		{"/home/xx1", "/etc", "/etc"},
		{"/home/xx1", "/etc/", "/etc"},
		{"/home/xx1", "", "/home/xx1"},
		{"/home/xx1", "../other", "/home/other"},
	}
	for _, c := range cases {
		if got := remoteAbs(c.base, c.in); got != c.want {
			t.Fatalf("remoteAbs(%q, %q) = %q, want %q", c.base, c.in, got, c.want)
		}
	}
}

// TestJoinRemotePath 拼接不产生 // 与丢失根目录
func TestJoinRemotePath(t *testing.T) {
	cases := []struct{ dir, name, want string }{
		{"/", "etc", "/etc"},
		{"", "etc", "/etc"},
		{"/var", "log", "/var/log"},
		{"/var/", "log", "/var/log"},
		{"/var", "/log", "/var/log"},
	}
	for _, c := range cases {
		if got := joinRemotePath(c.dir, c.name); got != c.want {
			t.Fatalf("joinRemotePath(%q, %q) = %q, want %q", c.dir, c.name, got, c.want)
		}
	}
}

// TestDirNameRemote 父目录解析
func TestDirNameRemote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/", "/"},
		{"/etc", "/"},
		{"/etc/ssh", "/etc"},
		{"/etc/ssh/", "/etc"},
		{"/var/log/nginx/access.log", "/var/log/nginx"},
	}
	for _, c := range cases {
		if got := dirNameRemote(c.in); got != c.want {
			t.Fatalf("dirNameRemote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestBaseNameRemote 文件名解析
func TestBaseNameRemote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/", "/"},
		{"/etc/ssh_config", "ssh_config"},
		{"/etc/ssh/", "ssh"},
		{"中文文件.txt", "中文文件.txt"},
	}
	for _, c := range cases {
		if got := baseNameRemote(c.in); got != c.want {
			t.Fatalf("baseNameRemote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestPathBreadcrumb 面包屑逐级路径
func TestPathBreadcrumb(t *testing.T) {
	if got := pathBreadcrumb("/"); len(got) != 0 {
		t.Fatalf("根目录不应有面包屑片段: %v", got)
	}
	got := pathBreadcrumb("/var/log/nginx")
	want := []struct{ name, path string }{
		{"var", "/var"},
		{"log", "/var/log"},
		{"nginx", "/var/log/nginx"},
	}
	if len(got) != len(want) {
		t.Fatalf("片段数量不符: %d", len(got))
	}
	for i, w := range want {
		if got[i]["name"] != w.name || got[i]["path"] != w.path {
			t.Fatalf("第 %d 段不符: %v", i, got[i])
		}
	}
}

// TestShellSafeWord 仅保留安全字符（用于 ~user 拼接）
func TestShellSafeWord(t *testing.T) {
	if got := shellSafeWord("root; rm -rf /"); got != "rootrm-rf" {
		t.Fatalf("危险字符未被过滤: %q", got)
	}
	if got := shellSafeWord("deploy_user-1.2"); got != "deploy_user-1.2" {
		t.Fatalf("安全字符被错误过滤: %q", got)
	}
}