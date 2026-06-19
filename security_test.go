package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ---- isRateLimited / recordAttempt / clearAttempts ----

func TestIsRateLimitedReturnsFalseWhenNoEntry(t *testing.T) {
	mu := &sync.Mutex{}
	attempts := map[string]*loginEntry{}
	if isRateLimited(mu, attempts, "1.2.3.4", 5) {
		t.Fatal("expected not rate-limited for unknown IP")
	}
}

func TestIsRateLimitedReturnsTrueAfterLimitExceeded(t *testing.T) {
	mu := &sync.Mutex{}
	attempts := map[string]*loginEntry{}
	ip := "10.0.0.1"
	window := 10 * time.Minute
	for i := 0; i < 5; i++ {
		recordAttempt(mu, attempts, ip, window)
	}
	if !isRateLimited(mu, attempts, ip, 5) {
		t.Fatal("expected rate-limited after 5 attempts")
	}
}

func TestIsRateLimitedReturnsFalseBeforeLimitReached(t *testing.T) {
	mu := &sync.Mutex{}
	attempts := map[string]*loginEntry{}
	ip := "10.0.0.2"
	window := 10 * time.Minute
	for i := 0; i < 4; i++ {
		recordAttempt(mu, attempts, ip, window)
	}
	if isRateLimited(mu, attempts, ip, 5) {
		t.Fatal("expected not rate-limited before 5 attempts")
	}
}

func TestIsRateLimitedReturnsFalseAfterWindowExpires(t *testing.T) {
	mu := &sync.Mutex{}
	attempts := map[string]*loginEntry{}
	ip := "10.0.0.3"
	window := 1 * time.Millisecond
	for i := 0; i < 10; i++ {
		recordAttempt(mu, attempts, ip, window)
	}
	time.Sleep(5 * time.Millisecond)
	if isRateLimited(mu, attempts, ip, 5) {
		t.Fatal("expected not rate-limited after window expired")
	}
}

func TestClearAttemptsResetsCounter(t *testing.T) {
	mu := &sync.Mutex{}
	attempts := map[string]*loginEntry{}
	ip := "10.0.0.4"
	window := 10 * time.Minute
	for i := 0; i < 5; i++ {
		recordAttempt(mu, attempts, ip, window)
	}
	if !isRateLimited(mu, attempts, ip, 5) {
		t.Fatal("expected rate-limited before clear")
	}
	clearAttempts(mu, attempts, ip)
	if isRateLimited(mu, attempts, ip, 5) {
		t.Fatal("expected not rate-limited after clearAttempts")
	}
}

func TestRecordAttemptResetsCounterWhenWindowExpired(t *testing.T) {
	mu := &sync.Mutex{}
	attempts := map[string]*loginEntry{}
	ip := "10.0.0.5"
	// Saturate with expired window
	for i := 0; i < 5; i++ {
		recordAttempt(mu, attempts, ip, 1*time.Millisecond)
	}
	time.Sleep(5 * time.Millisecond)
	// One more attempt should start a fresh window (count = 1)
	recordAttempt(mu, attempts, ip, 10*time.Minute)
	if isRateLimited(mu, attempts, ip, 5) {
		t.Fatal("expected count reset to 1 after window expiry, not rate-limited yet")
	}
}

func TestIsRateLimitedConcurrentSafety(t *testing.T) {
	mu := &sync.Mutex{}
	attempts := map[string]*loginEntry{}
	ip := "10.0.0.6"
	window := 10 * time.Minute
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recordAttempt(mu, attempts, ip, window)
			isRateLimited(mu, attempts, ip, 5)
		}()
	}
	wg.Wait()
}

// ---- clientIP / X-Forwarded-For ----

func makeReq(remoteAddr, xff string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = remoteAddr
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	return r
}

func TestClientIPReturnsDirectIPForPublicPeer(t *testing.T) {
	// Public peer — XFF must be IGNORED (spoofing prevention)
	r := makeReq("203.0.113.1:12345", "1.1.1.1")
	got := clientIP(r)
	if got != "203.0.113.1" {
		t.Fatalf("expected direct IP 203.0.113.1 for public peer, got %s", got)
	}
}

func TestClientIPUsesXFFForLoopbackPeer(t *testing.T) {
	r := makeReq("127.0.0.1:9000", "203.0.113.99")
	got := clientIP(r)
	if got != "203.0.113.99" {
		t.Fatalf("expected XFF IP for loopback peer, got %s", got)
	}
}

func TestClientIPUsesXFFForPrivatePeer(t *testing.T) {
	r := makeReq("192.168.1.50:9000", "198.51.100.7")
	got := clientIP(r)
	if got != "198.51.100.7" {
		t.Fatalf("expected XFF IP for private peer, got %s", got)
	}
}

func TestClientIPIgnoresMalformedXFF(t *testing.T) {
	r := makeReq("127.0.0.1:9000", "not-an-ip")
	got := clientIP(r)
	// Falls back to loopback when XFF is invalid
	if got != "127.0.0.1" {
		t.Fatalf("expected loopback fallback for malformed XFF, got %s", got)
	}
}

