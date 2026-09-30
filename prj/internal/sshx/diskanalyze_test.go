package sshx

import (
	"strings"
	"testing"

	"ServerManager2/internal/models"
)

// ---- 分节解析 ----

func TestSplitMarkedSections(t *testing.T) {
	out := "X1SA1\r\nhello\r\nX1SA2\r\nworld\r\nline2\r\nX1SAEND\r\n"
	secs := splitMarkedSections(out, []string{"X1SA1", "X1SA2", "X1SA3", "X1SAEND"})
	if got := strings.TrimSpace(secs["X1SA1"]); got != "hello" {
		t.Fatalf("X1SA1 = %q, want hello", got)
	}
	if got := strings.TrimSpace(secs["X1SA2"]); got != "world\nline2" {
		t.Fatalf("X1SA2 = %q", got)
	}
	if _, ok := secs["X1SA3"]; ok {
		t.Fatalf("未出现的标记不应产生键")
	}
	// 标记之前的内容不属于任何节
	if strings.Contains(secs["X1SA1"], "X1SA1") {
		t.Fatalf("标记本身不应进入节内容")
	}
}

// 脚本每一行都必须"单行自洽"：buildSudoCommand 按行给命令加 `echo 'pw' | sudo -S` 前缀，
// 行首若是 shell 关键字（if/for/while/case/{）会在 sudo 模式下被拆坏。
func TestDiskScriptsAreSudoLineSafe(t *testing.T) {
	keyword := func(line string) bool {
		switch strings.Fields(line)[0] {
		case "if", "then", "else", "fi", "for", "do", "done", "while", "case", "esac", "{", "}", "(", ")", "sudo":
			return true
		}
		return false
	}
	cmds := map[string]string{
		"quickscan": buildDiskQuickscanCommand("/"),
		"volume":    buildDiskVolumeScanCommand("/data"),
		"drill":     buildDiskDrillCommand("/data/x"),
		"duFallbk":  buildDuMaxDepthFallback("/data"),
		"findFallb": buildBigFileLsFallback("/data"),
		"deleted":   buildDeletedFallback(),
	}
	for name, cmd := range cmds {
		if cmd == "" {
			t.Fatalf("%s: 命令为空", name)
		}
		for _, line := range strings.Split(cmd, "\n") {
			if strings.TrimSpace(line) == "" {
				t.Fatalf("%s: 出现空行（会被 sudo 包装成空命令）", name)
			}
			if keyword(line) {
				t.Fatalf("%s: 行首是 shell 关键字，sudo 包装后会被拆坏: %q", name, line)
			}
			if strings.HasPrefix(line, "#") {
				t.Fatalf("%s: 注释行没有意义: %q", name, line)
			}
		}
	}
}

func TestDiskCommandsQuoteMountPath(t *testing.T) {
	// 路径含空格与单引号时要正确转义
	mount := "/data/my dir's"
	cmd := buildDiskVolumeScanCommand(mount)
	if !strings.Contains(cmd, `'/data/my dir'\''s'`) {
		t.Fatalf("挂载点未被单引号转义: %s", cmd)
	}
	if strings.Contains(cmd, "\n\n") {
		t.Fatalf("不应出现空行")
	}
}

// sudo 包装按行加前缀，密码里的单引号若未转义，整条命令都会被 shell 拆坏
func TestBuildSudoCommandEscapesPassword(t *testing.T) {
	got := buildSudoCommand("du -x -d1 -k /data", "p'a$$w")
	if !strings.Contains(got, `echo 'p'\''a$$w' | sudo -S du`) {
		t.Fatalf("密码未被正确转义: %s", got)
	}
	// 多行脚本逐行包装
	multi := buildSudoCommand("echo 'X1SA1'\ndf -PTk /", "pw")
	if strings.Count(multi, "sudo -S") != 2 {
		t.Fatalf("多行脚本应逐行包装: %s", multi)
	}
	if !strings.Contains(multi, "sudo -S echo 'X1SA1'") {
		t.Fatalf("标记行也应被包装（否则 sudo 模式下标记会丢）: %s", multi)
	}
}

