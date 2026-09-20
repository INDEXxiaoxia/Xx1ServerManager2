package main

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

// TestBuildTextBytesNeverAddsBOM 是本次需求的核心回归：
// 未带 BOM 的文件保存后绝对不能凭空多出 Windows BOM。
func TestBuildTextBytesNeverAddsBOM(t *testing.T) {
	out, err := buildTextBytes("hello\n", encUTF8, false, eolLF)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}
	if bytes.HasPrefix(out, utf8BOM) {
		t.Fatal("keepBOM=false 时不应写入 UTF-8 BOM")
	}
	if string(out) != "hello\n" {
		t.Fatalf("内容不符: %q", string(out))
	}
}

// TestBuildTextBytesKeepBOM 仅在显式要求时写入 BOM
func TestBuildTextBytesKeepBOM(t *testing.T) {
	out, err := buildTextBytes("hello\n", encUTF8, true, eolLF)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}
	if !bytes.HasPrefix(out, utf8BOM) {
		t.Fatal("keepBOM=true 时应写入 UTF-8 BOM")
	}
	if string(out[3:]) != "hello\n" {
		t.Fatalf("BOM 之后的正文不符: %q", string(out[3:]))
	}
}

// TestBuildTextBytesEOL 换行风格转换，尤其不能把 CRLF 变成 \r\r\n
func TestBuildTextBytesEOL(t *testing.T) {
	cases := []struct {
		name string
		text string
		eol  string
		want string
	}{
		{"LF 保持", "a\nb\n", eolLF, "a\nb\n"},
		{"LF 转 CRLF", "a\nb\n", eolCRLF, "a\r\nb\r\n"},
		{"CRLF 文本转 CRLF 不重复插入 \\r", "a\r\nb\r\n", eolCRLF, "a\r\nb\r\n"},
		{"CRLF 文本转 LF", "a\r\nb\r\n", eolLF, "a\nb\n"},
		{"混用换行统一为 CRLF", "a\r\nb\nc\r", eolCRLF, "a\r\nb\r\nc\r\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := buildTextBytes(c.text, encUTF8, false, c.eol)
			if err != nil {
				t.Fatalf("编码失败: %v", err)
			}
			if string(out) != c.want {
				t.Fatalf("转换结果不符: got %q want %q", string(out), c.want)
			}
		})
	}
}

// TestGB18030RoundTrip GB18030 文件按原编码写回后仍可解出同样的文字
func TestGB18030RoundTrip(t *testing.T) {
	const text = "中文内容\n第二行\n"
	enc, err := buildTextBytes(text, encGB18030, false, eolLF)
	if err != nil {
		t.Fatalf("GB18030 编码失败: %v", err)
	}
	if bytes.Equal(enc, []byte(text)) {
		t.Fatal("GB18030 编码结果不应与 UTF-8 相同（说明没有真正转码）")
	}
	decoded, ok := decodeText(encGB18030, enc)
	if !ok {
		t.Fatal("GB18030 解码失败")
	}
	if decoded != text {
		t.Fatalf("往返结果不符: got %q want %q", decoded, text)
	}
	// 同时确认识别环节能把这段字节判为 GB18030 而不是二进制
	cand := detectTextCandidate(enc)
	if cand.Binary {
		t.Fatal("GB18030 文本不应被判为二进制")
	}
	if cand.Encoding != encGB18030 {
		t.Fatalf("编码识别结果不符: got %s", cand.Encoding)
	}
}

// TestUnencodableChars 目标编码无法表示时应报错而不是静默替换。
// 注意：GB18030 是完整 Unicode 映射（含 emoji 的四字节区），GBK/GB2312 则不能表示 emoji。
func TestUnencodableChars(t *testing.T) {
	if _, err := buildTextBytes("emoji 🚀 测试", encGBK, false, eolLF); err == nil {
		t.Fatal("GBK 无法表示 emoji，应返回错误")
	}
	if _, err := buildTextBytes("emoji 🚀 测试", encGB2312, false, eolLF); err == nil {
		t.Fatal("GB2312 无法表示 emoji，应返回错误")
	}
	// GB18030 可以表示 emoji，且往返必须一致
	out, err := buildTextBytes("emoji 🚀 测试", encGB18030, false, eolLF)
	if err != nil {
		t.Fatalf("GB18030 应能编码 emoji: %v", err)
	}
	back, ok := decodeText(encGB18030, out)
	if !ok {
		t.Fatal("GB18030 解码失败")
	}
	if back != "emoji 🚀 测试" {
		t.Fatalf("往返结果不符: %q", back)
	}
}

