package main

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// 支持在线编辑的文本编码。UTF-16 只识别不编辑（避免引入额外转码路径）。
const (
	encUTF8    = "utf-8"
	encGB18030 = "gb18030"
	encGBK     = "gbk"
	encGB2312  = "gb2312"
	encUTF16LE = "utf-16le"
	encUTF16BE = "utf-16be"
	encBinary  = "binary"
)

// 换行风格
const (
	eolLF   = "lf"
	eolCRLF = "crlf"
	eolCR   = "cr"
)

var (
	utf8BOM    = []byte{0xEF, 0xBB, 0xBF}
	utf16LEBOM = []byte{0xFF, 0xFE}
	utf16BEBOM = []byte{0xFE, 0xFF}
)

// textCandidate 编码识别结果
type textCandidate struct {
	Encoding string // 检测到的编码
	HasBOM   bool   // 是否带 BOM（UTF-8/UTF-16）
	Binary   bool   // 是否为二进制内容
	Reason   string // 不可编辑原因的补充说明
}

// detectTextCandidate 识别字节流的编码。
// 顺序：BOM 判定 → UTF-8 合法性 → GB18030 可解码性 → 二进制。
func detectTextCandidate(b []byte) textCandidate {
	switch {
	case bytes.HasPrefix(b, utf8BOM):
		return textCandidate{Encoding: encUTF8, HasBOM: true}
	case bytes.HasPrefix(b, utf16LEBOM):
		return textCandidate{Encoding: encUTF16LE, HasBOM: true, Reason: "UTF-16LE 编码暂不支持在线编辑"}
	case bytes.HasPrefix(b, utf16BEBOM):
		return textCandidate{Encoding: encUTF16BE, HasBOM: true, Reason: "UTF-16BE 编码暂不支持在线编辑"}
	}

	if utf8.Valid(b) {
		if isBinaryContent(b) {
			return textCandidate{Encoding: encBinary, Binary: true, Reason: "文件包含二进制内容"}
		}
		return textCandidate{Encoding: encUTF8}
	}

	if _, ok := decodeWith(simplifiedchinese.GB18030, b); ok {
		if isBinaryContent(b) {
			return textCandidate{Encoding: encBinary, Binary: true, Reason: "文件包含二进制内容"}
		}
		return textCandidate{Encoding: encGB18030}
	}

	return textCandidate{Encoding: encBinary, Binary: true, Reason: "无法识别为文本编码，可能是二进制文件"}
}

// isBinaryContent 采样前 8KB：出现 0x00 或不可打印字符占比过高即视为二进制。
// 注意：不可打印字符只统计 ASCII 控制符，中文等多字节字符不受影响。
func isBinaryContent(b []byte) bool {
	sample := b
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	if len(sample) == 0 {
		return false
	}
	bad := 0
	for _, c := range sample {
		if c == 0x00 {
			return true
		}
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' && c != 0x0C && c != 0x1B {
			bad++
		}
	}
	return bad*10 > len(sample) // 超过 10% 的控制字符
}

// detectEOL 统计换行风格，返回主要风格与是否混用
func detectEOL(b []byte) (string, bool) {
	var crlf, lf, cr int
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '\r':
			if i+1 < len(b) && b[i+1] == '\n' {
				crlf++
				i++
			} else {
				cr++
			}
		case '\n':
			lf++
		}
	}
	styles := 0
	for _, n := range []int{crlf, lf, cr} {
		if n > 0 {
			styles++
		}
	}
	mixed := styles > 1

	best, bestN := eolLF, lf
	if crlf > bestN {
		best, bestN = eolCRLF, crlf
	}
	if cr > bestN {
		best = eolCR
	}
	if crlf == 0 && lf == 0 && cr == 0 {
		best = eolLF
	}
	return best, mixed
}

