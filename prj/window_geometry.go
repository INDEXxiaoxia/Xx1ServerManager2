package main

import (
	"context"
	"encoding/json"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"os"
	"path/filepath"
)

type windowGeometry struct {
	Width, Height int
	Maximized     bool
}

func windowGeometryPath() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ServerManager2", "window.json")
}
func loadWindowGeometry() windowGeometry {
	var value windowGeometry
	data, err := os.ReadFile(windowGeometryPath())
	if err != nil || json.Unmarshal(data, &value) != nil || value.Width < 760 || value.Height < 480 {
		return windowGeometry{Width: 1000, Height: 680}
	}
	return value
}
func (g windowGeometry) startState() options.WindowStartState {
	if g.Maximized {
		return options.Maximised
	}
	return options.Normal
}
func (g windowGeometry) saveBeforeClose(ctx context.Context) bool {
	if runtime.WindowIsMinimised(ctx) {
		return false
	}
	g.Maximized = runtime.WindowIsMaximised(ctx)
	if !g.Maximized {
		w, h := runtime.WindowGetSize(ctx)
		if w >= 760 && h >= 480 {
			g.Width, g.Height = w, h
		}
	}
	data, _ := json.Marshal(g)
	_ = os.WriteFile(windowGeometryPath(), data, 0600)
	return false
}
