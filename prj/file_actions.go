package main

import (
	"bytes"
	"fmt"
	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type UploadEntry struct {
	Local     string `json:"local"`
	Relative  string `json:"relative"`
	Directory bool   `json:"directory"`
}

func (a *App) SelectUploadDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择上传目录"})
}
func (a *App) ListUploadEntries(paths []string) ([]UploadEntry, error) {
	entries := []UploadEntry{}
	for _, root := range paths {
		if !filepath.IsAbs(root) {
			return nil, fmt.Errorf("需要绝对路径")
		}
		err := filepath.WalkDir(root, func(local string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && !d.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(filepath.Dir(root), local)
			if err != nil {
				return err
			}
			entries = append(entries, UploadEntry{local, filepath.ToSlash(rel), d.IsDir()})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return entries, nil
}
func (a *App) PreviewRemoteImage(serverID, remote string) (*BackgroundPreview, error) {
	c, err := a.acquireFileClient(serverID)
	if err != nil {
		return nil, err
	}
	data, err := c.ReadAll(remote, 0, maxBackgroundBytes)
	if err != nil {
		return nil, err
	}
	return backgroundPreview(path.Base(remote), bytes.NewReader(data))
}
func (a *App) SaveContainerLogs(serverID, engine, id string, mb int) (string, error) {
	if (engine != "docker" && engine != "podman") || !regexp.MustCompile("^[a-fA-F0-9]{12,64}$").MatchString(id) || (mb != 1 && mb != 10 && mb != 50) {
		return "", fmt.Errorf("无效的日志请求")
	}
	sv, err := a.getServerWithPassword(serverID)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(regexp.MustCompile(`[<>:"/\\|?*[:cntrl:]]`).ReplaceAllString(sv.Name, "_"))
	if name == "" {
		name = "server"
	}
	runes := []rune(name)
	if len(runes) > 60 {
		name = string(runes[:60])
	}
	filename := fmt.Sprintf("%s-%s-%d.log", name, uuid.NewString()[:8], time.Now().UnixMilli())
	dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "保存容器日志", DefaultFilename: filename})
	if err != nil || dest == "" {
		return "", err
	}
	c := a.newServerClient(sv, 10)
	if err = c.Connect(); err != nil {
		return "", err
	}
	defer c.Close()
	cmd := fmt.Sprintf("bash -o pipefail -c '%s logs --timestamps %s 2>&1 | tail -c %d'", engine, id, mb<<20)
	code, out, msg := c.ExecCommand(cmd, 180, false)
	if code != 0 {
		return "", fmt.Errorf("读取日志失败：%.500s %.500s", msg, out)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".container-log-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.WriteString(out)
	closeErr := tmp.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	return dest, os.Rename(tmp.Name(), dest)
}

// Preview a single level without retaining large directory listings.
type DropListing struct {
	Count int           `json:"count"`
	Items []UploadEntry `json:"items"`
}

func (a *App) PreviewDropLevel(roots []string, directory string) (DropListing, error) {
	const limit = 40
	result := DropListing{}
	if directory == "" {
		result.Count = len(roots)
		if len(roots) > limit {
			return result, nil
		}
		for _, root := range roots {
			info, err := os.Lstat(root)
			if err != nil {
				return result, err
			}
			result.Items = append(result.Items, UploadEntry{root, filepath.Base(root), info.IsDir()})
		}
		return result, nil
	}
	file, err := os.Open(directory)
	if err != nil {
		return result, err
	}
	defer file.Close()
	for {
		entries, err := file.ReadDir(128)
		for _, item := range entries {
			result.Count++
			if result.Count <= limit {
				result.Items = append(result.Items, UploadEntry{filepath.Join(directory, item.Name()), item.Name(), item.IsDir()})
			} else {
				result.Items = nil
			}
		}
		if err == io.EOF {
			return result, nil
		}
		if err != nil {
			return result, err
		}
	}
}
