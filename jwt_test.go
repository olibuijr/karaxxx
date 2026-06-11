package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const testJWTSecret = "test-secret-at-least-32-bytes-long-xx"

func withTestJWTSecret(t *testing.T) {
	t.Helper()
	original := jwtSecret
	jwtSecret = testJWTSecret
	t.Cleanup(func() {
		jwtSecret = original
	})
}

func signTestJWT(t *testing.T, payload jwtPayload) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal(payload) failed: %v", err)
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := header + "." + payloadB64

	mac := hmac.New(sha256.New, []byte(jwtSecret))
	if _, err := mac.Write([]byte(signingInput)); err != nil {
		t.Fatalf("mac.Write() failed: %v", err)
	}

	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig
}

func TestCreateTokenRoundTrip(t *testing.T) {
	withTestJWTSecret(t)

	token := createToken(42, "alice")
	if token == "" {
		t.Fatal("createToken() returned empty token")
	}

	uid, username, ok := parseToken(token)
	if !ok {
		t.Fatal("parseToken() rejected token from createToken()")
	}
	if uid != 42 || username != "alice" {
		t.Fatalf("parseToken() = (%d, %q, %t), want (%d, %q, %t)", uid, username, ok, 42, "alice", true)
	}
}

func TestParseTokenRejectsTamperedToken(t *testing.T) {
	withTestJWTSecret(t)

	token := createToken(42, "alice")
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	if strings.HasSuffix(parts[2], "a") {
		parts[2] = parts[2][:len(parts[2])-1] + "b"
	} else {
		parts[2] = parts[2][:len(parts[2])-1] + "a"
	}

	if _, _, ok := parseToken(strings.Join(parts, ".")); ok {
		t.Fatal("parseToken() accepted tampered token")
	}
}

func TestParseTokenRejectsExpiredToken(t *testing.T) {
	withTestJWTSecret(t)

	token := signTestJWT(t, jwtPayload{
		UID: 7,
		UN:  "expired-user",
		Iat: time.Now().Add(-2 * time.Hour).Unix(),
		Exp: time.Now().Add(-1 * time.Hour).Unix(),
	})

	if _, _, ok := parseToken(token); ok {
		t.Fatal("parseToken() accepted expired token")
	}
}

func TestParseTokenRejectsShortSignature(t *testing.T) {
	withTestJWTSecret(t)

	if _, _, ok := parseToken("a.b.c"); ok {
		t.Fatal("parseToken() accepted malformed token")
	}
}
