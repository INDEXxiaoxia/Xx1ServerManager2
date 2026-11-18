package main

import (
	"ServerManager2/internal/models"
	"strings"
	"testing"
)

func TestKeyConfigEncryptedAndPreserved(t *testing.T) {
	a := newTestApp(t)
	if e := a.ensureMachineKey(); e != nil {
		t.Fatal(e)
	}
	s, e := a.SaveServer(models.Server{Name: "key", Host: "host", Username: "user", AuthType: "key", PrivateKey: "private-key-sentinel", KeyPassphrase: "pass-sentinel", DefaultRoot: true, SudoPassword: "sudo-sentinel"})
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(storedCipher(t, a, s.ID), "sentinel") {
		t.Fatal("credential stored in plaintext")
	}
	if s.PrivateKey != "" || s.SudoPassword != "" {
		t.Fatal("save response leaked secrets")
	}
	s.Name = "edited"
	if _, e = a.SaveServer(s); e != nil {
		t.Fatal(e)
	}
	got, e := a.getServerWithPassword(s.ID)
	if e != nil {
		t.Fatal(e)
	}
	if got.PrivateKey != "private-key-sentinel" || got.KeyPassphrase != "pass-sentinel" || !got.DefaultRoot {
		t.Fatal("credentials not preserved")
	}
	list, e := a.ListServers()
	if e != nil {
		t.Fatal(e)
	}
	if list[0].PrivateKey != "" || !list[0].KeySet {
		t.Fatal("public config incorrect")
	}
}
