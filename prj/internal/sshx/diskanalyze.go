package sshx

import (
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"ServerManager2/internal/models"
)

// ---- 运维工具 · 磁盘空间检测 ----
//
// 设计要点：
//  1. 分两次 SSH 调用：第 1 段「瞬时检查」秒级返回（容量 / inode / 日志服务 / 已删除句柄），
//     第 2 段「体积段」随后跑 du 与大文件扫描。用户点完立刻能看到关键结论，不必干等 du。
//  2. 分节沿用 diskCommand()/splitDiskSections() 的既有范式：脚本自己 echo 出专用标记，
//     输出按标记切成若干节再解析。
//  3. 每一行脚本都必须「单行自洽」——因为 buildSudoCommand 是按行给命令加
//     `echo 'pw' | sudo -S ` 前缀的，跨行的 if/for/heredoc 会在 sudo 模式下被拆坏。
//     因此：不使用行首 shell 关键字，不使用 shell 变量，挂载点直接内插字面量。
//  4. 全部走管道，不落任何临时文件（磁盘已满时不能依赖写盘能力）。
//  5. 本模块只做分析，不执行任何处置动作。

// 分节标记
const (
	secQuickDF      = "X1SA1" // 目标盘容量
	secQuickInode   = "X1SA2" // 目标盘 inode
	secQuickMounts  = "X1SA3" // /proc/mounts
	secQuickJournal = "X1SA4" // journald 占用
	secQuickDocker  = "X1SA5" // docker system df
	secQuickDeleted = "X1SA6" // 已删除但仍被持有的文件
	secQuickEnd     = "X1SAEND"

	secVolProbe = "X1SB1" // 能力探测
	secVolDirs  = "X1SB2" // 一级目录占用
	secVolBig   = "X1SB3" // 专项大文件
	secVolAll   = "X1SB4" // 全盘大文件
	secVolEnd   = "X1SBEND"

	// secUnavailable 该项检查在此机器上不可用（未安装 / 无权限）
	secUnavailable = "X1NA"

	// 能力探测结果
	probeDuNew   = "X1DUOK"   // du 支持 -d1
	probeDuOld   = "X1DUOLD"  // 老版本 du，只能走 --max-depth
	probeFindNew = "X1FINDOK" // find 支持 -printf
	probeFindOld = "X1FINDOLD"
)

// 体积段的超时：大目录 du 可能很慢，给足但可取消
const (
	diskQuickscanTimeout = 30
	diskVolumeTimeout    = 90
	diskDrillTimeout     = 40
)

// 大文件阈值（字节）：专项扫描 100MB，全盘扫描 200MB
const (
	diskBigFileBytes = 100 * 1024 * 1024
	diskAllFileBytes = 200 * 1024 * 1024
)

// buildDiskQuickscanCommand 第 1 段：瞬时检查
func buildDiskQuickscanCommand(mount string) string {
	q := shellQuote(mount)
	lines := []string{
		"echo '" + secQuickDF + "'",
		// -P 保证 POSIX 输出格式（一行一个文件系统），-T 带类型；老 coreutils 不支持 -T 时逐级回退
		"df -PTk " + q + " 2>/dev/null || df -Pk " + q + " 2>/dev/null || df -m " + q + " 2>/dev/null",
		"echo '" + secQuickInode + "'",
		"df -Pi " + q + " 2>/dev/null || df -i " + q + " 2>/dev/null",
		"echo '" + secQuickMounts + "'",
		"cat /proc/mounts 2>/dev/null",
		"echo '" + secQuickJournal + "'",
		"journalctl --disk-usage 2>/dev/null || echo '" + secUnavailable + "'",
		"echo '" + secQuickDocker + "'",
		"docker system df 2>/dev/null || echo '" + secUnavailable + "'",
		"echo '" + secQuickDeleted + "'",
		// 已删除但被进程持有的文件：df 满而 du 干净时就是它
		"lsof +L1 2>/dev/null | head -40",
		"echo '" + secQuickEnd + "'",
	}
	return strings.Join(lines, "\n")
}

// diskBigFileScanPaths 专项大文件扫描的候选目录（只在存在时才会命中）
func diskBigFileScanPaths(mount string) []string {
	subs := []string{
		"/var/lib/docker/containers", // 容器 json 日志，最常见的"5G 大日志"
		"/var/log",
		"/var/lib/mysql",
		"/var/log/journal",
		"/opt",
		"/root",
	}
	out := make([]string, 0, len(subs))
	for _, s := range subs {
		out = append(out, path.Join(mount, s))
	}
	return out
}

// buildDiskVolumeScanCommand 第 2 段：体积段
func buildDiskVolumeScanCommand(mount string) string {
	q := shellQuote(mount)
	bigPaths := shellJoin(diskBigFileScanPaths(mount))
	lines := []string{
		"echo '" + secVolProbe + "'",
		"du -d1 -k /tmp >/dev/null 2>&1 && echo '" + probeDuNew + "' || echo '" + probeDuOld + "'",
		"find " + q + " -xdev -maxdepth 0 -printf '" + probeFindNew + "' 2>/dev/null || echo '" + probeFindOld + "'",
		"echo '" + secVolDirs + "'",
		// 2>&1 把 du 的 "Permission denied" 一并带回来：解析时按"首字段必须是整数"自动跳过，
		// 同时让我们能识别出"有目录因权限不足没算进去"
		"du -x -d1 -k " + q + " 2>&1 | sort -rn | head -30",
		"echo '" + secVolBig + "'",
		"find " + bigPaths + " -xdev -type f -size +" + strconv.Itoa(diskBigFileBytes) + " -printf '%s %p\\n' 2>&1 | sort -rn | head -30",
		"echo '" + secVolAll + "'",
		"find " + q + " -xdev -type f -size +" + strconv.Itoa(diskAllFileBytes) + " -printf '%s %p\\n' 2>&1 | sort -rn | head -20",
		"echo '" + secVolEnd + "'",
	}
	return strings.Join(lines, "\n")
}

