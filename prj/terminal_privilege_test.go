package main

import (
	"ServerManager2/internal/models"
	"testing"
	"time"
)

func TestHostTerminalDoesNotInheritDockerGrant(t *testing.T) {
	a := newTestApp(t)
	sv := &models.Server{ID: "server-a", Host: "example", Username: "yw", Password: "login-password"}
	a.elevated = map[string]rootGrant{sv.ID: {"sudo-password", time.Now(), rootIdentity(sv)}}
	host := a.newTerminalClient(sv, "")
	if host.Username != "yw" || host.DefaultRoot {
		t.Fatal("host login inherited operation elevation")
	}
	if root, _ := host.RefreshRoot(); root {
		t.Fatal("PTY refresh re-enabled temporary elevation")
	}
	operation := a.newServerClient(sv, 10)
	if !operation.DefaultRoot || operation.SudoPassword != "sudo-password" {
		t.Fatal("host login changed cached Docker authorization")
	}
	container := a.newTerminalClient(sv, "docker exec -it abcdef123456 /bin/sh")
	if !container.DefaultRoot {
		t.Fatal("container operation lost cached authorization")
	}
}
func TestHostTerminalHonorsExplicitDefaultRoot(t *testing.T) {
	a := newTestApp(t)
	sv := &models.Server{ID: "server-b", Username: "admin", DefaultRoot: true, SudoPassword: "configured"}
	a.elevated = map[string]rootGrant{sv.ID: {"temporary", time.Now(), rootIdentity(sv)}}
	host := a.newTerminalClient(sv, "")
	root, pw := host.RefreshRoot()
	if !root || !host.DefaultRoot || pw != "configured" {
		t.Fatal("explicit root login configuration was not preserved")
	}
}
