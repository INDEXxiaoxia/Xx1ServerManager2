// Package ansi 把 SSH PTY 的原始字节流清洗成"人看起来的样子"，
// 供 AI 读取终端内容时使用。
//
// 为什么必须清洗：
//  1. 颜色、窗口标题、bracketed paste 标记对模型完全是噪声；
//  2. 进度条 / spinner 会用 \r 反复覆盖同一行，一次 yum install 就能产生上万行"半成品"，
//     按"保留最后一个 \r 之后的内容"折叠后体积能降一个数量级，且信息不丢（最终状态才是结果）；
//  3. 多字节中文必须整段处理，不能按字节切，否则会出现乱码（本项目对 U+FFFD 替换很敏感）。
package ansi

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// Strip 清洗原始输出：剥离转义序列、折叠 \r 覆盖、处理退格、丢弃其他控制字符。
//
// 关于 \r 的处理是本函数最容易写错的地方：
//
//	"abc\r\n"  是正常的行结束（PTY 输出基本都是 CRLF）→ 必须原样保留换行；
//	"50%\r100%" 是进度条覆盖同一行 → 只保留最后一段。
//
// 所以只有"单独的 \r"（后面不跟 \n）才当作回到行首丢弃本行内容。
func Strip(raw string) string {
	var out []byte
	line := make([]byte, 0, 256) // 当前行（按 rune 对齐的字节）
	flush := func() {
		out = append(out, line...)
		line = line[:0]
	}
	i := 0
	for i < len(raw) {
		b := raw[i]
		switch {
		case b == 0x1b: // ESC 开头的转义序列
			i += skipEscape(raw[i:])
			continue
		case b == '\r':
			if i+1 < len(raw) && raw[i+1] == '\n' {
				flush()
				out = append(out, '\n')
				i += 2
				continue
			}
			// 单独的 \r：回到行首，丢弃本行已写内容（进度条/spinner 的覆盖行为）
			line = line[:0]
			i++
			continue
		case b == '\n':
			flush()
			out = append(out, '\n')
			i++
			continue
		case b == 0x08: // 退格：删掉最后一个字符
			line = dropLastRune(line)
			i++
			continue
		case b == '\t':
			line = append(line, '\t')
			i++
			continue
		case b < 0x20 || b == 0x7f: // 其余控制字符（含 DEL）直接丢弃
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(raw[i:])
		if r == utf8.RuneError && size <= 1 {
			// 非法字节：原样保留，绝不用 U+FFFD 替换（否则就分不清"远端本来就是乱码"和"被我们改坏了"）
			line = append(line, b)
			i++
			continue
		}
		line = append(line, raw[i:i+size]...)
		i += size
	}
	// 末尾尚未结束的行（快照正好切在行中间）也要保留，否则分块清洗再拼接会粘行
	flush()
	return string(out)
}

// skipEscape 返回从 ESC 开始应该跳过多少字节
func skipEscape(s string) int {
	if len(s) < 2 {
		return 1
	}
	switch s[1] {
	case '[': // CSI: ESC [ 参数 中间字节 终止字节(0x40-0x7E)
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
		}
		return len(s)
	case ']', 'P', '^', '_': // OSC / DCS / PM / APC：到 BEL 或 ST(ESC \) 结束
		for i := 2; i < len(s); i++ {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
		return len(s)
	case '(', ')', '*', '+', '#': // 字符集指定符：ESC ( B 之类
		if len(s) < 3 {
			return len(s)
		}
		return 3
	default: // 两字节转义（如 ESC = ESC >）
		return 2
	}
}

// dropLastRune 删掉最后一个 rune（退格）
func dropLastRune(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	// 从尾部回退到上一个 rune 边界
	i := len(b) - 1
	for i > 0 && b[i]&0xc0 == 0x80 {
		i--
	}
	return b[:i]
}

// HasAltScreen 判断流里是否出现过"切到备用屏"（vim / top / less 这类全屏交互程序启动）。
// 用于接管模式下识别"命令进了交互式程序、提示符可能永远不回来"。
func HasAltScreen(raw string) bool {
	return strings.Contains(raw, "\x1b[?1049h") ||
		strings.Contains(raw, "\x1b[?47h") ||
		strings.Contains(raw, "\x1b[?1047h")
}

// promptPatterns 「等着人工输入」的提示词（匹配时统一转小写，所以只需列小写形式）。
//
// 命中任意一条就要暂停 AI 并交给用户——盲目继续注入命令只会把终端搅乱。
// 注意取舍：宁可偶尔误判（用户可以点"继续等待"），也不要漏判导致整条接管链卡死。
// 因此中文只取"提示味"明确的词，不用孤零零的"确认"（正常输出里太常见）。
var promptPatterns = []string{
	"password for", "password:", "passwd:", "[sudo]", "sudo password",
	"密码：", "密码:", "请输入密码", "请输入",
	"(yes/no)", "[yes/no]", "(y/n)", "[y/n]", "are you sure", "do you want",
	"continue?", "overwrite?", "press any key", "请按回车",
	"是否继续", "是否确认", "确认吗", "继续吗", "请确认",
}

// LooksLikePrompt 在文本尾部寻找"等待输入"的提示，返回命中的片段。
// 只看尾部 400 个字符：命令的输出里出现 "密码" 这种词很正常，只有出现在最后才是提示。
func LooksLikePrompt(s string) (string, bool) {
	tail := s
	if len(tail) > 400 {
		tail = tail[len(tail)-400:]
	}
	lower := strings.ToLower(tail)
	// 逐个提示词找，取最靠后的一个（最新出现的提示才是当前卡住的地方）
	best := -1
	for _, p := range promptPatterns {
		if idx := strings.LastIndex(lower, p); idx > best {
			best = idx
		}
	}
	if best < 0 {
		return "", false
	}
	snippet := tail[best:]
	if i := strings.IndexByte(snippet, '\n'); i >= 0 {
		snippet = snippet[:i]
	}
	snippet = strings.TrimSpace(snippet)
	if len(snippet) > 120 {
		snippet = snippet[:120] + "…"
	}
	return snippet, true
}

// FoldRepeats 把连续重复的行折叠成 "行 ×N"（≥3 次才折，重复两次通常是正常的对照输出）。
// 日志里同一条错误刷屏时能省下大量 token。
func FoldRepeats(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		j := i + 1
		for j < len(lines) && lines[j] == lines[i] {
			j++
		}
		n := j - i
		if n >= 3 && strings.TrimSpace(lines[i]) != "" {
			out = append(out, lines[i]+"    ×"+strconv.Itoa(n))
		} else {
			for k := i; k < j; k++ {
				out = append(out, lines[k])
			}
		}
		i = j
	}
	return strings.Join(out, "\n")
}