// buildDiskDrillCommand 下钻一层
func buildDiskDrillCommand(dir string) string {
	return "du -x -d1 -k " + shellQuote(dir) + " 2>&1 | sort -rn | head -30"
}

// buildDuMaxDepthFallback 老版本 du（不支持 -d1）的等价命令
func buildDuMaxDepthFallback(dir string) string {
	return "du -x -k --max-depth=1 " + shellQuote(dir) + " 2>&1 | sort -rn | head -30"
}

// buildBigFileLsFallback find 不支持 -printf 时的回退（用 ls 取体积）
func buildBigFileLsFallback(mount string) string {
	bigPaths := shellJoin(diskBigFileScanPaths(mount))
	return "find " + bigPaths + " -xdev -type f -size +" + strconv.Itoa(diskAllFileBytes/1024) +
		"k -exec ls -l {} \\; 2>/dev/null | awk '{print $5, $9}' | sort -rn | head -30"
}

// buildDeletedFallback lsof 不可用时，用 /proc 找出已删除但被持有的文件
func buildDeletedFallback() string {
	return "find /proc/[0-9]*/fd -lname '*deleted*' 2>/dev/null | head -40"
}

// ---- 分节解析 ----

// splitMarkedSections 按标记把输出切成若干节。标记必须独占一行（脚本用 echo 输出）。
// 未出现的标记对应空串；标记之后的全部内容都归属该节。
func splitMarkedSections(out string, markers []string) map[string]string {
	known := make(map[string]bool, len(markers))
	for _, m := range markers {
		known[m] = true
	}
	res := make(map[string]string, len(markers))
	var buf []string
	cur := ""
	flush := func() {
		if cur == "" {
			return
		}
		res[cur] = strings.Join(buf, "\n")
		buf = buf[:0]
	}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if t := strings.TrimSpace(line); known[t] {
			flush()
			cur = t
			continue
		}
		if cur != "" {
			buf = append(buf, line)
		}
	}
	flush()
	return res
}

// parseDfInodes 解析 df -Pi 的 inode 信息。
//
// 注意：df -Pi 的输出是 6 列（Filesystem Inodes IUsed IFree IUse% Mounted on），
// 会被 parseDf 的六列分支误当作 1K 块容量解析，因此 inode 绝不能复用 parseDf。
func parseDfInodes(out string) (total, used int64, usage float64, ok bool) {
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		var totalS, usedS, pctS string
		switch {
		case len(f) >= 6:
			// Filesystem Inodes IUsed IFree IUse% Mounted on
			totalS, usedS, pctS = f[1], f[2], f[4]
		case len(f) == 5 && strings.HasSuffix(f[3], "%"):
			// 设备名过长时 GNU df 会把设备单独占一行，数据行只剩 5 列：
			// Inodes IUsed IFree IUse% Mounted on
			totalS, usedS, pctS = f[0], f[1], f[3]
		default:
			continue
		}
		t, e1 := strconv.ParseInt(totalS, 10, 64)
		u, e2 := strconv.ParseInt(usedS, 10, 64)
		pct, e3 := strconv.ParseFloat(strings.TrimSuffix(pctS, "%"), 64)
		if e1 != nil || e2 != nil || e3 != nil || t <= 0 {
			// 表头（Inodes / IUse%）或 "-"（该文件系统不统计 inode）
			continue
		}
		return t, u, round2(pct), true
	}
	return 0, 0, 0, false
}

// parseDuDepth1 解析 du -x -d1 -k 的输出（首字段为 KB，其余为路径）。
// 首字段不是整数的行（du 的 Permission denied 报错行）自动跳过。
func parseDuDepth1(out, base string, usedMB int64) ([]models.DiskDirItem, int64) {
	items, total := parseSizePathLines(out, base, 1024, usedMB)
	return items, total
}

// parseFindSizeLines 解析 find -printf '%s %p\n' 的输出（首字段为字节）
func parseFindSizeLines(out string, usedMB int64) ([]models.DiskDirItem, int64) {
	return parseSizePathLines(out, "", 1024*1024, usedMB)
}