func TestClientIPFirstXFFEntryUsed(t *testing.T) {
	r := makeReq("127.0.0.1:9000", "10.10.10.1, 172.16.0.1, 8.8.8.8")
	got := clientIP(r)
	if got != "10.10.10.1" {
		t.Fatalf("expected first XFF entry 10.10.10.1, got %s", got)
	}
}

func TestClientIPNoPortInRemoteAddr(t *testing.T) {
	r := makeReq("203.0.113.5", "")
	// Should not panic
	got := clientIP(r)
	if got == "" {
		t.Fatal("expected non-empty IP for RemoteAddr without port")
	}
}

// ---- validUsername ----

func TestValidUsernameAcceptsAlphanumericAndDashes(t *testing.T) {
	cases := []string{"alice", "bob_123", "user-name", "ABC", "a1b2c3"}
	for _, c := range cases {
		if !validUsername(c) {
			t.Errorf("expected %q to be valid", c)
		}
	}
}

func TestValidUsernameRejectsTooShort(t *testing.T) {
	if validUsername("ab") {
		t.Error("expected 2-char username to be invalid (min 3)")
	}
}

func TestValidUsernameRejectsTooLong(t *testing.T) {
	long := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 33 chars
	if validUsername(long) {
		t.Errorf("expected 33-char username to be invalid (max 32)")
	}
}

func TestValidUsernameAcceptsBoundaryLengths(t *testing.T) {
	min3 := "abc"
	max32 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 32 chars
	if !validUsername(min3) {
		t.Error("expected 3-char username to be valid")
	}
	if !validUsername(max32) {
		t.Errorf("expected 32-char username to be valid, got invalid for len=%d", len(max32))
	}
}

func TestValidUsernameRejectsSpecialChars(t *testing.T) {
	cases := []string{"user@host", "user name", "user!", "user+1", "日本語"}
	for _, c := range cases {
		if validUsername(c) {
			t.Errorf("expected %q to be invalid", c)
		}
	}
}

// ---- isAllowedCDNHost ----

func TestIsAllowedCDNHostAllowsKnownDomains(t *testing.T) {
	cases := []string{
		"v123.xhcdn.com",
		"heavyfetish.com",
		"www.punishbang.com",
		"acdn.sunporno.com",
	}
	for _, c := range cases {
		if !isAllowedCDNHost(c) {
			t.Errorf("expected %q to be allowed", c)
		}
	}
}

func TestIsAllowedCDNHostBlocksArbitraryDomains(t *testing.T) {
	cases := []string{"evil.com", "attacker.io", "google.com", "localhost"}
	for _, c := range cases {
		if isAllowedCDNHost(c) {
			t.Errorf("expected %q to be blocked", c)
		}
	}
}

func TestIsAllowedCDNHostBlocksSubdomainBypassAttempt(t *testing.T) {
	// e.g. "xhcdn.com.evil.com" — should be blocked since it doesn't end with ".xhcdn.com"
	if isAllowedCDNHost("xhcdn.com.evil.com") {
		t.Error("expected subdomain bypass attempt to be blocked")
	}
}

func TestIsAllowedCDNHostBlocksEmptyString(t *testing.T) {
	if isAllowedCDNHost("") {
		t.Error("expected empty host to be blocked")
	}
}

// ---- slugToTitle ----

func TestSlugToTitleConvertsHyphens(t *testing.T) {
	if got := slugToTitle("big-ass-teen"); got != "Big Ass Teen" {
		t.Errorf("got %q", got)
	}
}

func TestSlugToTitleConvertsUnderscores(t *testing.T) {
	if got := slugToTitle("hot_girl_next_door"); got != "Hot Girl Next Door" {
		t.Errorf("got %q", got)
	}
}

func TestSlugToTitleHandlesEmptyString(t *testing.T) {
	if got := slugToTitle(""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestSlugToTitleHandlesSingleWord(t *testing.T) {
	if got := slugToTitle("teen"); got != "Teen" {
		t.Errorf("got %q", got)
	}
}

// ---- parseDuration ----

func TestParseDurationFullISO(t *testing.T) {
	if got := parseDuration("PT00H11M15S"); got != 675 {
		t.Errorf("expected 675, got %d", got)
	}
}

func TestParseDurationHoursOnly(t *testing.T) {
	if got := parseDuration("PT02H"); got != 7200 {
		t.Errorf("expected 7200, got %d", got)
	}
}

func TestParseDurationSecondsOnly(t *testing.T) {
	if got := parseDuration("PT45S"); got != 45 {
		t.Errorf("expected 45, got %d", got)
	}
}

func TestParseDurationZero(t *testing.T) {
	if got := parseDuration("PT0S"); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestParseDurationEmptyString(t *testing.T) {
	if got := parseDuration(""); got != 0 {
		t.Errorf("expected 0 for empty, got %d", got)
	}
}

func TestParseDurationGarbage(t *testing.T) {
	if got := parseDuration("GARBAGE"); got != 0 {
		t.Errorf("expected 0 for garbage, got %d", got)
	}
}

func TestParseDurationHoursAndMinutes(t *testing.T) {
	if got := parseDuration("PT1H30M"); got != 5400 {
		t.Errorf("expected 5400, got %d", got)
	}
}