// TailLines 取最后 n 行（n<=0 时返回全部）
func TailLines(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// Elide 超长文本按"头部 20% + 尾部 80%"保留，中间标明省略了多少行。
//
// 为什么头尾都要：头部交代"在跑什么"，尾部是结论（错在哪、装完了没），
// 中间通常是最没价值的重复日志。
func Elide(s string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s, false
	}
	headBudget := maxBytes / 5
	tailBudget := maxBytes - headBudget

	lines := strings.Split(s, "\n")
	// 从头部累加到 headBudget
	headEnd := 0
	used := 0
	for i, ln := range lines {
		used += len(ln) + 1
		if used > headBudget {
			headEnd = i
			break
		}
	}
	if headEnd == 0 {
		headEnd = 1
	}
	// 从尾部累加到 tailBudget
	tailStart := len(lines)
	used = 0
	for i := len(lines) - 1; i >= headEnd; i-- {
		used += len(lines[i]) + 1
		if used > tailBudget {
			tailStart = i + 1
			break
		}
		tailStart = i
	}
	if tailStart < headEnd {
		tailStart = headEnd
	}
	omitted := tailStart - headEnd
	head := strings.Join(lines[:headEnd], "\n")
	tail := strings.Join(lines[tailStart:], "\n")
	return head + "\n… 已省略中间 " + strconv.Itoa(omitted) + " 行 …\n" + tail, true
}
