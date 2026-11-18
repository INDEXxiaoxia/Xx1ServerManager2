package main

import (
	"strings"
	"testing"
)

func TestContainerShellCommand(t *testing.T) {
	for _, engine := range []string{"docker", "podman"} {
		cmd, err := containerShellCommand(engine, "abcdef123456")
		if err != nil || cmd != engine+" exec -it abcdef123456 /bin/sh" {
			t.Fatalf("%s %v", cmd, err)
		}
		if strings.Contains(cmd, "||") {
			t.Fatal("must not fall back to host shell")
		}
	}
	for _, input := range [][2]string{{"docker;id", "abcdef123456"}, {"docker", "abc;whoami"}, {"docker", "--privileged"}, {"podman", ""}} {
		if _, err := containerShellCommand(input[0], input[1]); err == nil {
			t.Fatal(input)
		}
	}
}
