package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
)

func TestBackgroundPreviewDimensions(t *testing.T) {
	for _, tc := range []struct{ w, h, wantW, wantH int }{{32, 16, 32, 16}, {4096, 2160, 1920, 1012}, {64, 4096, 30, 1920}, {1, 4096, 1, 1920}} {
		var in bytes.Buffer
		if err := png.Encode(&in, image.NewGray(image.Rect(0, 0, tc.w, tc.h))); err != nil {
			t.Fatal(err)
		}
		preview, err := backgroundPreview("test.png", bytes.NewReader(in.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		raw, err := base64.StdEncoding.DecodeString(preview.Data)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
		if err != nil || cfg.Width != tc.wantW || cfg.Height != tc.wantH {
			t.Fatalf("unexpected preview: %+v %v", cfg, err)
		}
	}
}

func TestBackgroundPreviewRejectsOversizedHeader(t *testing.T) {
	// DecodeConfig accepts the dimensions before any pixel buffer is allocated.
	in := image.NewGray(image.Rect(0, 0, 16385, 1))
	var buf bytes.Buffer
	if err := png.Encode(&buf, in); err != nil {
		t.Fatal(err)
	}
	if _, err := backgroundPreview("wide.png", bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatal("accepted oversized input")
	}
	if _, err := backgroundPreview("bad.jpg", bytes.NewReader([]byte("broken"))); err == nil {
		t.Fatal("accepted invalid input")
	}
}

func TestBackgroundWebP(t *testing.T) {
	f, err := os.Open("frontend/src/assets/moe/idle.webp")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = backgroundPreview("idle.webp", f); err != nil {
		t.Fatal(err)
	}
}