// normalizeEOL 把任意换行统一为 LF，供前端文本区显示与编辑
func normalizeEOL(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// applyEOL 把 LF 文本转换为目标换行风格。
// 先统一为 LF 再替换，避免对已含 \r\n 的文本二次插入 \r 造成 \r\r\n。
func applyEOL(s, eol string) string {
	s = normalizeEOL(s)
	if eol == eolCRLF {
		return strings.ReplaceAll(s, "\n", "\r\n")
	}
	if eol == eolCR {
		return strings.ReplaceAll(s, "\n", "\r")
	}
	return s
}

// charset 返回编码对应的 charset（不支持时返回 nil）。
// GBK/GB2312 说明：golang.org/x/text 只提供 GB18030 与 GBK 两张表，
// GB2312 是 GBK 的子集，因此 GB2312 选项按 GBK 解码（超集解码不会产生乱码）。
func charset(name string) encoding.Encoding {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case encUTF8, "utf8", "":
		return nil // UTF-8 走原生路径，不经过 transform
	case encGB18030, "gb-18030":
		return simplifiedchinese.GB18030
	case encGBK, "cp936", "windows936", encGB2312:
		return simplifiedchinese.GBK
	default:
		return nil
	}
}

// normalizedEncoding 归一化编码名
func normalizedEncoding(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "utf8", encUTF8:
		return encUTF8
	case "gb-18030", encGB18030:
		return encGB18030
	case "cp936", "windows936", encGBK:
		return encGBK
	case encGB2312:
		return encGB2312
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

// decodeText 按编码解码字节流为 UTF-8 文本。
// GB18030 等宽字符集解码失败时不会返回 error，而是产出 U+FFFD，
// 因此这里额外校验是否存在替换符，避免把乱码当成正常内容打开。
func decodeText(name string, b []byte) (string, bool) {
	cs := charset(name)
	if cs == nil {
		if !utf8.Valid(b) {
			return "", false
		}
		return string(b), true
	}
	out, ok := decodeWith(cs, b)
	if !ok {
		return "", false
	}
	if strings.ContainsRune(out, utf8.RuneError) {
		return "", false
	}
	return out, true
}

func decodeWith(cs encoding.Encoding, b []byte) (string, bool) {
	dec := cs.NewDecoder()
	out, _, err := transform.Bytes(dec, b)
	if err != nil {
		return "", false
	}
	return string(out), true
}

// encodeText 把 UTF-8 文本按目标编码编码为字节（不含 BOM）。
// 用 transform.String：输入是 Go 字符串（UTF-8），transform.Bytes 会按源编码的
// Decoder 解析一次输入，语义不对（会把中文当成非法输入而误报错误）。
// 无法表示的字符由 x/text 的 RepertoireError 报出，这里直接透传为可读的中文错误，
// 不做「把编码结果当字符串查 U+FFFD」的判断 —— 编码结果不是 UTF-8，那样查必然误判。
func encodeText(name, text string) ([]byte, error) {
	cs := charset(name)
	if cs == nil {
		return []byte(text), nil
	}
	out, _, err := transform.String(cs.NewEncoder(), text)
	if err != nil {
		return nil, fmt.Errorf("当前编码 %s 无法表示文本中的部分字符（如表情符号），请改用 UTF-8：%v", normalizedEncoding(name), err)
	}
	return []byte(out), nil
}

// buildTextBytes 组装最终写入远端的字节：编码 + 可选 BOM。
// keepBOM 为假时绝不添加 BOM —— 这是「不要给文件加 Windows BOM」的保证。
func buildTextBytes(text, encName string, keepBOM bool, eol string) ([]byte, error) {
	body, err := encodeText(encName, applyEOL(text, eol))
	if err != nil {
		return nil, err
	}
	if !keepBOM {
		return body, nil
	}
	if normalizedEncoding(encName) != encUTF8 {
		return nil, fmt.Errorf("仅 UTF-8 支持 BOM，当前编码为 %s", normalizedEncoding(encName))
	}
	return append(append([]byte{}, utf8BOM...), body...), nil
}