// parseSizePathLines 解析「体积 + 路径」的通用行格式。
// divisor 为"首字段 → MB"的除数；base 非空时跳过基准目录自身的合计行。
func parseSizePathLines(out, base string, divisor int64, usedMB int64) ([]models.DiskDirItem, int64) {
	var items []models.DiskDirItem
	var total int64
	base = normalizeRemote(base)
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		idx := strings.IndexAny(line, " \t")
		if idx < 0 {
			continue
		}
		// 不 TrimLeft 整个行：du 的输出可能是 "SIZE\tPATH"，也可能有前导空格
		size, err := strconv.ParseInt(strings.TrimSpace(line[:idx]), 10, 64)
		if err != nil || size <= 0 {
			continue
		}
		p := normalizeRemote(strings.TrimSpace(line[idx+1:]))
		if p == "" || p == base {
			continue
		}
		// 向上取整，避免小目录/小文件被显示成 0MB
		mb := (size + divisor - 1) / divisor
		items = append(items, models.DiskDirItem{Path: p, SizeMB: mb, HasChildren: true})
		total += mb
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].SizeMB > items[j].SizeMB })
	if usedMB > 0 {
		for i := range items {
			items[i].Percent = round2(float64(items[i].SizeMB) * 100 / float64(usedMB))
		}
	}
	return items, total
}

// normalizeRemote 规范化远程路径用于比较（去尾部斜杠，根目录保留为 /）
func normalizeRemote(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if p != "/" {
		p = strings.TrimRight(p, "/")
	}
	if p == "" {
		p = "/"
	}
	return p
}

// parseHumanSize 解析 1.2G / 200M / 512B / 1.5GiB / 12345 这类体积写法为字节数。
// 带 % 或括号（docker 的 "2.1GB (50%)" 拆出的 "(50%)"）一律拒绝。
func parseHumanSize(s string) (int64, bool) {
	t := strings.TrimRight(strings.TrimSpace(s), ".,;:") // 句末标点（如 "Journal is using 512M."）
	if t == "" || strings.ContainsAny(t, "%()") {
		return 0, false
	}
	u := strings.ToUpper(t)
	if strings.HasSuffix(u, "B") {
		u = u[:len(u)-1]
	}
	if strings.HasSuffix(u, "I") { // Gi -> G
		u = u[:len(u)-1]
	}
	mult := int64(1)
	if u != "" {
		switch u[len(u)-1] {
		case 'K':
			mult = 1 << 10
		case 'M':
			mult = 1 << 20
		case 'G':
			mult = 1 << 30
		case 'T':
			mult = 1 << 40
		case 'P':
			mult = 1 << 50
		}
		if mult > 1 {
			u = u[:len(u)-1]
		}
	}
	if u == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(u, 64)
	if err != nil || f < 0 {
		return 0, false
	}
	return int64(f * float64(mult)), true
}

// hasSizeUnit 判断字段是否带单位后缀（用于在 docker 输出里区分体积列与 TOTAL/ACTIVE 计数列）
func hasSizeUnit(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			_, ok := parseHumanSize(s)
			return ok
		}
	}
	return false
}

// parseJournalUsage 解析 journalctl --disk-usage
// 新版："Archived and active journals take up 1.2G in the file system."
// 旧版："Journal is using 1.2G."
func parseJournalUsage(out string) (int64, bool) {
	for _, key := range []string{"take up", "is using", "using"} {
		i := strings.Index(out, key)
		if i < 0 {
			continue
		}
		f := strings.Fields(strings.TrimSpace(out[i+len(key):]))
		if len(f) == 0 {
			continue
		}
		if b, ok := parseHumanSize(f[0]); ok {
			return b / (1024 * 1024), true
		}
	}
	return 0, false
}

// parseDockerDf 解析 docker system df。
// 行内字段为：TYPE TOTAL ACTIVE SIZE RECLAIMABLE，其中 RECLAIMABLE 形如 "2.1GB (50%)"。
// 因为 TOTAL/ACTIVE 是纯数字、也能被 parseHumanSize 解析成字节，所以只取「带单位后缀」
// 的字段，并且取最后两个（SIZE、RECLAIMABLE）。
func parseDockerDf(out string) (DockerUsage, bool) {
	var du DockerUsage
	found := false
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		var kind string
		switch {
		case strings.HasPrefix(line, "Images"):
			kind = "images"
		case strings.HasPrefix(line, "Containers"):
			kind = "containers"
		case strings.HasPrefix(line, "Local Volumes"):
			kind = "volumes"
		case strings.HasPrefix(line, "Build Cache"):
			kind = "cache"
		default:
			continue
		}
		var sizes []int64
		for _, f := range strings.Fields(line) {
			if !hasSizeUnit(f) {
				continue
			}
			if v, ok := parseHumanSize(f); ok {
				sizes = append(sizes, v/(1024*1024))
			}
		}
		if len(sizes) == 0 {
			continue
		}
		size := sizes[len(sizes)-1]
		var reclaim int64
		if len(sizes) >= 2 {
			size = sizes[len(sizes)-2]
			reclaim = sizes[len(sizes)-1]
		}
		du.ReclaimableMB += reclaim
		switch kind {
		case "images":
			du.ImagesMB = size
		case "containers":
			du.ContainersMB = size
		case "volumes":
			du.VolumesMB = size
		case "cache":
			du.BuildCacheMB = size
		}
		found = true
	}
	return du, found
}

// parseLsofDeleted 解析 lsof +L1 的输出。
// 列：COMMAND PID USER FD TYPE DEVICE SIZE/OFF NLINK NODE NAME
func parseLsofDeleted(out string) []DeletedFile {
	var items []DeletedFile
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 10 || f[0] == "COMMAND" {
			continue
		}
		if !strings.Contains(strings.ToLower(line), "deleted") {
			continue
		}
		mb := int64(0)
		if v, err := strconv.ParseInt(f[6], 10, 64); err == nil {
			mb = v / (1024 * 1024)
		}
		items = append(items, DeletedFile{
			Command: f[0],
			PID:     f[1],
			Path:    strings.Join(f[9:], " "),
			SizeMB:  mb,
		})
	}
	return items
}

