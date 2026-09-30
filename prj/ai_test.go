package main

import (
	"strings"
	"testing"

	"ServerManager2/internal/ansi"
)

// ---- 注入与标记 ----

func TestBuildStepInjectSingleLine(t *testing.T) {
	inject, key := buildStepInject("ab12cd34", "df -h")
	if !strings.HasSuffix(inject, "\r") {
		t.Fatalf("注入必须以 \\r 结尾（PTY 靠 CR 提交命令）: %q", inject)
	}
	if !strings.Contains(inject, "df -h ; printf ") {
		t.Fatalf("单行命令应直接接 printf: %q", inject)
	}
	// 关键：注入文本本身不能包含完整标记，否则终端回显的命令行会命中标记，
	// 导致"命令还没跑"被误判成"已经跑完"
	if strings.Contains(inject, key) {
		t.Fatalf("注入文本里出现了完整标记 %q，回显会误命中: %q", key, inject)
	}
	if !strings.Contains(inject, "'ab12cd34'") {
		t.Fatalf("stepID 应作为单独参数传入: %q", inject)
	}
}

func TestBuildStepInjectMultiLine(t *testing.T) {
	inject, key := buildStepInject("ff00ff00", "cd /tmp\nls -l")
	if strings.Contains(inject, key) {
		t.Fatalf("多行注入同样不能出现完整标记: %q", inject)
	}
	// 多行命令：标记另起一行
	if !strings.Contains(inject, "cd /tmp\nls -l\nprintf ") {
		t.Fatalf("多行命令后标记应单独占一行: %q", inject)
	}
	if !strings.HasSuffix(inject, "\r") {
		t.Fatalf("末尾要有 \\r")
	}
}

// 模拟真实终端流：命令行回显 + 输出 + 隐藏标记
func TestFindMarkerAgainstRealisticStream(t *testing.T) {
	stepID := "deadbeef"
	inject, key := buildStepInject(stepID, "echo hello")

	// 终端把注入的内容回显出来，再输出命令结果，最后是 printf 的标记行
	stream := inject[:len(inject)-1] + "\r\n" + // 回显（readline 会把 \r 变成 CRLF）
		"hello\r\n" +
		"\n" + key + "0X1E\r\x1b[K" + // 标记：退出码 0，随后被擦除
		"user@host:~$ "

	idx, code, ok := findMarker(stream, key)
	if !ok {
		t.Fatalf("应找到标记")
	}
	if code != 0 {
		t.Fatalf("退出码 = %d, want 0", code)
	}
	// 标记位置必须在输出之后（不能落在回显的命令行里）
	if !strings.Contains(stream[:idx], "hello") {
		t.Fatalf("标记位置过早，输出没被包含进来")
	}
	out := stripEcho(ansi.Strip(stream[:idx]), "echo hello")
	if out != "hello" {
		t.Fatalf("提取到的输出 = %q, want %q", out, "hello")
	}

	// 这里固定一个很容易踩的坑：标记行末尾是 \r + ESC[K（把它从用户眼前擦掉），
	// 而 ansi.Strip 会把"单独的 \r 覆盖行"整行丢弃 —— 所以在清洗后的文本里
	// **永远找不到标记**。这就是代码必须拿原始字节流去找标记的原因。
	if _, _, ok := findMarker(ansi.Strip(stream), key); ok {
		t.Fatalf("清洗后的文本里不应还能找到标记（这正是要拿原始流找的原因）")
	}
}

func TestFindMarkerSkipsNonNumericCandidate(t *testing.T) {
	key := "X1S12345678D"
	// 先出现一个"标记后面不是数字"的伪命中，后面才是真的
	text := "noise " + key + "abc X1E more\n" + key + "127X1E\n"
	idx, code, ok := findMarker(text, key)
	if !ok || code != 127 {
		t.Fatalf("应跳过伪命中并取到 127: idx=%d code=%d ok=%v", idx, code, ok)
	}
	if idx < strings.Index(text, "more") {
		t.Fatalf("取到的是伪命中的位置")
	}
}

func TestFindMarkerAbsent(t *testing.T) {
	if _, _, ok := findMarker("just output, no marker", "X1SaaaabbbbD"); ok {
		t.Fatalf("没有标记时不应命中")
	}
	// 只有前缀没有结束符：说明命令还在跑
	if _, _, ok := findMarker("X1SaaaabbbbD0", "X1SaaaabbbbD"); ok {
		t.Fatalf("缺 X1E 结束符时不应判定完成")
	}
}