// ---- inode ----

func TestParseDfInodes(t *testing.T) {
	gnu := "Filesystem     Inodes  IUsed   IFree IUse% Mounted on\n" +
		"/dev/sda1     6553600 5898240  655360   90% /\n"
	total, used, pct, ok := parseDfInodes(gnu)
	if !ok || total != 6553600 || used != 5898240 || pct != 90 {
		t.Fatalf("gnu inode 解析错误: total=%d used=%d pct=%v ok=%v", total, used, pct, ok)
	}

	// 设备名过长时 GNU df 会把设备单独放一行
	wrapped := "Filesystem        Inodes IUsed IFree IUse% Mounted on\n" +
		"/dev/mapper/vg--data-lv--data\n" +
		"                  1310720 1310720 0 100% /data\n"
	total, used, pct, ok = parseDfInodes(wrapped)
	if !ok || total != 1310720 || used != 1310720 || pct != 100 {
		t.Fatalf("换行设备名解析错误: total=%d used=%d pct=%v ok=%v", total, used, pct, ok)
	}

	// 表头被换成中文时也要能跳过
	zh := "文件系统        Inodes   IUsed    IFree IUse% 挂载点\n/dev/sdb1      2621440  262144  2359296   10% /data\n"
	if _, _, pct, ok := parseDfInodes(zh); !ok || pct != 10 {
		t.Fatalf("中文表头解析错误: pct=%v ok=%v", pct, ok)
	}

	// 该文件系统不统计 inode（容量列是 "-"）
	if _, _, _, ok := parseDfInodes("Filesystem Inodes IUsed IFree IUse% Mounted on\n/dev/sdc1 - - - - /mnt\n"); ok {
		t.Fatalf("inode 为 - 时不应判定为有效")
	}
	// 只有表头
	if _, _, _, ok := parseDfInodes("Filesystem Inodes IUsed IFree IUse% Mounted on\n"); ok {
		t.Fatalf("只有表头时不应判定为有效")
	}
}

// parseDf 会把 df -Pi 的六列当成"1K 块容量"，得到 total=inodes/1024 的假磁盘。
// 这里把这个陷阱固定下来：inode 段必须走 parseDfInodes，绝不能被 parseDf 处理。
// 若哪天 parseDf 的行为变了，本用例会先失败，提醒重新确认 inode 解析路径。
func TestParseDfWouldMisparseInodeOutput(t *testing.T) {
	out := "Filesystem Inodes IUsed IFree IUse% Mounted on\n/dev/sda1 6553600 5898240 655360 90% /\n"
	disks := parseDf(out)
	if len(disks) != 1 || disks[0].TotalMB != 6553600/1024 {
		t.Fatalf("前提已变：parseDf 对 df -Pi 输出的解析结果与预期不同（%+v），请重新确认 inode 解析路径", disks)
	}
}

// ---- 体积行解析 ----

func TestParseDuDepth1(t *testing.T) {
	base := "/data"
	out := "409600\t/data\n" +
		"204800\t/opt/models\n" + // 不应出现，但解析器不关心是否在 base 下
		"12288\t/data/secret\n" +
		"du: cannot read directory '/data/private': Permission denied\n" +
		"iso\n" + // 首字段不是整数的其它杂行
		"1024\t/data/sub\n"
	items, total := parseDuDepth1(out, base, 400000)
	if len(items) != 3 {
		t.Fatalf("应解析出 3 项，得到 %d：%+v", len(items), items)
	}
	// 基准目录自身的合计行必须被排除
	for _, it := range items {
		if it.Path == base {
			t.Fatalf("基准目录自身的合计行不应出现在子项里")
		}
	}
	// 按体积降序
	if items[0].SizeMB != 200 || items[1].SizeMB != 12 || items[2].SizeMB != 1 {
		t.Fatalf("排序/换算错误：%+v", items)
	}
	if total != 213 {
		t.Fatalf("合计 = %d, want 213", total)
	}
	if items[0].Percent != 0.05 {
		t.Fatalf("占比 = %v, want 0.05", items[0].Percent)
	}
	if !items[0].HasChildren {
		t.Fatalf("du -d1 的每一项都是目录，HasChildren 应为 true")
	}
}

