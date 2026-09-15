package main

import (
	"bytes"
	"context"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := hashPassword("latihanIELTS2026")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword("latihanIELTS2026", hash) {
		t.Fatal("valid password did not match its hash")
	}
	if verifyPassword("password-yang-salah", hash) {
		t.Fatal("invalid password matched")
	}
}

func TestValidatePassword(t *testing.T) {
	for _, value := range []string{"short1", "tanpaangkaaa", "1234567890"} {
		if validatePassword(value) == "" {
			t.Fatalf("invalid password %q accepted", value)
		}
	}
	if message := validatePassword("belajar2026"); message != "" {
		t.Fatalf("valid password rejected: %s", message)
	}
}

func TestUserIDFromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), authContextKey{}, authUser{ID: 42})
	if got := userID(ctx); got != 42 {
		t.Fatalf("userID=%d, want 42", got)
	}
	if got := userID(context.Background()); got != 0 {
		t.Fatalf("anonymous userID=%d, want 0", got)
	}
}

func TestAuthUIStartsInPendingState(t *testing.T) {
	page, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(page, []byte(`id="session-gate"`)) {
		t.Fatal("session gate is missing")
	}
	if !bytes.Contains(page, []byte(`id="auth-shell" aria-labelledby="auth-title" hidden`)) {
		t.Fatal("auth form must remain hidden until the session check settles")
	}

	script, err := webFiles.ReadFile("web/auth.js")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(script, []byte(`$("#session-gate").hidden = true`)) < 2 {
		t.Fatal("both authenticated and signed-out outcomes must dismiss the session gate")
	}
}
