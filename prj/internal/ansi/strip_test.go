package ansi

import (
	"strings"
	"testing"
)

func TestStripRemovesColorsAndTitles(t *testing.T) {
	raw := "\x1b]0;root@host: ~\x07\x1b[01;32muser@host\x1b[0m:\x1b[01;34m~\x1b[0m$ ls\r\n" +
		"\x1b[31merror\x1b[0m: not found\r\n"
	got := Strip(raw)
	want := "user@host:~$ ls\nerror: not found\n"
	if got != want {
		t.Fatalf("Strip = %q, want %q", got, want)
	}
}

// 进度条/spinner 会用 \r 反复覆盖同一行：只保留最终状态，体积能降一个数量级
func TestStripCollapsesCarriageReturns(t *testing.T) {
	raw := "Installing: 10%\rInstalling: 45%\rInstalling: 80%\rInstalling: 100% done\nnext line\n"
	got := Strip(raw)
	if got != "Installing: 100% done\nnext line\n" {
		t.Fatalf("\\r 折叠错误: %q", got)
	}
	// 典型的 spinner 噪音：上千个 \r 只留最后一行
	spin := strings.Repeat("\r|/-\\", 300) + "\rdone\n"
	if got := Strip(spin); got != "done\n" {
		t.Fatalf("spinner 折叠错误: %q", got)
	}
}

func TestStripHandlesBackspaceAndControls(t *testing.T) {
	// abc 退两格后写 d，BEL 与 NUL 直接丢弃
	got := Strip("abc\b\bd\x07\x00y\n")
	if got != "ady\n" {
		t.Fatalf("退格/控制字符处理错误: %q", got)
	}
}

// 分块清洗再拼接时不能把两段粘成一行（末尾换行必须原样保留）
func TestStripKeepsTrailingNewline(t *testing.T) {
	if got := Strip("abc\n"); got != "abc\n" {
		t.Fatalf("末尾换行被吞掉: %q", got)
	}
	if got := Strip("abc"); got != "abc" {
		t.Fatalf("没有换行时不应补: %q", got)
	}
	if got := Strip(""); got != "" {
		t.Fatalf("空输入应返回空: %q", got)
	}
	a := Strip("line1\n")
	b := Strip("line2\n")
	if a+b != "line1\nline2\n" {
		t.Fatalf("分块拼接错误: %q", a+b)
	}
}

// PTY 输出基本都是 CRLF：\r\n 必须当成普通换行，不能被"进度条折叠"吃掉
func TestStripKeepsCRLF(t *testing.T) {
	want := "line1\nline2\n"
	if got := Strip("line1\r\nline2\r\n"); got != want {
		t.Fatalf("CRLF 被破坏: %q", got)
	}
	// 进度条里混用 CRLF 与单独 \r 的两行，仍然只保留最终状态
	got := Strip("downloading 10%\rdownloading 100%\r\nnext\r\ndone\r\n")
	if got != "downloading 100%\nnext\ndone\n" {
		t.Fatalf("混合处理错误: %q", got)
	}
	// 快照正好切在进度条中间：丢弃半成品那一行，前面的完整行保留
	got = Strip("ok\r\ndownloading 5")
	if got != "ok\ndownloading 5" {
		t.Fatalf("快照切分处理错误: %q", got)
	}
}

func TestStripKeepsMultibyte(t *testing.T) {
	// 中文与 emoji 必须整段保留，不能被按字节切开（也不能变成 U+FFFD）
	raw := "\x1b[32m磁盘已满\x1b[0m：/var 占用 92%\n"
	got := Strip(raw)
	if got != "磁盘已满：/var 占用 92%\n" {
		t.Fatalf("多字节处理错误: %q", got)
	}
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("出现了替换字符 U+FFFD")
	}
	// 退格删的是一个字符而不是一个字节
	if got := Strip("中文X\b\n"); got != "中文\n" {
		t.Fatalf("退格删除多字节字符错误: %q", got)
	}
}

func TestStripPreservesInvalidBytes(t *testing.T) {
	// 非法字节原样保留（不能替换成 U+FFFD，否则分不清是远端乱码还是我们改坏了）
	got := Strip("a\xffb\n")
	if got != "a\xffb\n" {
		t.Fatalf("非法字节被改动: %q", got)
	}
}