// parseDeletedFallbackPaths 解析 /proc/*/fd 回退方案的输出（只有路径，拿不到体积）
func parseDeletedFallbackPaths(out string) []DeletedFile {
	var items []DeletedFile
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" || !strings.HasPrefix(line, "/proc/") {
			continue
		}
		items = append(items, DeletedFile{Path: line, SizeUnknown: true})
	}
	return items
}

// ---- 结果结构 ----

// DockerUsage docker 磁盘占用（单位 MB）
type DockerUsage struct {
	ImagesMB      int64
	ContainersMB  int64
	VolumesMB     int64
	BuildCacheMB  int64
	ReclaimableMB int64
}

// DeletedFile 已删除但仍被进程持有的文件
type DeletedFile struct {
	Command     string
	PID         string
	Path        string
	SizeMB      int64
	SizeUnknown bool
}

// DiskQuickscan 第 1 段「瞬时检查」的解析结果
type DiskQuickscan struct {
	Disk      models.DiskInfo
	DiskFound bool

	InodeTotal int64
	InodeUsed  int64
	InodeUsage float64
	InodeOK    bool

	JournalMB int64
	JournalOK bool

	Docker   DockerUsage
	DockerOK bool

	Deleted     []DeletedFile
	LsofMissing bool // lsof 输出里没有表头 → 该机器大概率没装 lsof

	PermissionDenied bool
	Raw              map[string]string
}

// VolumeScan 第 2 段「体积段」的解析结果
type VolumeScan struct {
	TopDirs  []models.DiskDirItem
	BigFiles []models.DiskDirItem

	ScannedMB int64
	GapMB     int64

	PermissionDenied bool
	Partial          bool
	Notes            []string
	Raw              map[string]string
}

// ---- 客户端方法 ----

// DiskQuickscan 执行瞬时检查：目标盘容量 / inode / 挂载 / journald / docker / 已删除句柄
func (c *Client) DiskQuickscan(mount string, useSudo bool) (*DiskQuickscan, error) {
	if mount == "" {
		mount = "/"
	}
	_, out, _ := c.ExecCommand(buildDiskQuickscanCommand(mount), diskQuickscanTimeout, useSudo)
	secs := splitMarkedSections(out, []string{
		secQuickDF, secQuickInode, secQuickMounts, secQuickJournal, secQuickDocker, secQuickDeleted, secQuickEnd,
	})
	q := &DiskQuickscan{Raw: secs}

	// 容量：复用现成的 parseDf（-P 格式），并从中挑出与 mount 匹配的那条
	disks := parseDf(secs[secQuickDF])
	want := normalizeRemote(mount)
	for _, d := range disks {
		if normalizeRemote(d.Mount) == want {
			q.Disk = d
			q.DiskFound = true
			break
		}
	}
	if !q.DiskFound && len(disks) > 0 {
		q.Disk = disks[0]
		q.DiskFound = true
	}

	if t, u, pct, ok := parseDfInodes(secs[secQuickInode]); ok {
		q.InodeTotal, q.InodeUsed, q.InodeUsage, q.InodeOK = t, u, pct, true
	}

	// df 输出里没有挂载选项，只读标记要靠 /proc/mounts（否则"只读挂载"这条结论永远不触发）
	if ro := parseReadOnlyMounts(secs[secQuickMounts]); len(ro) > 0 && q.DiskFound && ro[q.Disk.Mount] {
		q.Disk.IsReadOnly = true
	}

	if mb, ok := parseJournalUsage(secs[secQuickJournal]); ok {
		q.JournalMB, q.JournalOK = mb, true
	}
	if du, ok := parseDockerDf(secs[secQuickDocker]); ok {
		q.Docker, q.DockerOK = du, true
	}

	delSec := secs[secQuickDeleted]
	if !strings.Contains(delSec, "COMMAND") {
		// 没有 lsof 表头：可能没装 lsof，也可能输出被截断。
		// 再用 /proc 兜底找一遍（拿不到体积，但能确认"有东西没释放"）
		q.LsofMissing = true
		_, fbOut, _ := c.ExecCommand(buildDeletedFallback(), diskQuickscanTimeout, useSudo)
		q.Deleted = parseDeletedFallbackPaths(fbOut)
	} else {
		q.Deleted = parseLsofDeleted(delSec)
	}

	q.PermissionDenied = strings.Contains(out, "Permission denied")
	return q, nil
}

