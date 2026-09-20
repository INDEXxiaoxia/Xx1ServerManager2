package main

import (
	"fmt"
	"path"
	"strings"
)

// 远程路径均为 POSIX 风格（服务器端），因此统一使用 path 而非 filepath，
// 避免在 Windows 上构建时把 "/" 当作分隔符处理错误。

// cleanRemotePath 规范化远程绝对路径：
// 折叠重复斜杠、解析 . 与 ..、去掉末尾斜杠（根目录除外）。
func cleanRemotePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	p = strings.ReplaceAll(p, "\\", "/")
	cleaned := path.Clean(p)
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

// remoteAbs 把相对路径解析为绝对路径。
// 说明：SFTP 的请求通常以 home 目录为基准，前端展示统一使用绝对路径，
// 因此这里用 home 作为相对路径的基准。
func remoteAbs(base, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return cleanRemotePath(base)
	}
	if strings.HasPrefix(p, "/") {
		return cleanRemotePath(p)
	}
	if base == "" {
		base = "/"
	}
	return cleanRemotePath(base + "/" + p)
}

// joinRemotePath 拼接远程路径，正确处理根目录与末尾斜杠
func joinRemotePath(dir, name string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" || dir == "/" {
		return "/" + strings.TrimPrefix(name, "/")
	}
	return strings.TrimSuffix(dir, "/") + "/" + strings.TrimPrefix(name, "/")
}

// dirNameRemote 取远程路径的父目录（根目录返回 "/"）
func dirNameRemote(p string) string {
	p = cleanRemotePath(p)
	if p == "/" {
		return "/"
	}
	d := path.Dir(p)
	if d == "." || d == "" {
		return "/"
	}
	return d
}

// baseNameRemote 取远程路径的文件名
func baseNameRemote(p string) string {
	p = strings.TrimSuffix(cleanRemotePath(p), "/")
	if p == "" || p == "/" {
		return "/"
	}
	return path.Base(p)
}

// pathBreadcrumb 生成面包屑片段：/var/log -> [{"name":"var","path":"/var"},{"name":"log","path":"/var/log"}]
func pathBreadcrumb(p string) []map[string]string {
	p = cleanRemotePath(p)
	if p == "/" {
		return nil
	}
	segs := strings.Split(strings.Trim(p, "/"), "/")
	out := make([]map[string]string, 0, len(segs))
	cur := ""
	for _, s := range segs {
		if s == "" {
			continue
		}
		cur += "/" + s
		out = append(out, map[string]string{"name": s, "path": cur})
	}
	return out
}

// isPermissionError 判断是否为权限类错误（用于给出更准确的中文提示）
func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permission denied") || strings.Contains(msg, "access denied")
}

// isNotExistError 判断是否为“不存在”类错误
func isNotExistError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such file") || strings.Contains(msg, "not exist") || strings.Contains(msg, "not found")
}

// describePathError 把远端错误翻译为面向用户的中文提示
func describePathError(p string, err error) string {
	switch {
	case isNotExistError(err):
		return fmt.Sprintf("目录不存在：%s", p)
	case isPermissionError(err):
		return fmt.Sprintf("没有访问权限：%s（当前账号无权读取该目录）", p)
	default:
		return fmt.Sprintf("无法访问 %s：%v", p, err)
	}
}