package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadEntriesPreserveRootAndEmptyDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "资料")
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a b"), []byte("sample"), 0644); err != nil {
		t.Fatal(err)
	}
	entries, err := new(App).ListUploadEntries([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[0].Relative != "资料" || entries[1].Relative != "资料/a b" || entries[1].Directory || entries[2].Relative != "资料/empty" || !entries[2].Directory {
		t.Fatal(entries)
	}
}
func TestDropPreviewLimitsEachLevel(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 41; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprint(i)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	app := new(App)
	top, err := app.PreviewDropLevel([]string{root}, "")
	if err != nil || top.Count != 1 || !top.Items[0].Directory {
		t.Fatalf("root: %+v %v", top, err)
	}
	children, err := app.PreviewDropLevel(nil, root)
	if err != nil || children.Count != 41 || len(children.Items) != 0 {
		t.Fatalf("children: %+v %v", children, err)
	}
}