// DiskVolumeScan 执行体积段：一级目录占用 + 专项大文件。
// du 超时或不可用时返回 Partial=true 的结果（不是错误），由上层决定如何呈现。
func (c *Client) DiskVolumeScan(mount string, useSudo bool) (*VolumeScan, error) {
	if mount == "" {
		mount = "/"
	}
	_, out, _ := c.ExecCommand(buildDiskVolumeScanCommand(mount), diskVolumeTimeout, useSudo)
	secs := splitMarkedSections(out, []string{secVolProbe, secVolDirs, secVolBig, secVolAll, secVolEnd})
	v := &VolumeScan{Raw: secs}

	probe := secs[secVolProbe]
	usedMB := int64(0)

	// 一级目录占用：主命令无结果时才换 --max-depth 重试一次
	// （不预先按探测结果决定：du -d1 不受支持与 du 跑太大被超时杀掉，表现都是"没结果"）
	dirOut := secs[secVolDirs]
	v.TopDirs, v.ScannedMB = parseDuDepth1(dirOut, mount, usedMB)
	if len(v.TopDirs) == 0 {
		if _, retry, _ := c.ExecCommand(buildDuMaxDepthFallback(mount), diskVolumeTimeout, useSudo); strings.TrimSpace(retry) != "" {
			if items, total := parseDuDepth1(retry, mount, usedMB); len(items) > 0 {
				v.TopDirs, v.ScannedMB = items, total
				dirOut = retry
				v.Notes = append(v.Notes, "首次目录扫描没有返回结果，已用 du --max-depth=1 重试成功"+
					"（老版本 du 不支持 -d1，或首次扫描被超时中断）")
			}
		}
	}

	bigOut := secs[secVolBig] + "\n" + secs[secVolAll]
	if strings.Contains(probe, probeFindOld) {
		v.Notes = append(v.Notes, "该机器的 find 不支持 -printf，大文件体积改用 ls 兼容方式获取")
		if _, retry, _ := c.ExecCommand(buildBigFileLsFallback(mount), diskVolumeTimeout, useSudo); strings.TrimSpace(retry) != "" {
			bigOut = bigOut + "\n" + retry
		}
	}
	v.BigFiles, _ = parseFindSizeLines(bigOut, usedMB)
	v.BigFiles = dedupeDirItems(v.BigFiles)

	if strings.Contains(dirOut+bigOut, "Permission denied") {
		v.PermissionDenied = true
	}
	if len(v.TopDirs) == 0 {
		v.Partial = true
		v.Notes = append(v.Notes, "目录扫描没有返回结果：可能是 du 超时、该盘挂载异常，或目录全部无读权限")
	}
	return v, nil
}

// DiskDrill 对某个目录下钻一层。
// du 被超时中断时（ExecCommand 返回 -1 且已拿到部分输出）标记 Partial，
// 让上层能如实告诉用户"这只是统计到的部分"。
func (c *Client) DiskDrill(dir string, useSudo bool) (models.DrillResult, error) {
	if dir == "" {
		dir = "/"
	}
	res := models.DrillResult{Items: []models.DiskDirItem{}}
	code, out, _ := c.ExecCommand(buildDiskDrillCommand(dir), diskDrillTimeout, useSudo)
	items, _ := parseDuDepth1(out, dir, 0)
	if len(items) == 0 {
		// 老 du 或输出为空时再试一次 --max-depth
		code2, retry, _ := c.ExecCommand(buildDuMaxDepthFallback(dir), diskDrillTimeout, useSudo)
		if strings.TrimSpace(retry) != "" {
			if it, _ := parseDuDepth1(retry, dir, 0); len(it) > 0 {
				items, code = it, code2
			}
		}
	}
	if items == nil {
		items = []models.DiskDirItem{}
	}
	res.Items = items
	if code == -1 && len(items) > 0 {
		res.Partial = true
		res.Note = "目录扫描在 " + strconv.Itoa(diskDrillTimeout) +
			" 秒上限处被中断，下面只列出了已统计到的部分子目录（体积数字也会偏小）"
	}
	return res, nil
}

// dedupeDirItems 同一路径去重（专项扫描与全盘扫描会重叠）
func dedupeDirItems(items []models.DiskDirItem) []models.DiskDirItem {
	seen := make(map[string]bool, len(items))
	out := make([]models.DiskDirItem, 0, len(items))
	for _, it := range items {
		if seen[it.Path] {
			continue
		}
		seen[it.Path] = true
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SizeMB > out[j].SizeMB })
	return out
}

// ---- 结论推断 ----