func TestParseFindSizeLines(t *testing.T) {
	out := "5368709120 /var/lib/docker/containers/abc/abc-json.log\n" +
		"1073741824 /opt/models/model.bin\n" +
		"find: '/root/.cache': Permission denied\n" +
		"0 /var/log/empty.log\n"
	items, _ := parseFindSizeLines(out, 100000)
	if len(items) != 2 {
		t.Fatalf("应解析出 2 项（0 字节与报错行都跳过），得到 %d：%+v", len(items), items)
	}
	if items[0].Path != "/var/lib/docker/containers/abc/abc-json.log" || items[0].SizeMB != 5120 {
		t.Fatalf("首项错误：%+v", items[0])
	}
	if items[1].SizeMB != 1024 {
		t.Fatalf("体积换算错误：%+v", items[1])
	}
}

func TestParseSizePathLinesSkipsGarbage(t *testing.T) {
	items, total := parseSizePathLines("\n\n   \nnot a number /x\n", "", 1024, 0)
	if len(items) != 0 || total != 0 {
		t.Fatalf("垃圾输入应得到空结果，得到 %+v", items)
	}
}

// ---- 体积单位 ----

func TestParseHumanSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"1.2G", 1288490188, true},
		{"200M", 209715200, true},
		{"1.5GiB", 1610612736, true},
		{"12kB", 12288, true},
		{"0B", 0, true},
		{"12345", 12345, true},
		{"2.0T", 2199023255552, true},
		{"(50%)", 0, false},
		{"50%", 0, false},
		{"abc", 0, false},
		{"", 0, false},
		{"-5M", 0, false},
	}
	for _, c := range cases {
		got, ok := parseHumanSize(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseHumanSize(%q) = %d,%v want %d,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestParseJournalUsage(t *testing.T) {
	if mb, ok := parseJournalUsage("Archived and active journals take up 1.2G in the file system."); !ok || mb != 1228 {
		t.Fatalf("新版输出解析错误: %d,%v", mb, ok)
	}
	if mb, ok := parseJournalUsage("Journal is using 512M."); !ok || mb != 512 {
		t.Fatalf("旧版输出解析错误: %d,%v", mb, ok)
	}
	if _, ok := parseJournalUsage("X1NA"); ok {
		t.Fatalf("不可用占位不应解析成功")
	}
}

func TestParseDockerDf(t *testing.T) {
	out := "TYPE            TOTAL     ACTIVE    SIZE      RECLAIMABLE\n" +
		"Images          18        4         12.5GB    7.2GB (57%)\n" +
		"Containers      4         3         850MB     12MB (1%)\n" +
		"Local Volumes   9         2         3.2GB     3.2GB (100%)\n" +
		"Build Cache     62        0         1.8GB     1.8GB (100%)\n"
	du, ok := parseDockerDf(out)
	if !ok {
		t.Fatalf("应解析成功")
	}
	if du.ImagesMB != 12800 {
		t.Errorf("Images = %d, want 12800", du.ImagesMB)
	}
	if du.ContainersMB != 850 {
		t.Errorf("Containers = %d, want 850", du.ContainersMB)
	}
	if du.ReclaimableMB != 7372+12+3276+1843 {
		t.Errorf("Reclaimable = %d, want %d", du.ReclaimableMB, 7372+12+3276+1843)
	}
	// TOTAL/ACTIVE 是纯数字，不能被当成体积
	if du.ImagesMB == 18 {
		t.Fatalf("把 TOTAL 计数当成了体积")
	}
	if _, ok := parseDockerDf("X1NA"); ok {
		t.Fatalf("不可用占位不应解析成功")
	}
}

// ---- 已删除句柄 ----

func TestParseLsofDeleted(t *testing.T) {
	out := "COMMAND     PID USER   FD   TYPE DEVICE SIZE/OFF NLINK    NODE NAME\n" +
		"nginx      1234 root    5u   REG  253,0 5368709120     0  123456 /var/log/nginx/access.log (deleted)\n" +
		"java       5678 app     9w   REG  253,0  104857600     0  654321 /opt/app/app.log (deleted)\n" +
		"mysqld     9999 mysql   3u   REG  253,0   10485760     0  111111 /var/lib/mysql/ib_logfile0\n"
	items := parseLsofDeleted(out)
	if len(items) != 2 {
		t.Fatalf("应解析出 2 个已删除文件，得到 %d", len(items))
	}
	if items[0].Command != "nginx" || items[0].PID != "1234" || items[0].SizeMB != 5120 {
		t.Fatalf("首项错误：%+v", items[0])
	}
	if !strings.Contains(items[0].Path, "access.log") {
		t.Fatalf("路径错误：%q", items[0].Path)
	}
}

func TestParseDeletedFallbackPaths(t *testing.T) {
	out := "/proc/1234/fd/5\n/proc/1234/fd/7\nnot-a-path\n\n"
	items := parseDeletedFallbackPaths(out)
	if len(items) != 2 || !items[0].SizeUnknown {
		t.Fatalf("回退解析错误：%+v", items)
	}
}

// ---- 报告组装 ----

func sampleQuick() *DiskQuickscan {
	q := &DiskQuickscan{
		Disk: models.DiskInfo{
			Mount:   "/",
			Device:  "/dev/sda1",
			Fstype:  "ext4",
			TotalMB: 102400,
			UsedMB:  80179,
			AvailMB: 17131,
			Usage:   78.3,
		},
		DiskFound:  true,
		InodeTotal: 6553600,
		InodeUsed:  6225920,
		InodeUsage: 95,
		InodeOK:    true,
		JournalMB:  1228,
		JournalOK:  true,
		Docker:     DockerUsage{ImagesMB: 12800, ContainersMB: 850, ReclaimableMB: 7372},
		DockerOK:   true,
		Deleted: []DeletedFile{
			{Command: "nginx", PID: "1234", Path: "/var/log/nginx/access.log (deleted)", SizeMB: 5120},
		},
		Raw: map[string]string{},
	}
	return q
}

func sampleVolume() *VolumeScan {
	return &VolumeScan{
		TopDirs: []models.DiskDirItem{
			{Path: "/var/lib/docker", SizeMB: 25000, HasChildren: true},
			{Path: "/opt/models", SizeMB: 10000, HasChildren: true},
			{Path: "/var/log", SizeMB: 4198, HasChildren: true},
		},
		BigFiles: []models.DiskDirItem{
			{Path: "/var/lib/docker/containers/abc/abc-json.log", SizeMB: 5324},
			{Path: "/opt/models/llama.bin", SizeMB: 4096},
		},
		ScannedMB: 25000 + 10000 + 4198,
		Raw:       map[string]string{},
	}
}

func TestBuildDiskReportFull(t *testing.T) {
	vol := sampleVolume()
	rep := BuildDiskReport("task-1", "srv-1", "测试机", "/", sampleQuick(), vol, 1234)

	if rep.Stage != "done" || rep.Partial {
		t.Fatalf("完整报告应为 done 且非 partial：%s %v", rep.Stage, rep.Partial)
	}
	if rep.TotalMB != 102400 || rep.UsedMB != 80179 {
		t.Fatalf("容量字段未从 quickscan 带入：%+v", rep)
	}
	if rep.GapMB != 80179-vol.ScannedMB {
		t.Fatalf("差额 = %d, want %d", rep.GapMB, 80179-vol.ScannedMB)
	}
	if rep.TopDirs[0].Percent == 0 {
		t.Fatalf("TopDirs 占比未按该盘已用容量回填")
	}

	kinds := map[string]models.DiskFinding{}
	for _, f := range rep.Findings {
		kinds[f.Kind] = f
	}
	for _, want := range []string{"inode", "deleted-open", "journal", "docker", "bigfile", "gap"} {
		if _, ok := kinds[want]; !ok {
			t.Fatalf("缺少 %s 结论：%+v", want, rep.Findings)
		}
	}
	if kinds["inode"].Level != "critical" {
		t.Errorf("inode 95%% 应为 critical，得到 %s", kinds["inode"].Level)
	}
	if kinds["deleted-open"].SizeMB != 5120 {
		t.Errorf("已删除句柄体积 = %d, want 5120", kinds["deleted-open"].SizeMB)
	}
	// 日志类大文件必须给 truncate 而不是 rm
	if !strings.Contains(kinds["bigfile"].Command, "truncate -s 0") &&
		!strings.Contains(kinds["bigfile"].Command, "ls -lh") {
		t.Errorf("大文件建议命令缺失：%q", kinds["bigfile"].Command)
	}
	var jsonLogFinding bool
	for _, f := range rep.Findings {
		if strings.Contains(f.Detail, "max-size") {
			jsonLogFinding = true
		}
	}
	if !jsonLogFinding {
		t.Errorf("容器 json 日志应给出日志轮转建议")
	}
	if rep.ReclaimableMB <= 0 {
		t.Errorf("可回收体积合计应为正数")
	}
	// 结论必须按严重程度排序
	if rep.Findings[0].Level != "critical" {
		t.Errorf("第一条结论应为 critical，得到 %s", rep.Findings[0].Level)
	}
}

func TestBuildDiskReportQuickscanOnly(t *testing.T) {
	rep := BuildDiskReport("task-2", "srv-1", "测试机", "/", sampleQuick(), nil, 300)
	if rep.Stage != "quickscan" || !rep.Partial {
		t.Fatalf("体积段未完成时应为 quickscan + partial：%s %v", rep.Stage, rep.Partial)
	}
	if len(rep.TopDirs) != 0 {
		t.Fatalf("体积段未跑时不应有目录排行")
	}
	for _, f := range rep.Findings {
		if f.Kind == "gap" || f.Kind == "bigfile" {
			t.Fatalf("体积段未跑时不应有 gap/bigfile 结论")
		}
	}
}

func TestBuildDiskReportUnavailableTools(t *testing.T) {
	q := sampleQuick()
	q.JournalOK = false
	q.DockerOK = false
	q.InodeOK = false
	rep := BuildDiskReport("t", "s", "n", "/", q, sampleVolume(), 10)
	unavail := 0
	for _, f := range rep.Findings {
		if f.Level == "unavailable" {
			unavail++
		}
	}
	if unavail != 3 {
		t.Fatalf("三项检查不可用时应产出 3 条 unavailable，得到 %d：%+v", unavail, rep.Findings)
	}
}

func TestBuildDiskReportDeletedUnknownSize(t *testing.T) {
	q := sampleQuick()
	q.Deleted = []DeletedFile{{Path: "/proc/1234/fd/5", SizeUnknown: true}}
	q.LsofMissing = true
	rep := BuildDiskReport("t", "s", "n", "/", q, nil, 10)
	var found bool
	for _, f := range rep.Findings {
		if f.Kind == "deleted-open" {
			found = true
			if f.SizeMB != 0 {
				t.Errorf("体积未知时不应给出体积合计")
			}
			if !strings.Contains(f.Detail, "lsof") {
				t.Errorf("体积未知时应说明原因：%q", f.Detail)
			}
		}
	}
	if !found {
		t.Fatalf("应产出 deleted-open 结论")
	}
}

func TestBuildDiskReportPseudoAndReserved(t *testing.T) {
	q := sampleQuick()
	q.Disk.Fstype = "tmpfs"
	q.Disk.IsPseudo = true
	q.Disk.Usage = 100
	q.Disk.AvailMB = 512
	q.InodeOK = false
	rep := BuildDiskReport("t", "s", "n", "/dev/shm", q, nil, 10)
	var hasPseudo, hasReserved, hasUsage bool
	for _, f := range rep.Findings {
		switch f.Kind {
		case "pseudo":
			hasPseudo = true
		case "reserved":
			hasReserved = true
		case "usage":
			hasUsage = true
		}
	}
	if !hasPseudo || !hasReserved || !hasUsage {
		t.Fatalf("伪文件系统场景缺少结论：pseudo=%v reserved=%v usage=%v", hasPseudo, hasReserved, hasUsage)
	}
}

// 给用户的每条建议命令都必须说明「做什么、跑完会怎样」——否则"复制走自己跑"是有风险的
func TestEveryCommandHasNote(t *testing.T) {
	reports := []models.DiskReport{
		BuildDiskReport("t", "s", "n", "/", sampleQuick(), sampleVolume(), 1),
		BuildDiskReport("t", "s", "n", "/data", sampleQuick(), nil, 1),
	}
	// 一个全量场景：权限不足 + 容器 json 日志 + 大文件
	q := sampleQuick()
	q.Deleted = []DeletedFile{{Path: "/proc/1/fd/5", SizeUnknown: true}}
	v := sampleVolume()
	v.PermissionDenied = true
	reports = append(reports, BuildDiskReport("t", "s", "n", "/", q, v, 1))

	n := 0
	for _, r := range reports {
		for _, f := range r.Findings {
			if f.Command == "" {
				continue
			}
			n++
			if strings.TrimSpace(f.CommandNote) == "" {
				t.Errorf("结论 %q 给了命令却没说作用：%q", f.Title, f.Command)
			}
			if !strings.Contains(f.CommandNote, "作用") || !strings.Contains(f.CommandNote, "运行后") {
				t.Errorf("命令说明应包含「作用」与「运行后」两部分：%q", f.CommandNote)
			}
		}
	}
	if n == 0 {
		t.Fatalf("样本里没有任何带命令的结论，用例失去意义")
	}
}

func TestIsLogLikePath(t *testing.T) {
	yes := []string{
		"/var/log/nginx/access.log",
		"/var/lib/docker/containers/a/b-json.log",
		"/data/app/app.out",
		"/var/log/messages",
	}
	no := []string{"/opt/models/llama.bin", "/data/dump.sql", "/data/backup.tar.gz"}
	for _, p := range yes {
		if !isLogLikePath(p) {
			t.Errorf("%s 应判定为日志类", p)
		}
	}
	for _, p := range no {
		if isLogLikePath(p) {
			t.Errorf("%s 不应判定为日志类", p)
		}
	}
}

func TestNormalizeRemote(t *testing.T) {
	cases := map[string]string{
		"/":         "/",
		"/data/":    "/data",
		"/data":     "/data",
		"  /data  ": "/data",
		"data":      "data",
		"":          "",
	}
	for in, want := range cases {
		if got := normalizeRemote(in); got != want {
			t.Errorf("normalizeRemote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFmtMBAndPct(t *testing.T) {
	if got := fmtMB(5120); got != "5.00 GB" {
		t.Errorf("fmtMB(5120) = %q", got)
	}
	if got := fmtMB(512); got != "512 MB" {
		t.Errorf("fmtMB(512) = %q", got)
	}
	if got := fmtPct(92); got != "92%" {
		t.Errorf("fmtPct(92) = %q", got)
	}
	if got := fmtPct(92.6); got != "92.6%" {
		t.Errorf("fmtPct(92.6) = %q", got)
	}
	if got := fmtPct(100); got != "100%" {
		t.Errorf("fmtPct(100) = %q", got)
	}
}
