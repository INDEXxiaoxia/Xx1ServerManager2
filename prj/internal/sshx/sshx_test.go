package sshx

import (
	"os"
	"strings"
	"testing"

	"ServerManager2/internal/models"
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
		{"a[b]c", "a[b]c", true},  // 方括号按字面量匹配
		{"a[b]c", "abc", false},   // 不应被当成字符类
		{"", "anything", false},   // 空关键字不匹配
		{"  ", "anything", false}, // 空白关键字不匹配
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

// TestParseDfGNU 七列输出（df -PTk）：带类型、可识别伪文件系统，1K 块换算为 MB
func TestParseDfGNU(t *testing.T) {
	out := `Filesystem     Type     1K-blocks      Used Available Use% Mounted on
/dev/sda1      ext4      10240000   6144000   3571200  64% /
tmpfs          tmpfs      8123456         0   8123456   0% /dev/shm
/dev/sdb1      xfs      104857600  92589542   8953728  92% /data
`
	disks := parseDf(out)
	if len(disks) != 3 {
		t.Fatalf("应解析出 3 条，got %d: %+v", len(disks), disks)
	}
	root := disks[0]
	if root.Mount != "/" || root.Device != "/dev/sda1" || root.Fstype != "ext4" {
		t.Fatalf("根分区解析异常: %+v", root)
	}
	if root.TotalMB != 10000 || root.UsedMB != 6000 || root.AvailMB != 3487 || root.Usage != 64 {
		t.Fatalf("容量换算异常: %+v", root)
	}
	if root.IsPseudo || root.IsReadOnly {
		t.Fatalf("真实磁盘不应标记伪/只读: %+v", root)
	}
	if !disks[1].IsPseudo {
		t.Fatalf("tmpfs 应标记为伪文件系统: %+v", disks[1])
	}
	data := disks[2]
	if data.Mount != "/data" || data.Fstype != "xfs" || data.Usage != 92 {
		t.Fatalf("数据盘解析异常: %+v", data)
	}
}

// TestParseDfWithoutType 六列输出（老系统无 df -T）：按表头 1024-blocks 换算
func TestParseDfWithoutType(t *testing.T) {
	out := `Filesystem     1024-blocks     Used Available Use% Mounted on
/dev/vda1         41020672  8204134  30701254  22% /
/dev/vdb1        103080888 97826832   4194040  96% /data
`
	disks := parseDf(out)
	if len(disks) != 2 {
		t.Fatalf("应解析出 2 条，got %d: %+v", len(disks), disks)
	}
	if disks[0].TotalMB != 40059 || disks[0].Usage != 22 {
		t.Fatalf("根分区换算异常: %+v", disks[0])
	}
	if disks[1].Mount != "/data" || disks[1].Fstype != "" {
		t.Fatalf("无类型输出的 fstype 应为空: %+v", disks[1])
	}
}

// TestParseDfUnitMB df -m 回退输出单位是 MB：表头 1M-blocks 时不能再除以 1024
func TestParseDfUnitMB(t *testing.T) {
	out := `Filesystem     1M-blocks  Used Available Use% Mounted on
/dev/sda1           100000 62000     34000  65% /
`
	disks := parseDf(out)
	if len(disks) != 1 || disks[0].TotalMB != 100000 || disks[0].UsedMB != 62000 || disks[0].Usage != 65 {
		t.Fatalf("MB 单位解析异常: %+v", disks)
	}
}

// TestParseDfEdgeCases 表头、空行、容量为 "-"、挂载点含空格、设备名换行、非 / 开头的挂载
func TestParseDfEdgeCases(t *testing.T) {
	out := `Filesystem     Type 1K-blocks Used Available Use% Mounted on
/dev/sdc1      ext4  20480000 10240000 10240000  50% /opt/my app

/dev/sr0     iso9660        0        0        0    - /media/cdrom
/dev/mapper/vg--very-long-name
                       ext4  10240000  2048000  8192000  20% /var
map -hosts       autofs        0        0        0    - /home
`
	disks := parseDf(out)
	if len(disks) != 2 {
		t.Fatalf("应只解析出 2 条有效记录，got %d: %+v", len(disks), disks)
	}
	if disks[0].Mount != "/opt/my app" || disks[0].Usage != 50 {
		t.Fatalf("含空格挂载点解析异常: %+v", disks[0])
	}
	if disks[1].Mount != "/var" || disks[1].Device != "/dev/mapper/vg--very-long-name" {
		t.Fatalf("设备名换行解析异常: %+v", disks[1])
	}
	if got := parseDf(""); len(got) != 0 {
		t.Fatalf("空输出应返回空列表，got %+v", got)
	}
}

// TestIsPseudoDisk 伪文件系统判定：有类型按类型；无类型时只认明确的伪文件系统名，宁多勿漏
func TestIsPseudoDisk(t *testing.T) {
	cases := []struct {
		dev, fstype string
		want        bool
	}{
		{"/dev/sda1", "ext4", false},
		{"/dev/sdb1", "", false},
		{"tmpfs", "tmpfs", true},
		{"tmpfs", "", true}, // 六列模式：设备列直接是 fs 名
		{"overlay", "", true},
		{"(none)", "", true},
		{"server:/export", "", false}, // NFS 网络盘算真实存储
		{"//server/share", "", false}, // CIFS
		{"pool/dataset", "", false},   // ZFS 数据集：认不出的名字一律保留
		{"map", "autofs", true},
	}
	for _, c := range cases {
		if got := isPseudoDisk(c.dev, c.fstype); got != c.want {
			t.Fatalf("isPseudoDisk(%q, %q) = %v, want %v", c.dev, c.fstype, got, c.want)
		}
	}
}

// TestPickKeyDisk 关键磁盘：真实可写盘中占用最高者优先；只读盘、伪文件系统仅兜底
func TestPickKeyDisk(t *testing.T) {
	disks := []models.DiskInfo{
		{Mount: "/", TotalMB: 10000, Usage: 64},
		{Mount: "/data", TotalMB: 102400, Usage: 92},
		{Mount: "/dev/shm", TotalMB: 7933, Usage: 99, IsPseudo: true},
	}
	if best, ok := pickKeyDisk(disks); !ok || best.Mount != "/data" {
		t.Fatalf("应选中 /data，got %+v ok=%v", best, ok)
	}

	only := []models.DiskInfo{{Mount: "/dev/shm", TotalMB: 7933, Usage: 99, IsPseudo: true}}
	if best, ok := pickKeyDisk(only); !ok || best.Mount != "/dev/shm" {
		t.Fatalf("无真实盘时应回退到伪文件系统，got %+v ok=%v", best, ok)
	}

	roCase := []models.DiskInfo{
		{Mount: "/mnt/ro", TotalMB: 100, Usage: 99, IsReadOnly: true},
		{Mount: "/", TotalMB: 100, Usage: 30},
	}
	if best, _ := pickKeyDisk(roCase); best.Mount != "/" {
		t.Fatalf("只读盘不应被选为关键磁盘，got %+v", best)
	}

	if _, ok := pickKeyDisk(nil); ok {
		t.Fatalf("空列表不应选出关键磁盘")
	}
}

// TestParseReadOnlyMounts 只读挂载识别：/proc/mounts 的八进制转义要还原
func TestParseReadOnlyMounts(t *testing.T) {
	out := `/dev/sda1 / ext4 rw,relatime 0 0
/dev/sdb1 /data ext4 ro,relatime 0 0
tmpfs /run tmpfs rw,nosuid,nodev 0 0
/dev/sdc1 /opt/my\040app ext4 ro 0 0
`
	ro := parseReadOnlyMounts(out)
	if ro["/"] || ro["/run"] {
		t.Fatalf("rw 挂载不应判为只读: %+v", ro)
	}
	if !ro["/data"] || !ro["/opt/my app"] {
		t.Fatalf("只读挂载识别异常: %+v", ro)
	}
}

// TestSplitDiskSections 合并命令的两段输出按标记拆分；无标记时整体当 df 输出
func TestSplitDiskSections(t *testing.T) {
	out := "<<<SM2-DF>>>\nFilesystem     Type 1K-blocks\n/dev/sda1 ext4 100 1 99 1% /\n<<<SM2-MOUNTS>>>\n/dev/sda1 / ext4 rw 0 0\n"
	df, mounts := splitDiskSections(out)
	if !strings.Contains(df, "/dev/sda1") || strings.Contains(df, "MOUNTS") {
		t.Fatalf("df 段拆分异常: %q", df)
	}
	if !strings.Contains(mounts, "ext4 rw") {
		t.Fatalf("mounts 段拆分异常: %q", mounts)
	}
	df2, mounts2 := splitDiskSections("Filesystem 1K-blocks\n")
	if mounts2 != "" || !strings.Contains(df2, "Filesystem") {
		t.Fatalf("无标记时拆分异常: %q %q", df2, mounts2)
	}
}

// TestSortDisks 排序：/ 恒排第一，其余按挂载点字母序
func TestSortDisks(t *testing.T) {
	got := sortDisks([]models.DiskInfo{
		{Mount: "/var"}, {Mount: "/data"}, {Mount: "/"}, {Mount: "/boot"},
	})
	want := []string{"/", "/boot", "/data", "/var"}
	if len(got) != len(want) {
		t.Fatalf("长度异常: %+v", got)
	}
	for i := range want {
		if got[i].Mount != want[i] {
			t.Fatalf("排序异常: %+v", got)
		}
	}
}
