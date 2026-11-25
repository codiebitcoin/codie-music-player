package proxy

import (
	"os"
	"testing"
)

// A proxy is read as typed, from a bare address or a URL, and anything that
// isn't an HTTP proxy is none: a scheme ffmpeg can't follow, or nothing.
func TestNormalize(t *testing.T) {
	for _, c := range []struct{ typed, want string }{
		{"", ""},
		{"   ", ""},
		{"127.0.0.1:7897", "http://127.0.0.1:7897"},
		{" http://127.0.0.1:7897 ", "http://127.0.0.1:7897"},
		{"https://proxy.example:8443", "https://proxy.example:8443"},
		{"socks5://127.0.0.1:1080", ""},
		{"nonsense", ""},
		{"http://127.0.0.1", ""},
		{"http://", ""},
		{"http://127.0.0.1:7897/", "http://127.0.0.1:7897"},
		{"http://127.0.0.1:7897/path?x=1", "http://127.0.0.1:7897"},
	} {
		if got := Normalize(c.typed); got != c.want {
			t.Errorf("%q: %q, want %q", c.typed, got, c.want)
		}
	}
}

// Applying a proxy sets what the programs read, and local addresses are kept
// out of it; applying none takes them away again.
func TestApply(t *testing.T) {
	defer Apply("")
	Apply("127.0.0.1:7897")
	for _, k := range []string{"http_proxy", "HTTP_PROXY", "https_proxy", "HTTPS_PROXY", "all_proxy", "ALL_PROXY"} {
		if got := os.Getenv(k); got != "http://127.0.0.1:7897" {
			t.Errorf("%s = %q", k, got)
		}
	}
	for _, k := range []string{"no_proxy", "NO_PROXY"} {
		if got := os.Getenv(k); got != NoProxy {
			t.Errorf("%s = %q, want %q", k, got, NoProxy)
		}
	}
	Apply("")
	for _, k := range []string{"http_proxy", "https_proxy", "all_proxy"} {
		if got := os.Getenv(k); got != "" {
			t.Errorf("%s is still %q after none", k, got)
		}
	}
}

// The environment the app was started with is the app's own: taking the
// listener's proxy away leaves it as it was.
func TestApplyKeepsTheStartsEnvironment(t *testing.T) {
	t.Setenv("http_proxy", "http://from-the-shell:3128")
	Apply("")
	if got := os.Getenv("http_proxy"); got != "http://from-the-shell:3128" {
		t.Errorf("http_proxy = %q", got)
	}
	Apply("127.0.0.1:7897")
	if got := os.Getenv("http_proxy"); got != "http://127.0.0.1:7897" {
		t.Errorf("http_proxy = %q", got)
	}
}
