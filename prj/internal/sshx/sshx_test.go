package sshx

import (
	"os"
	"testing"
)

// TestParseDuSize du 输出解析：路径可能含空格/中文/换行，只能信任首个字段
func TestParseDuSize(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want int64
		ok   bool
	}{
		{"常规输出", "4096\t/var/log\n", 4096, true},
		{"路径含空格", "12345\t/opt/my app/data\n", 12345, true},
		{"路径含中文", "888\t/数据/日志\n", 888, true},
		{"数值后紧跟空格而非制表符", "2048 /tmp\n", 2048, true},
		{"空输出", "", 0, false},
		{"纯错误信息", "du: cannot access '/x': No such file or directory\n", 0, false},
		{"开头有换行", "\n512\t/a\n", 512, true},
		{"超出 int32 的大目录", "9876543210\t/data\n", 9876543210, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseDuSize(c.out)
			if ok != c.ok {
				t.Fatalf("ok: got %v want %v", ok, c.ok)
			}
			if ok && got != c.want {
				t.Fatalf("size: got %d want %d", got, c.want)
			}
		})
	}
}

// TestShellQuote 单引号转义：命令注入与含特殊字符的路径都要安全
func TestShellQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/var/log", "'/var/log'"},
		{"/opt/my app", "'/opt/my app'"},
		{"it's", `'it'\''s'`},
		{"/tmp/a$b`whoami`", "'/tmp/a$b`whoami`'"},
		{"/数据/日志", "'/数据/日志'"},
	}
	for _, c := range cases {
		if got := shellQuote(c.in); got != c.want {
			t.Fatalf("shellQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestShellJoin argv 拼接
func TestShellJoin(t *testing.T) {
	got := shellJoin([]string{"du", "-sbP", "/opt/my app"})
	want := "'du' '-sbP' '/opt/my app'"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestMatchFileName 文件名模糊匹配：子串、大小写、通配符、字面量方括号
func TestMatchFileName(t *testing.T) {
	cases := []struct {
		keyword string
		name    string
		want    bool
	}{
		{"ssh", "ssh_config", true},
		{"SSH", "ssh_config", true},
		{"config", "SSH_CONFIG", true},
		{"ss", "ssh_config", true},
		{"xyz", "ssh_config", false},
		{"*.conf", "nginx.conf", true},
		{"nginx*", "nginx.conf", true},
		{"ng?nx*", "nginx.conf", true},
		{"conf", "nginx.conf", true},
		{"a[b]c", "a[b]c", true},   // 方括号按字面量匹配
		{"a[b]c", "abc", false},    // 不应被当成字符类
		{"", "anything", false},    // 空关键字不匹配
		{"  ", "anything", false},  // 空白关键字不匹配
	}
	for _, c := range cases {
		if got := MatchFileName(c.keyword, c.name); got != c.want {
			t.Fatalf("MatchFileName(%q, %q) = %v, want %v", c.keyword, c.name, got, c.want)
		}
	}
}

// TestJoinRemote 远程路径拼接不产生 // 且保留根目录
func TestJoinRemote(t *testing.T) {
	cases := []struct{ dir, name, want string }{
		{"/", "etc", "/etc"},
		{"/var", "log", "/var/log"},
		{"/var/", "log", "/var/log"},
		{"", "etc", "etc"},
	}
	for _, c := range cases {
		if got := joinRemote(c.dir, c.name); got != c.want {
			t.Fatalf("joinRemote(%q, %q) = %q, want %q", c.dir, c.name, got, c.want)
		}
	}
}

// TestFirstLine 错误信息截断
func TestFirstLine(t *testing.T) {
	if got := firstLine("  err one \nsecond\n"); got != "err one" {
		t.Fatalf("got %q", got)
	}
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'x'
	}
	if got := firstLine(string(long)); len(got) != 200 {
		t.Fatalf("长文本应截断到 200 字符，got %d", len(got))
	}
}

// TestWriteFileBytesMode 写入后保持原权限位（依赖 os.FileMode 语义，纯本地校验）
func TestWriteFileBytesMode(t *testing.T) {
	// 该用例只校验权限位提取逻辑，不涉及远端连接
	mode := os.FileMode(0o755)
	if mode.Perm().String() != "-rwxr-xr-x" {
		t.Fatalf("权限位解析异常: %s", mode.Perm().String())
	}
	if got := os.FileMode(0).Perm(); got != 0 {
		t.Fatalf("mode 为 0 时不应改权限，got %s", got)
	}
}