package main

import (
	"strings"
	"testing"
)

func TestUploadCancelledBeforeStart(t *testing.T) {
	a := &App{}
	if err := a.CancelFileTransfer("early"); err != nil {
		t.Fatal(err)
	}
	if err := a.UploadFileTask("early", "", "", ""); err == nil || !strings.Contains(err.Error(), "已取消") {
		t.Fatalf("got %v", err)
	}
	if len(a.transfers) != 0 {
		t.Fatal("cancel marker not consumed")
	}
}
func TestAIExecutionMutualExclusion(t *testing.T) {
	a := &App{}
	a.aiExecutionMu.Lock()
	defer a.aiExecutionMu.Unlock()
	if _, err := a.AIExecOne("", "", "pwd"); err == nil || !strings.Contains(err.Error(), "已有 AI") {
		t.Fatalf("got %v", err)
	}
}
