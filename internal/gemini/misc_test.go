package gemini

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestParseProcessIDs(t *testing.T) {
	out := "  1 /sbin/launchd\n 10 /Applications/Antigravity.app/Contents/MacOS/Antigravity\n" +
		" 11 /Applications/Antigravity.app/Contents/Frameworks/Antigravity Helper (Renderer).app/Contents/MacOS/Antigravity Helper (Renderer)\n" +
		" 12 /Users/x/.local/bin/agy\n 13 antigravity\n 14 /usr/bin/agyx\n 99 /Users/x/.local/bin/agy\n"
	if got := parseProcessIDs(out, 99); !slices.Equal(got, []int{10, 12, 13}) {
		t.Fatalf("pids = %v", got)
	}
	if !isAntigravityExecutable(`C:\Users\x\AppData\Local\Programs\Antigravity\Antigravity.exe`, true) || !isAntigravityExecutable("agy.exe", true) {
		t.Fatal("windows executables")
	}
	if _, ok := windowsProcess(5, "agy.exe", 5); ok {
		t.Fatal("self excluded")
	}
	if p, ok := windowsProcess(6, `C:\Programs\Antigravity\Antigravity.exe`, 5); !ok || p.client != SessionClientApp {
		t.Fatalf("windows app = %+v, %v", p, ok)
	}
	clients := map[int]string{}
	for _, p := range parseProcesses(out, 99) {
		clients[p.pid] = p.client
	}
	if clients[10] != SessionClientApp || clients[12] != SessionClientCLI || clients[13] != SessionClientApp {
		t.Fatalf("clients = %v", clients)
	}
}

func TestDetectClientVersion(t *testing.T) {
	dir := t.TempDir()
	plist := filepath.Join(dir, "Info.plist")
	writeFile(t, plist, []byte("<dict>\n\t<key>CFBundleShortVersionString</key>\n\t<string>2.31.2</string>\n</dict>"))
	if got := DetectClientVersion([]string{filepath.Join(dir, "missing"), plist}); got != "2.31.2" {
		t.Fatalf("plist = %s", got)
	}
	product := filepath.Join(dir, "product.json")
	writeFile(t, product, []byte(`{"version":"2.1.0"}`))
	if got := DetectClientVersion([]string{product}); got != MinimumClientVersion {
		t.Fatalf("old version must floor to the minimum, got %s", got)
	}
	if got := DetectClientVersion(nil); got != MinimumClientVersion {
		t.Fatalf("unknown = %s", got)
	}
	if compareVersions("2.25.10", "2.25.9") <= 0 || compareVersions("x", "2.25.0") >= 0 {
		t.Fatal("compareVersions")
	}
	if clientPlatform("windows", "amd64") != "WINDOWS_AMD64" || clientPlatform("plan9", "386") != "PLATFORM_UNSPECIFIED" {
		t.Fatal("platform names")
	}
}

func TestQuotaThrottleBackoff(t *testing.T) {
	var th QuotaThrottle
	if th.Wait("a", testNow) != 0 {
		t.Fatal("fresh account waits")
	}
	if w := th.ObserveRateLimit("a", testNow, &RateLimitedError{}); w != RateLimitBackoffStart {
		t.Fatalf("first wait %v", w)
	}
	if w := th.ObserveRateLimit("a", testNow, &RateLimitedError{}); w != 2*RateLimitBackoffStart {
		t.Fatalf("second wait %v", w)
	}
	for range 10 {
		th.ObserveRateLimit("a", testNow, &RateLimitedError{})
	}
	if w := th.Wait("a", testNow); w != RateLimitBackoffMax {
		t.Fatalf("capped wait %v", w)
	}
	th.Succeeded("a")
	if th.ObserveRateLimit("a", testNow, &RateLimitedError{RetryAfter: time.Second, HasRetryAfter: true}) != time.Second {
		t.Fatal("Retry-After wins")
	}
	if th.Wait("b", testNow) != 0 {
		t.Fatal("other accounts are not held back")
	}
}

func TestParsePayloadsShapes(t *testing.T) {
	official := `{"token":{"access_token":"a1","refresh_token":"r1","expiry":"2026-11-13T11:00:00Z"},"auth_method":"consumer"}`
	bare := `{"access_token":"a2","refresh_token":"r2","expiry_date":1794567890000,"id_token":"x.y.z"}`
	text := "here you go:\n[" + official + "," + bare + "," + official + "]"
	got := ParsePayloads(text)
	if len(got) != 2 {
		t.Fatalf("payloads = %d", len(got))
	}
	second := mustCred(t, got[1])
	if second.RefreshToken != "r2" || second.IDToken != "x.y.z" || second.Expiry.IsZero() || second.AuthMethod != "consumer" {
		t.Fatalf("bare object = %+v", second)
	}
	if n := len(ParsePayloads(official + "\n" + bare)); n != 2 {
		t.Fatalf("ndjson = %d", n)
	}
	if ParsePayloads(`{"hello":"world"}`) != nil {
		t.Fatal("no credentials")
	}
}

func TestErrorCodes(t *testing.T) {
	cases := map[error]string{
		ErrNotInstalled:                            ErrorCodeNotInstalled,
		ErrOfficialCredentialMissing:               ErrorCodeNotSignedIn,
		ErrAuthenticationRequired:                  ErrorCodeAuthenticationNeeded,
		&RateLimitedError{}:                        ErrorCodeRateLimited,
		&TimeoutError{Operation: "x"}:              ErrorCodeTimeout,
		ErrQuotaParse:                              ErrorCodeQuotaParseFailed,
		context.Canceled:                           ErrorCodeCancelled,
		&StatusError{Method: "x", StatusCode: 500}: ErrorCodeRefreshFailed,
	}
	for err, want := range cases {
		if got := ErrorCode(err); got != want {
			t.Errorf("%v = %s, want %s", err, got, want)
		}
	}
}
