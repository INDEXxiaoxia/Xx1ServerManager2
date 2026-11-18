package main

import (
	"ServerManager2/internal/models"
	"testing"
	"time"
)

func TestRootCacheIsolationAndExpiry(t *testing.T) {
	a := newTestApp(t)
	now := time.Now()
	one := &models.Server{ID: "one", Host: "a", Username: "user"}
	two := &models.Server{ID: "two", Host: "b", Username: "user"}
	a.elevated = map[string]rootGrant{"one": {"secret-one", now, rootIdentity(one)}, "two": {"secret-two", now, rootIdentity(two)}}
	if e := a.SetRootCacheMinutes(10); e != nil {
		t.Fatal(e)
	}
	if pw, ok := a.cachedRoot(one, now.Add(9*time.Minute)); !ok || pw != "secret-one" {
		t.Fatal("server one cache mismatch")
	}
	if pw, ok := a.cachedRoot(two, now); !ok || pw != "secret-two" {
		t.Fatal("server two cache mismatch")
	}
	if _, ok := a.cachedRoot(one, now.Add(10*time.Minute)); ok {
		t.Fatal("expired grant accepted")
	}
	if _, ok := a.cachedRoot(two, now); !ok {
		t.Fatal("other server grant removed")
	}
	two.UpdatedAt = now.Add(time.Minute)
	if _, ok := a.cachedRoot(two, now); !ok {
		t.Fatal("status update invalidated grant")
	}
	two.Host = "changed"
	if _, ok := a.cachedRoot(two, now); ok {
		t.Fatal("grant reused after host change")
	}
}
func TestRootCacheSessionAndSettings(t *testing.T) {
	a := newTestApp(t)
	n, e := a.GetRootCacheMinutes()
	if e != nil || n != 0 {
		t.Fatal(n, e)
	}
	sv := &models.Server{ID: "one"}
	now := time.Now()
	a.elevated = map[string]rootGrant{"one": {"pw", now, rootIdentity(sv)}}
	if _, ok := a.cachedRoot(sv, now.Add(48*time.Hour)); !ok {
		t.Fatal("session grant expired")
	}
	if e = a.SetRootCacheMinutes(60); e != nil {
		t.Fatal(e)
	}
	if _, ok := a.cachedRoot(sv, now.Add(61*time.Minute)); ok {
		t.Fatal("updated duration ignored")
	}
	if e = a.SetRootCacheMinutes(15); e == nil {
		t.Fatal("invalid cache duration accepted")
	}
}
