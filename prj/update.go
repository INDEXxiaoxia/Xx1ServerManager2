package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mattn/go-ieproxy"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const releaseRepo = "INDEXxiaoxia/Xx1ServerManager2"
const installerName = "ServerManager2-amd64-installer.exe"

var releaseTransport = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = ieproxy.GetProxyFunc()
	return transport
}()

type releaseAsset struct {
	Name   string
	Digest string
	Size   int64
}
type stableRelease struct {
	Tag    string `json:"tag_name"`
	Assets []releaseAsset
}

func fetchRelease() (stableRelease, error) {
	var release stableRelease
	client := http.Client{Transport: releaseTransport, Timeout: 20 * time.Second}
	response, err := client.Get("https://api.github.com/repos/" + releaseRepo + "/releases/latest")
	if err != nil {
		return release, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return release, fmt.Errorf("GitHub: %s", response.Status)
	}
	err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&release)
	if err == nil && !regexp.MustCompile("^[vV]?[0-9]+[.][0-9]+[.][0-9]+$").MatchString(release.Tag) {
		err = fmt.Errorf("无法识别版本号")
	}
	return release, err
}
func (a *App) LatestVersion() (string, error) {
	release, err := fetchRelease()
	return release.Tag, err
}
func (a *App) DownloadUpdate(expected string) (string, error) {
	release, err := fetchRelease()
	if err != nil {
		return "", err
	}
	if release.Tag != expected {
		return "", fmt.Errorf("版本已变化，请重新检查更新")
	}
	for _, asset := range release.Assets {
		if asset.Name != installerName {
			continue
		}
		path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "保存更新安装包", DefaultFilename: "ServerManager2-" + release.Tag + "-installer.exe"})
		if err != nil || path == "" {
			return "", err
		}
		client := http.Client{Transport: releaseTransport, Timeout: 10 * time.Minute}
		resp, err := client.Get("https://github.com/" + releaseRepo + "/releases/download/" + release.Tag + "/" + installerName)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("GitHub: %s", resp.Status)
		}
		file, err := os.CreateTemp(filepath.Dir(path), ".servermanager-update-*")
		if err != nil {
			return "", err
		}
		defer os.Remove(file.Name())
		hash := sha256.New()
		n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, asset.Size+1))
		closeErr := file.Close()
		if err != nil {
			return "", err
		}
		if closeErr != nil {
			return "", closeErr
		}
		if n != asset.Size || !strings.EqualFold(asset.Digest, fmt.Sprintf("sha256:%x", hash.Sum(nil))) {
			return "", fmt.Errorf("下载校验失败，请重试")
		}
		if err = os.Rename(file.Name(), path); err != nil {
			return "", err
		}
		return path, nil
	}
	return "", fmt.Errorf("该版本未提供 Windows 安装包")
}

type RecentVersion struct {
	Tag  string
	Date string
	Body string
}

func (a *App) RecentVersions() ([]RecentVersion, error) {
	client := http.Client{Transport: releaseTransport, Timeout: 20 * time.Second}
	response, err := client.Get("https://api.github.com/repos/" + releaseRepo + "/releases?per_page=3")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub: %s", response.Status)
	}
	var records []struct {
		Tag  string `json:"tag_name"`
		Date string `json:"published_at"`
		Body string `json:"body"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&records); err != nil {
		return nil, err
	}
	versions := make([]RecentVersion, 0, len(records))
	for _, record := range records {
		versions = append(versions, RecentVersion{Tag: record.Tag, Date: record.Date, Body: record.Body})
	}
	return versions, nil
}