// BuildDiskReport 把两段采集结果组装成完整报告（纯函数，便于单测）。
// volume 为 nil 时表示体积段尚未完成，报告 Stage=quickscan、Partial=true。
func BuildDiskReport(taskID, serverID, serverName, mount string, quick *DiskQuickscan, volume *VolumeScan, elapsedMs int64) models.DiskReport {
	if mount == "" {
		mount = "/"
	}
	rep := models.DiskReport{
		TaskID:     taskID,
		ServerID:   serverID,
		ServerName: serverName,
		Mount:      mount,
		Stage:      "quickscan",
		Partial:    true,
		ElapsedMs:  elapsedMs,
		ScannedAt:  time.Now().Format("2006-01-02 15:04:05"),
	}
	if quick != nil && quick.DiskFound {
		rep.Device = quick.Disk.Device
		rep.Fstype = quick.Disk.Fstype
		rep.TotalMB = quick.Disk.TotalMB
		rep.UsedMB = quick.Disk.UsedMB
		rep.AvailMB = quick.Disk.AvailMB
		rep.Usage = quick.Disk.Usage
	}
	if quick != nil {
		rep.InodeTotal = quick.InodeTotal
		rep.InodeUsed = quick.InodeUsed
		rep.InodeUsage = quick.InodeUsage
	}

	var findings []models.DiskFinding
	add := func(f models.DiskFinding) { findings = append(findings, f) }

	// 1) 容量本身
	if quick != nil && quick.DiskFound && quick.Disk.Usage >= 90 {
		add(models.DiskFinding{
			Level:  "critical",
			Kind:   "usage",
			Title:  mount + " 已用 " + fmtPct(quick.Disk.Usage),
			Detail: "可用空间仅剩 " + fmtMB(quick.Disk.AvailMB) + "，继续写入会失败（日志、数据库、容器都可能报错）。",
			SizeMB: quick.Disk.AvailMB,
		})
	}
	if quick != nil && quick.DiskFound && quick.Disk.Usage >= 99 && quick.Disk.AvailMB > 0 {
		add(models.DiskFinding{
			Level: "info",
			Kind:  "reserved",
			Title: "看起来写满了，但还显示 " + fmtMB(quick.Disk.AvailMB) + " 可用",
			Detail: "ext4 等文件系统默认保留 5% 给 root，普通用户会看到「空间不足」而 root 还能写。" +
				"这是预期行为，不需要处理；如需调整可用 tune2fs -m 1 <设备>（有风险）。",
		})
	}

	// 2) inode 耗尽（du 找不到大头时最常见的原因之一）
	if quick != nil && quick.InodeOK && quick.InodeUsage >= 85 {
		lvl := "warning"
		if quick.InodeUsage >= 95 {
			lvl = "critical"
		}
		detail := "inode 使用率 " + fmtPct(quick.InodeUsage) + "（" + strconv.FormatInt(quick.InodeUsed, 10) +
			" / " + strconv.FormatInt(quick.InodeTotal, 10) + "）。大量小文件（session、缓存、邮件队列、日志碎片）会先把 inode 用光，" +
			"此时即使还有剩余空间也无法创建新文件——du 排行会看起来「没有大目录」。"
		if quick.DiskFound && quick.InodeUsage > quick.Disk.Usage {
			detail += "当前 inode 使用率高于空间使用率（" + fmtPct(quick.Disk.Usage) + "），说明瓶颈就在 inode。"
		}
		add(models.DiskFinding{
			Level:   lvl,
			Kind:    "inode",
			Title:   "inode 使用率 " + fmtPct(quick.InodeUsage),
			Detail:  detail,
			Command: "df -Pi " + mount + "; find " + mount + " -xdev -type d -size +100k 2>/dev/null | head -20",
			CommandNote: "作用：再按 inode 维度看一遍使用率，并列出「自身目录项超过 10 万条」的目录——海量小文件通常就藏在这几个目录里。" +
				"运行后：只读，不修改任何文件；输出是 inode 百分比和几条超大目录路径，对它们继续 du 就能定位到具体文件。",
		})
	} else if quick != nil && !quick.InodeOK {
		add(models.DiskFinding{
			Level:  "unavailable",
			Kind:   "inode",
			Title:  "本机未提供 inode 统计",
			Detail: "该文件系统不统计 inode（或 df -i 输出异常），已跳过 inode 检查。",
		})
	}

	// 3) 已删除但仍被持有的文件（df 满、du 干净的头号嫌疑）
	if quick != nil && len(quick.Deleted) > 0 {
		var totalMB int64
		var lines []string
		unknown := false
		for _, d := range quick.Deleted {
			totalMB += d.SizeMB
			if d.SizeUnknown {
				unknown = true
			}
			lines = append(lines, strings.TrimSpace(d.Command+" "+d.PID+" "+fmtMB(d.SizeMB)+" "+d.Path))
		}
		detail := "这些文件已经被删除（或日志被轮转删除），但进程还持有打开的文件句柄，磁盘空间不会释放，" +
			"所以 df 显示满了而 du 找不到对应目录。常见于被 rm 掉的日志、被覆盖的旧版本文件。"
		if unknown {
			detail += "（本机没有 lsof，无法读出每个文件的体积，可安装 lsof 或勾选「用 root 重跑」后再看。）"
			totalMB = 0
		}
		add(models.DiskFinding{
			Level:    "critical",
			Kind:     "deleted-open",
			Title:    strconv.Itoa(len(quick.Deleted)) + " 个已删除文件仍被进程持有",
			Detail:   detail,
			Evidence: strings.Join(lines, "\n"),
			Command:  "lsof +L1",
			CommandNote: "作用：列出「链接数为 0（文件已删除）但仍被进程打开」的文件，以及持有它的进程名、PID 和大小。" +
				"运行后：只读。确认后按进程名重启对应服务（或对该文件执行 truncate -s 0）即可把这部分空间释放出来。" +
				"注意：直接把这个文件再 rm 一次没有用，空间依然不会释放。",
			SizeMB: totalMB,
		})
	}

	// 4) journald
	if quick != nil && quick.JournalOK && quick.JournalMB >= 1024 {
		add(models.DiskFinding{
			Level:   "warning",
			Kind:    "journal",
			Title:   "systemd 日志占用 " + fmtMB(quick.JournalMB),
			Detail:  "journald 默认不限制总量，长期运行会持续增长。建议设置保留上限或按时间清理。",
			Command: "journalctl --disk-usage; journalctl --vacuum-size=200M",
			CommandNote: "作用：第一条看 systemd 日志当前占多少；第二条把归档日志收缩到 200MB 以内（保留最近的部分）。" +
				"运行后：会删除较早的归档日志条目，空间立即释放；正在运行的服务与当前日志不受影响，不需要重启任何服务。",
			SizeMB: quick.JournalMB,
		})
	} else if quick != nil && !quick.JournalOK {
		add(models.DiskFinding{
			Level:  "unavailable",
			Kind:   "journal",
			Title:  "未能读取 systemd 日志占用",
			Detail: "journalctl 不存在或需要 root（可勾选「用 root 重跑」）。",
		})
	}

	// 5) docker
	if quick != nil && quick.DockerOK {
		if quick.Docker.ReclaimableMB >= 1024 {
			add(models.DiskFinding{
				Level: "warning",
				Kind:  "docker",
				Title: "docker 可回收约 " + fmtMB(quick.Docker.ReclaimableMB),
				Detail: "镜像 " + fmtMB(quick.Docker.ImagesMB) + "／容器 " + fmtMB(quick.Docker.ContainersMB) +
					"／数据卷 " + fmtMB(quick.Docker.VolumesMB) + "／构建缓存 " + fmtMB(quick.Docker.BuildCacheMB) +
					"。prune 会删除未使用的镜像、网络、构建缓存（加 --volumes 还会删未使用的数据卷，请确认后再执行）。",
				Command: "docker system df; docker system prune",
				CommandNote: "作用：第一条列出镜像/容器/数据卷/构建缓存各自的占用与可回收量；第二条删除未使用的镜像、" +
					"已停止的容器、未使用的网络与构建缓存。运行后：命令会先列出将要删除的内容并等你输入 y 确认；" +
					"正在运行的容器和使用中的数据卷不受影响（要连未使用数据卷一起删需自行加 --volumes）。",
				SizeMB: quick.Docker.ReclaimableMB,
			})
		}
	} else if quick != nil {
		add(models.DiskFinding{
			Level:  "unavailable",
			Kind:   "docker",
			Title:  "本机没有 docker（或 docker 未运行）",
			Detail: "已跳过容器占用检查。",
		})
	}

	// 6) 体积段
	if volume != nil {
		rep.Stage = "done"
		rep.Partial = volume.Partial
		rep.TopDirs = volume.TopDirs
		rep.BigFiles = volume.BigFiles
		rep.ScannedMB = volume.ScannedMB
		rep.Notes = append(rep.Notes, volume.Notes...)
		// 占比要按"该盘已用容量"算，而 du 解析时还拿不到 df 的数字，这里补上
		if rep.UsedMB > 0 {
			for i := range rep.TopDirs {
				rep.TopDirs[i].Percent = round2(float64(rep.TopDirs[i].SizeMB) * 100 / float64(rep.UsedMB))
			}
		}

		if rep.UsedMB > 0 {
			rep.GapMB = rep.UsedMB - rep.ScannedMB
			if rep.GapMB < 0 {
				rep.GapMB = 0
			}
			if rep.UsedMB > 0 && rep.GapMB*100/rep.UsedMB > 30 {
				add(models.DiskFinding{
					Level: "warning",
					Kind:  "gap",
					Title: "有 " + fmtMB(rep.GapMB) + " 的空间没被目录扫描解释掉",
					Detail: "该盘已用 " + fmtMB(rep.UsedMB) + "，但一级目录相加只有 " + fmtMB(rep.ScannedMB) +
						"（相差 " + fmtPct(float64(rep.GapMB)*100/float64(rep.UsedMB)) + "）。常见原因：" +
						"① 已删除但仍被进程持有的文件（见上）；② 无权限目录未被 du 统计（勾选「用 root 重跑」）；" +
						"③ 挂载点被上层目录的数据覆盖；④ du 与 df 的统计口径差异（稀疏文件、预留块）。",
					Command: "du -xsh " + mount + "/* 2>/dev/null | sort -h; df -h " + mount,
					CommandNote: "作用：用人类可读单位（GB/MB）列出该盘每个一级目录的体积，再显示一次文件系统层面的已用/可用。" +
						"运行后：只读。把 du 的合计与 df 的已用对一下——如果还是差很多，说明差额来自「已删除但仍被持有」" +
						"或「没权限看到的目录」，按报告里对应那一条继续处理。",
					SizeMB: rep.GapMB,
				})
			}
		}

		// 大文件逐条给出（日志类建议 truncate，其他只提示确认）
		for _, f := range rep.BigFiles {
			if f.SizeMB < 1024 {
				continue
			}
			lvl := "warning"
			if f.SizeMB >= 5120 {
				lvl = "critical"
			}
			detail := "完整路径：" + f.Path + "。该文件占用 " + fmtMB(f.SizeMB) + "。"
			cmd, cmdNote := "", ""
			isLog := isLogLikePath(f.Path)
			if isLog {
				detail += "这是日志类文件，清空内容即可释放空间，且**不要直接用 rm**：" +
					"删掉文件后写入该文件的进程仍会持有 inode，空间不会立刻释放（这正是「已删除但仍被持有」的成因）。"
				cmd = "truncate -s 0 " + shellQuote(f.Path)
				cmdNote = "作用：把这个日志文件的内容清空，但保留文件本身与 inode。运行后：文件大小立刻变成 0、" +
					"这块空间马上归还给系统；正在写它的服务不会报错、不需要重启（这正是它比 rm 安全的原因）。"
			} else {
				detail += "请确认它是否还被使用（模型文件、备份、导出文件等），确认后再处理。"
				cmd = "ls -lh " + shellQuote(f.Path)
				cmdNote = "作用：只看这个文件的大小和最后修改时间，不读内容也不改动任何东西。" +
					"运行后：确认它是否还在被使用、是否属于可以清理的旧数据，再决定怎么处理。"
			}
			isContainerLog := strings.Contains(f.Path, "/docker/containers/") && strings.HasSuffix(f.Path, "-json.log")
			if isContainerLog {
				detail += " 这是容器标准输出日志：根治办法是给容器配置日志轮转" +
					"（--log-opt max-size=100m --log-opt max-file=3），否则清空后还会再长回来。"
				cmdNote += " 但注意：这只是清空当前这一份，容器还在跑就还会继续写——根治要重建容器并加上" +
					" --log-opt max-size=100m --log-opt max-file=3（docker run 或 compose 的 logging 配置）。"
			}
			add(models.DiskFinding{
				Level: lvl,
				Kind:  "bigfile",
				// 标题只放体积 + 文件名：完整路径在 Detail 里。
				// 标题带整条路径时，超长路径会把结论卡片顶出可视区（前端已无法再截断）。
				Title:       fmtMB(f.SizeMB) + "  " + path.Base(f.Path),
				Detail:      detail,
				Command:     cmd,
				CommandNote: cmdNote,
				SizeMB:      f.SizeMB,
			})
		}

		if volume.PermissionDenied {
			add(models.DiskFinding{
				Level: "info",
				Kind:  "permission",
				Title: "有目录因权限不足未计入",
				Detail: "普通用户读不到的目录（如 /root、/var/lib/docker 的部分子目录）不会被 du 统计，" +
					"因此上面的目录排行会偏小。勾选「用 root 重跑」可以得到完整结果。",
				Command: "sudo du -xsh " + mount + "/* 2>/dev/null | sort -h",
				CommandNote: "作用：以 root 身份重新统计每个一级目录的体积（能读到普通用户读不到的目录）。" +
					"运行后：会提示输入密码才能继续；结果里的数字通常比上面更大，差额随之缩小。" +
					"本页的「以 root 权限扫描」就是自动做这件事，不用手敲。",
			})
		}
	}

	// 7) 目标盘本身的性质
	if quick != nil && quick.DiskFound {
		if quick.Disk.IsReadOnly {
			add(models.DiskFinding{
				Level:  "info",
				Kind:   "pseudo",
				Title:  mount + " 以只读方式挂载",
				Detail: "只读挂载不会因写入而增长，通常不需要关注。",
			})
		} else if quick.Disk.IsPseudo {
			add(models.DiskFinding{
				Level:  "info",
				Kind:   "pseudo",
				Title:  mount + " 是伪文件系统（" + quick.Disk.Fstype + "）",
				Detail: "tmpfs / overlay 等伪文件系统占用的是内存或镜像层，不参与「快满」预警。",
			})
		}
	}

	sortFindings(findings)
	// 保证切片非 nil：Go 的 nil 切片会序列化成 JSON null，前端 v-for 会直接报错
	if findings == nil {
		findings = []models.DiskFinding{}
	}
	if rep.TopDirs == nil {
		rep.TopDirs = []models.DiskDirItem{}
	}
	if rep.BigFiles == nil {
		rep.BigFiles = []models.DiskDirItem{}
	}
	if rep.Notes == nil {
		rep.Notes = []string{}
	}
	rep.Findings = findings
	// 可回收体积只统计"确实占了空间、且报告建议处理"的项。
	// gap（只是解释差额）与 usage（那是可用余量）都不计入，避免虚高。
	for _, f := range findings {
		switch f.Kind {
		case "deleted-open", "journal", "docker", "bigfile":
			if f.Level == "critical" || f.Level == "warning" {
				rep.ReclaimableMB += f.SizeMB
			}
		}
	}
	return rep
}