func TestStripEcho(t *testing.T) {
	out := stripEcho("\n\necho hello\nhello\n\n", "echo hello")
	if out != "hello" {
		t.Fatalf("stripEcho = %q", out)
	}
	// 命令很长时用前缀匹配
	long := "systemctl status nginx --no-pager -l"
	out = stripEcho(long+"\n● nginx.service - A high performance web server\n", long)
	if strings.Contains(out, "systemctl status") {
		t.Fatalf("长命令回显未被去掉: %q", out)
	}
	// 输出第一行不是回显时不能误删
	out = stripEcho("hello world\nsecond line\n", "echo hi")
	if out != "hello world\nsecond line" {
		t.Fatalf("不应误删输出: %q", out)
	}
}

func TestRandomStepID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id := randomStepID()
		if len(id) != 8 {
			t.Fatalf("stepID 长度应为 8: %q", id)
		}
		for _, c := range id {
			if !strings.ContainsRune("0123456789abcdef", c) {
				t.Fatalf("stepID 含非十六进制字符: %q", id)
			}
		}
		seen[id] = true
	}
	if len(seen) < 190 {
		t.Fatalf("stepID 重复率过高: %d/200", len(seen))
	}
}

// ---- 模型回复解析 ----

func TestParseAICommandReplyJSON(t *testing.T) {
	cmd, thought, finish, summary := parseAICommandReply(
		`{"thought":"先看看系统版本","command":"cat /etc/os-release","finish":false,"summary":""}`)
	if cmd != "cat /etc/os-release" || thought != "先看看系统版本" || finish || summary != "" {
		t.Fatalf("解析错误: cmd=%q thought=%q finish=%v summary=%q", cmd, thought, finish, summary)
	}
}

func TestParseAICommandReplyFenced(t *testing.T) {
	raw := "```json\n{\"thought\":\"看一下\",\"command\":\"df -h\",\"finish\":false}\n```"
	cmd, _, finish, _ := parseAICommandReply(raw)
	if cmd != "df -h" || finish {
		t.Fatalf("去围栏解析失败: %q", cmd)
	}
}

func TestParseAICommandReplyEmbeddedInProse(t *testing.T) {
	raw := "好的，我来处理：\n{\"thought\":\"安装依赖\",\"command\":\"apt-get install -y curl\",\"finish\":false}\n希望有帮助！"
	cmd, thought, _, _ := parseAICommandReply(raw)
	if cmd != "apt-get install -y curl" || thought != "安装依赖" {
		t.Fatalf("从散文里提取 JSON 失败: cmd=%q thought=%q", cmd, thought)
	}
}

func TestParseAICommandReplyFinish(t *testing.T) {
	cmd, _, finish, summary := parseAICommandReply(
		`{"thought":"都装好了","command":"","finish":true,"summary":"已安装 node 与 dsh"}`)
	if !finish || cmd != "" || summary != "已安装 node 与 dsh" {
		t.Fatalf("完成态解析错误: cmd=%q finish=%v summary=%q", cmd, finish, summary)
	}
}

// 模型没按约定输出时：当成纯文本、不执行任何命令、直接停下（绝不猜命令）
func TestParseAICommandReplyGarbageStops(t *testing.T) {
	cmd, thought, finish, summary := parseAICommandReply("我觉得你应该先检查一下磁盘空间。")
	if cmd != "" {
		t.Fatalf("解析失败时不应给出命令，得到 %q", cmd)
	}
	if !finish {
		t.Fatalf("解析失败时应停下")
	}
	if !strings.Contains(summary, "没有按约定") {
		t.Fatalf("应说明原因: %q", summary)
	}
	if !strings.Contains(thought, "磁盘空间") {
		t.Fatalf("应把原文当说明交给用户: %q", thought)
	}
}

func TestParseAICommandReplyCommandsArray(t *testing.T) {
	cmd, _, _, _ := parseAICommandReply(`{"commands":[{"command":"a"},{"command":"b"}],"finish":false}`)
	if cmd != "a\nb" {
		t.Fatalf("commands 数组应合并成多行命令: %q", cmd)
	}
}

