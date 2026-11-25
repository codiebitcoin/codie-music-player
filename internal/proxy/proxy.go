// Package proxy makes the HTTP proxy the listener sets reach what plays:
// ffmpeg and yt-dlp read it from the environment, as every Go HTTP client in
// the app does, so a source behind a proxy can play without a system-wide one.
package proxy

import (
	"net/url"
	"os"
	"strings"
)

// NoProxy is what never goes through the proxy: the app's own pages and the
// local streams its plugins serve are there, and a proxy would only break them.
const NoProxy = "localhost,127.0.0.1,::1"

// keys are the variables that carry a proxy, in both cases: ffmpeg and yt-dlp
// read the lower ones, and the rest of the world the upper ones.
var vars = []string{"http_proxy", "HTTP_PROXY", "https_proxy", "HTTPS_PROXY", "all_proxy", "ALL_PROXY"}
var bypass = []string{"no_proxy", "NO_PROXY"}

// Normalize reads a proxy as one might type it, such as 127.0.0.1:7897 or
// http://127.0.0.1:7897, as an HTTP proxy's address. "" is none, and so is
// whatever isn't one: a scheme ffmpeg can't use, or no address at all.
func Normalize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || u.Port() == "" {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	// A proxy is an address: what came after it, a path or a query, isn't one.
	u.Path, u.RawPath, u.RawQuery, u.Fragment = "", "", "", ""
	return u.String()
}

// applied is what Apply set, for the next one to unset, leaving the
// environment the app was started with alone.
var applied []string

// Apply sends the app, and the programs it starts from now on, through the
// proxy at s, which it reads as Normalize does. Empty takes the proxy away.
func Apply(s string) {
	for _, k := range applied {
		os.Unsetenv(k)
	}
	applied = nil
	s = Normalize(s)
	if s == "" {
		return
	}
	for _, k := range vars {
		os.Setenv(k, s)
		applied = append(applied, k)
	}
	for _, k := range bypass {
		os.Setenv(k, NoProxy)
		applied = append(applied, k)
	}
}