// isLogLikePath 判断是否日志类文件（给出 truncate 建议而不是 rm）
func isLogLikePath(p string) bool {
	lower := strings.ToLower(p)
	if strings.Contains(lower, "/log/") || strings.Contains(lower, "/logs/") {
		return true
	}
	for _, suf := range []string{".log", ".out", "-json.log", ".err", ".access", ".error"} {
		if strings.HasSuffix(lower, suf) {
			return true
		}
	}
	return false
}

// sortFindings 按严重程度排序：critical → warning → info → unavailable，同级按体积降序
func sortFindings(fs []models.DiskFinding) {
	rank := func(level string) int {
		switch level {
		case "critical":
			return 0
		case "warning":
			return 1
		case "info":
			return 2
		default:
			return 3
		}
	}
	sort.SliceStable(fs, func(i, j int) bool {
		ri, rj := rank(fs[i].Level), rank(fs[j].Level)
		if ri != rj {
			return ri < rj
		}
		return fs[i].SizeMB > fs[j].SizeMB
	})
}

func fmtMB(mb int64) string {
	switch {
	case mb >= 1024*1024:
		return strconv.FormatFloat(float64(mb)/(1024*1024), 'f', 2, 64) + " TB"
	case mb >= 1024:
		return strconv.FormatFloat(float64(mb)/1024, 'f', 2, 64) + " GB"
	default:
		return strconv.FormatInt(mb, 10) + " MB"
	}
}

func fmtPct(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s + "%"
}
