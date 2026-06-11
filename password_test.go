package main

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	hash := hashPassword("pw")
	if hash == "" {
		t.Fatal("hashPassword() returned empty hash")
	}
	if !checkPassword("pw", hash) {
		t.Fatal("checkPassword() rejected matching bcrypt password")
	}
	if checkPassword("wrong", hash) {
		t.Fatal("checkPassword() accepted wrong bcrypt password")
	}
}

func TestCheckPasswordSupportsLegacySHA256Format(t *testing.T) {
	salt := "somesalt"
	sum := sha256.Sum256([]byte(salt + "pw"))
	stored := salt + ":" + hex.EncodeToString(sum[:])

	if !checkPassword("pw", stored) {
		t.Fatal("checkPassword() rejected matching legacy password")
	}
	if checkPassword("wrong", stored) {
		t.Fatal("checkPassword() accepted wrong legacy password")
	}
}