// TestDetectTextCandidate 编码与二进制识别
func TestDetectTextCandidate(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		encoding string
		hasBOM   bool
		binary   bool
	}{
		{"纯 ASCII", []byte("hello world\n"), encUTF8, false, false},
		{"UTF-8 中文", []byte("中文内容\n"), encUTF8, false, false},
		{"UTF-8 带 BOM", append(append([]byte{}, utf8BOM...), []byte("hi\n")...), encUTF8, true, false},
		{"UTF-16LE BOM", []byte{0xFF, 0xFE, 0x68, 0x00}, encUTF16LE, true, false},
		{"UTF-16BE BOM", []byte{0xFE, 0xFF, 0x00, 0x68}, encUTF16BE, true, false},
		{"含 NUL 的二进制", []byte{0x7F, 0x45, 0x4C, 0x46, 0x00, 0x01, 0x02}, encBinary, false, true},
		{"控制字符为主", []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x0B, 0x0E, 0x0F, 0x10}, encBinary, false, true},
		{"空文件视为文本", []byte{}, encUTF8, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := detectTextCandidate(c.data)
			if got.Encoding != c.encoding {
				t.Fatalf("encoding: got %s want %s", got.Encoding, c.encoding)
			}
			if got.HasBOM != c.hasBOM {
				t.Fatalf("hasBOM: got %v want %v", got.HasBOM, c.hasBOM)
			}
			if got.Binary != c.binary {
				t.Fatalf("binary: got %v want %v", got.Binary, c.binary)
			}
		})
	}
}

// TestDetectEOL 换行统计
func TestDetectEOL(t *testing.T) {
	cases := []struct {
		name  string
		data  string
		eol   string
		mixed bool
	}{
		{"纯 LF", "a\nb\nc\n", eolLF, false},
		{"纯 CRLF", "a\r\nb\r\nc\r\n", eolCRLF, false},
		{"纯 CR", "a\rb\rc\r", eolCR, false},
		{"混用", "a\r\nb\nc\n", eolLF, true},
		{"无换行", "abc", eolLF, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			eol, mixed := detectEOL([]byte(c.data))
			if eol != c.eol {
				t.Fatalf("eol: got %s want %s", eol, c.eol)
			}
			if mixed != c.mixed {
				t.Fatalf("mixed: got %v want %v", mixed, c.mixed)
			}
		})
	}
}

// TestStripAndDetectBOM BOM 剥离与识别一致性
func TestStripAndDetectBOM(t *testing.T) {
	withBOM := append(append([]byte{}, utf8BOM...), []byte("body")...)
	if !detectHasBOM(encUTF8, withBOM) {
		t.Fatal("应识别到 UTF-8 BOM")
	}
	if got := string(stripBOM(withBOM)); got != "body" {
		t.Fatalf("剥离 BOM 结果不符: %q", got)
	}
	if detectHasBOM(encUTF8, []byte("body")) {
		t.Fatal("无 BOM 数据不应被识别为带 BOM")
	}
	// base64 往返（前端据 base64 还原文本）
	b64 := base64.StdEncoding.EncodeToString(stripBOM(withBOM))
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("base64 解码失败: %v", err)
	}
	if string(raw) != "body" {
		t.Fatalf("base64 往返结果不符: %q", string(raw))
	}
}

// TestNormalizeEOL 读取时换行统一为 LF（供文本区编辑）
func TestNormalizeEOL(t *testing.T) {
	if got := normalizeEOL("a\r\nb\rc\n"); got != "a\nb\nc\n" {
		t.Fatalf("归一化结果不符: %q", got)
	}
}

// TestEncodingWhitelist 编码白名单与归一化
func TestEncodingWhitelist(t *testing.T) {
	if got := normalizedEncoding("UTF8"); got != encUTF8 {
		t.Fatalf("UTF8 应归一化为 %s，got %s", encUTF8, got)
	}
	if got := normalizedEncoding("GB-18030"); got != encGB18030 {
		t.Fatalf("GB-18030 应归一化为 %s，got %s", encGB18030, got)
	}
	if got := normalizedEncoding("gb2312"); got != encGB2312 {
		t.Fatalf("gb2312 归一化失败: %s", got)
	}
	if charset(encGB2312) == nil {
		t.Fatal("GB2312 应映射到 GBK 解码表")
	}
	if charset(encGBK) == nil || charset(encGB18030) == nil {
		t.Fatal("GBK/GB18030 应有对应解码表")
	}
}

// TestBOMOnlyUTF8 非 UTF-8 编码不允许写 BOM
func TestBOMOnlyUTF8(t *testing.T) {
	if _, err := buildTextBytes("中文", encGB18030, true, eolLF); err == nil {
		t.Fatal("GB18030 不支持 BOM，应返回错误")
	}
}

// TestBuildTextBytesKeepsContent 多行内容与末尾换行不受影响
func TestBuildTextBytesKeepsContent(t *testing.T) {
	const text = "line1\nline2\n\nline4"
	out, err := buildTextBytes(text, encUTF8, false, eolLF)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}
	if string(out) != text {
		t.Fatalf("内容被改变: %q", string(out))
	}
	if strings.HasPrefix(string(out), "\uFEFF") {
		t.Fatal("不应写入 U+FEFF 字符形式的 BOM")
	}
}