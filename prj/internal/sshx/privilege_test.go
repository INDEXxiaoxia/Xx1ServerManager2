package sshx

import (
	"strings"
	"testing"
)

func TestExplicitOperationRoot(t *testing.T) {
	c := New("host", 22, "user", "saved", 10)
	if strings.Contains(c.commandForOperation("id", false), "sudo") {
		t.Fatal("ordinary command elevated")
	}
	if !strings.Contains(c.commandForOperation("id", true), "sudo -S") {
		t.Fatal("explicit sudo ignored")
	}
	c.Username = "root"
	if c.commandForOperation("id", true) != "id" {
		t.Fatal("root wrapped")
	}
}
