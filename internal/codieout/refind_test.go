package codieout

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// addrOf strips the http:// a test server's URL carries, which is what a
// device's address looks like.
func addrOf(url string) string { return strings.TrimPrefix(url, "http://") }

func TestFindPairedFollowsTheSecret(t *testing.T) {
	// A device that reconnects often comes back at another address, and the
	// pair is its secret, not the address: the device that takes the
	// credential is ours, wherever it answered from.
	stranger := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"success":false,"code":"INVALID_SIGNATURE","message":"not ours"}`)
	}))
	defer stranger.Close()
	ours := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"success":true,"data":{"is_loaded":true,"root_path":"/storage"}}`)
	}))
	defer ours.Close()

	devices := []Device{{Addr: addrOf(stranger.URL)}, {Addr: addrOf(ours.URL)}}
	got, ok := findPaired(context.Background(), &Creds{Addr: "10.0.0.9:8081", Secret: "s"}, devices)
	if !ok || got != addrOf(ours.URL) {
		t.Fatalf("findPaired = %q, %v; want %q", got, ok, addrOf(ours.URL))
	}
}

func TestFindPairedFindsNone(t *testing.T) {
	// Every device turning the credential down means ours isn't there, and
	// the old address is kept rather than pointed at a stranger.
	stranger := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"success":false,"code":"INVALID_SIGNATURE","message":"not ours"}`)
	}))
	defer stranger.Close()
	if got, ok := findPaired(context.Background(), &Creds{Addr: "10.0.0.9:8081", Secret: "s"}, []Device{{Addr: addrOf(stranger.URL)}}); ok || got != "" {
		t.Fatalf("findPaired = %q, %v; want none", got, ok)
	}
}