func TestParseAISuggestReply(t *testing.T) {
	text, cmds := parseAISuggestReply(
		`{"text":"用 ps 看进程","commands":[{"command":"ps aux | grep xxx.jar","reason":"只读，列出匹配的进程"}]}`)
	if text != "用 ps 看进程" || len(cmds) != 1 || cmds[0].Command != "ps aux | grep xxx.jar" {
		t.Fatalf("建议模式解析错误: text=%q cmds=%+v", text, cmds)
	}
	if cmds[0].Reason == "" {
		t.Fatalf("应保留 reason")
	}
}

func TestParseAISuggestReplyFallbackCodeFence(t *testing.T) {
	raw := "可以这样查：\n\n```bash\nps aux | grep -i xxx.jar\n```\n"
	text, cmds := parseAISuggestReply(raw)
	if !strings.Contains(text, "可以这样查") {
		t.Fatalf("应保留原文作为回答: %q", text)
	}
	if len(cmds) != 1 || cmds[0].Command != "ps aux | grep -i xxx.jar" {
		t.Fatalf("应从代码块提取命令: %+v", cmds)
	}
}

func TestParseAISuggestReplyStripsPromptDollar(t *testing.T) {
	_, cmds := parseAISuggestReply("```\n$ df -h\n# 注释不算命令\ndu -sh /var/*\n```")
	if len(cmds) != 2 || cmds[0].Command != "df -h" || cmds[1].Command != "du -sh /var/*" {
		t.Fatalf("代码块提取错误: %+v", cmds)
	}
}

func TestFirstJSONObject(t *testing.T) {
	in := `prefix {"a":"} not a brace {","b":{"c":1}} suffix`
	got, ok := firstJSONObject(in)
	if !ok || got != `{"a":"} not a brace {","b":{"c":1}}` {
		t.Fatalf("平衡括号提取错误: %q", got)
	}
	if _, ok := firstJSONObject("no json here"); ok {
		t.Fatalf("无 JSON 时不应成功")
	}
}

func TestUnfence(t *testing.T) {
	if got := unfence("```json\n{\"a\":1}\n```"); got != `{"a":1}` {
		t.Fatalf("unfence = %q", got)
	}
	if got := unfence(`{"a":1}`); got != `{"a":1}` {
		t.Fatalf("无围栏应原样返回: %q", got)
	}
}

// ---- 风险判定 ----

func TestRiskOf(t *testing.T) {
	high := []string{"rm -rf /", "mkfs.ext4 /dev/sdb1", "dd if=/dev/zero of=/dev/sda", "shutdown -h now", "reboot"}
	for _, c := range high {
		if lv, _ := riskOf(c); lv != "high" {
			t.Errorf("%q 应为 high，得到 %s", c, lv)
		}
	}
	medium := []string{"apt-get install -y nginx", "systemctl restart nginx", "rm -f /tmp/x.log", "docker system prune", "sed -i 's/a/b/' f", "truncate -s 0 /var/log/x.log"}
	for _, c := range medium {
		if lv, _ := riskOf(c); lv != "medium" {
			t.Errorf("%q 应为 medium，得到 %s", c, lv)
		}
	}
	low := []string{"df -h", "ls -l /var/log", "ps aux --sort=-%mem | head", "cat /etc/os-release", "systemctl status nginx --no-pager", "free -m"}
	for _, c := range low {
		if lv, _ := riskOf(c); lv != "low" {
			t.Errorf("%q 应为 low，得到 %s", c, lv)
		}
	}
	if lv, reason := riskOf("rm -rf /data"); lv != "high" || reason == "" {
		t.Errorf("高风险应带说明: %s %q", lv, reason)
	}
	if lv, _ := riskOf(""); lv != "low" {
		t.Errorf("空命令应为 low")
	}
}

func TestClampInt(t *testing.T) {
	if clampInt(0, 1, 100, 20) != 20 {
		t.Error("0 应取默认值")
	}
	if clampInt(5, 1, 100, 20) != 5 {
		t.Error("正常值应保留")
	}
	if clampInt(999, 1, 100, 20) != 100 {
		t.Error("超上限应截断")
	}
	if clampInt(-3, 1, 100, 20) != 20 {
		t.Error("负数应取默认值")
	}
}