func TestStripBracketedPaste(t *testing.T) {
	got := Strip("\x1b[?2004hecho hi\x1b[?2004l\n")
	if got != "echo hi\n" {
		t.Fatalf("bracketed paste 标记未剥离: %q", got)
	}
}

func TestHasAltScreen(t *testing.T) {
	if !HasAltScreen("x\x1b[?1049hmore") {
		t.Fatalf("应识别备用屏切换")
	}
	if HasAltScreen("普通输出 no escape") {
		t.Fatalf("普通输出不应误判")
	}
}

func TestLooksLikePrompt(t *testing.T) {
	yes := []string{
		"[sudo] password for deploy: ",
		"Password: ",
		"Do you want to continue? [Y/n] ",
		"rm: remove regular file 'x'? (y/n) ",
		"请输入密码：",
		"即将删除数据，是否继续？(y/n)",
		"Press any key to continue . . .",
	}
	for _, s := range yes {
		if _, ok := LooksLikePrompt(s); !ok {
			t.Errorf("应识别为提示: %q", s)
		}
	}
	no := []string{
		"total 12\ndrwxr-xr-x 2 root root 4096 Jan 1 00:00 .",
		"Reading package lists... Done\nBuilding dependency tree... Done",
		"error: password authentication failed for user", // 出现在中间/结论里，不是等待输入
	}
	for _, s := range no {
		if pat, ok := LooksLikePrompt(s); ok {
			t.Errorf("不应识别为提示: %q (命中 %q)", s, pat)
		}
	}
	// 命令输出里先提到"密码"、最后才真的在等输入：要取最后出现的那个
	if pat, ok := LooksLikePrompt("正在校验密码\n用户名: admin\nPassword: "); !ok || !strings.Contains(strings.ToLower(pat), "password") {
		t.Errorf("应取最靠后的提示: %q %v", pat, ok)
	}
}

func TestFoldRepeats(t *testing.T) {
	got := FoldRepeats("a\na\nb\nc\nc\nc\nc\nd")
	want := "a\na\nb\nc    ×4\nd"
	if got != want {
		t.Fatalf("重复折叠错误:\n got %q\nwant %q", got, want)
	}
	// 空行不折叠（日志里的空行有分隔意义）
	if got := FoldRepeats("\n\n\n\nx"); got != "\n\n\n\nx" {
		t.Fatalf("空行不应折叠: %q", got)
	}
}

func TestTailLines(t *testing.T) {
	s := "1\n2\n3\n4\n5"
	if got := TailLines(s, 2); got != "4\n5" {
		t.Fatalf("TailLines = %q", got)
	}
	if got := TailLines(s, 99); got != s {
		t.Fatalf("行数足够时应原样返回")
	}
	if got := TailLines(s, 0); got != s {
		t.Fatalf("n=0 应返回全部")
	}
}

func TestElide(t *testing.T) {
	// 短文本不动
	if got, cut := Elide("short", 1024); cut || got != "short" {
		t.Fatalf("短文本不应截断: %q %v", got, cut)
	}
	var b strings.Builder
	for i := 0; i < 500; i++ {
		b.WriteString("log line ")
		b.WriteString(strings.Repeat("x", 40))
		b.WriteString("\n")
	}
	long := b.String()
	got, cut := Elide(long, 2048)
	if !cut {
		t.Fatalf("长文本应被截断")
	}
	if len(got) > 2600 {
		t.Fatalf("截断后仍然过长: %d", len(got))
	}
	if !strings.Contains(got, "已省略中间") {
		t.Fatalf("应标明省略了多少行: %q", got[:120])
	}
	// 头尾都要有内容
	lines := strings.Split(got, "\n")
	if !strings.HasPrefix(lines[0], "log line") {
		t.Fatalf("头部内容丢失: %q", lines[0])
	}
	if !strings.HasPrefix(lines[len(lines)-2], "log line") && !strings.HasPrefix(lines[len(lines)-1], "log line") {
		t.Fatalf("尾部内容丢失: %q", lines[len(lines)-1])
	}
}

func TestStripThenElideKeepsHeadAndTail(t *testing.T) {
	// 组合使用：先清洗再截断，不能出现半行乱码
	raw := strings.Repeat("\x1b[32mok\x1b[0m line\n", 2000)
	got, cut := Elide(Strip(raw), 1024)
	if !cut || strings.Contains(got, "\x1b") {
		t.Fatalf("组合处理异常: cut=%v 残留转义=%v", cut, strings.Contains(got, "\x1b"))
	}
}
