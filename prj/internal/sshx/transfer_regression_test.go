package sshx

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestUploadRejectsShortWrite(t *testing.T) {
	f, e := os.CreateTemp(t.TempDir(), "upload")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	f.WriteString("payload")
	f.Seek(0, io.SeekStart)
	if e := uploadAll(shortWriter{}, f, 7, nil); !errors.Is(e, io.ErrShortWrite) {
		t.Fatalf("expected short write, got %v", e)
	}
}
func TestBatchOutputBoundedWithoutInterruptingCommand(t *testing.T) {
	b := cappedOutput{limit: 8}
	for i := 0; i < 100; i++ {
		n, e := b.Write([]byte("abcdef"))
		if n != 6 || e != nil {
			t.Fatal(n, e)
		}
	}
	if b.Len() != 8 || !strings.Contains(b.String(), "截断") {
		t.Fatal(b.String())
	}
}
func TestUploadFinalProgress(t *testing.T) {
	f, e := os.CreateTemp(t.TempDir(), "upload")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	data := strings.Repeat("x", 100000)
	f.WriteString(data)
	f.Seek(0, io.SeekStart)
	var last int64
	calls := 0
	if e := uploadAll(io.Discard, f, int64(len(data)), func(n, total int64) { last = n; calls++ }); e != nil {
		t.Fatal(e)
	}
	if last != int64(len(data)) || calls > 2 {
		t.Fatalf("last=%d calls=%d", last, calls)
	}
}
