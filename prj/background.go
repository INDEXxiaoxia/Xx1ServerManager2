package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const maxBackgroundBytes = 32 << 20

type BackgroundPreview struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func backgroundPreview(name string, r io.ReadSeeker) (*BackgroundPreview, error) {
	cfg, kind, err := image.DecodeConfig(r)
	if err != nil || (kind != "png" && kind != "jpeg" && kind != "webp") {
		return nil, fmt.Errorf("请选择有效的 JPG、PNG 或 WebP 图片")
	}
	w, h := cfg.Width, cfg.Height
	if w <= 0 || h <= 0 || w > 16384 || h > 16384 || int64(w)*int64(h) > 32_000_000 {
		return nil, fmt.Errorf("图片尺寸过大，请选择 3200 万像素以内、单边不超过 16384 像素的图片")
	}
	if _, err = r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	src, _, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("图片数据损坏，无法读取")
	}
	if longest := max(w, h); longest > 1920 {
		w = max(1, w*1920/longest)
		h = max(1, h*1920/longest)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	var buf bytes.Buffer
	if err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return &BackgroundPreview{Name: name, Data: base64.StdEncoding.EncodeToString(buf.Bytes())}, nil
}

func (a *App) ChooseBackgroundImage() (*BackgroundPreview, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择背景图片", Filters: []runtime.FileFilter{{DisplayName: "背景图片", Pattern: "*.jpg;*.jpeg;*.png;*.webp"}}})
	if err != nil || path == "" {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > maxBackgroundBytes {
		return nil, fmt.Errorf("请选择 32 MiB 以内的图片文件")
	}
	return backgroundPreview(filepath.Base(path), io.NewSectionReader(f, 0, stat.Size()))
}

// Used once to migrate originals saved by the previous desktop build.
func (a *App) ConvertSavedBackground(name, data string) (*BackgroundPreview, error) {
	if len(data) > base64.StdEncoding.EncodedLen(maxBackgroundBytes) {
		return nil, fmt.Errorf("已保存的背景过大")
	}
	raw, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, strings.NewReader(data)))
	if err != nil {
		return nil, err
	}
	return backgroundPreview(name, bytes.NewReader(raw))
}
