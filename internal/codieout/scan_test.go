package codieout

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// codieDevice answers like a codie device: with ping, its public ping; without,
// a firmware from before the ping, which only has its bootstrap.
func codieDevice(t *testing.T, ping bool) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/api/auth/ping":
			if !ping {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte(`{"success":true,"data":"pong"}`))
		case "/v1/api/auth/bootstrap":
			w.Write([]byte(`{"success":true,"data":{"language":"zh-CN","device_logged_in":true,"user_id":"u1"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

// portOf is the port an address is on.
func portOf(a net.Addr) string {
	_, p, err := net.SplitHostPort(a.String())
	if err != nil {
		return ""
	}
	return p
}

// A codie device on the network is found, and something else with the same
// port open isn't, whether it's another web app, says nothing, or isn't there.
func TestScanFindsOnlyCodieDevices(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	hosts := []string{"127.0.0.1"}

	device := codieDevice(t, true)
	got := find(ctx, hosts, portOf(device.Listener.Addr()))
	if len(got) != 1 || got[0].Addr != device.Listener.Addr().String() {
		t.Errorf("the codie device: %+v", got)
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no such page", http.StatusNotFound)
	}))
	defer other.Close()
	if got := find(ctx, hosts, portOf(other.Listener.Addr())); len(got) != 0 {
		t.Errorf("another app on the port: %+v", got)
	}

	silent, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	go func() {
		for {
			conn, err := silent.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	if got := find(ctx, hosts, portOf(silent.Addr())); len(got) != 0 {
		t.Errorf("a port that only listens: %+v", got)
	}

	shut, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	shutPort := portOf(shut.Addr())
	shut.Close()
	if got := find(ctx, hosts, shutPort); len(got) != 0 {
		t.Errorf("a port nothing listens on: %+v", got)
	}
}

// A device whose firmware is from before the ping is found by its bootstrap.
func TestScanFindsDeviceWithoutPing(t *testing.T) {
	old := codieDevice(t, false)
	if got := find(context.Background(), []string{"127.0.0.1"}, portOf(old.Listener.Addr())); len(got) != 1 {
		t.Errorf("the device: %+v", got)
	}
}

// Addresses read in the order of their numbers: 2 before 10, whichever is
// asked first.
func TestLessIP(t *testing.T) {
	sorted := []string{"10.0.0.2:8081", "192.168.1.2:8081", "192.168.1.10:8081"}
	for i, a := range sorted {
		for _, b := range sorted[i+1:] {
			if !lessIP(a, b) || lessIP(b, a) {
				t.Errorf("%s and %s aren't in that order", a, b)
			}
		}
	}
}

// interfaceAddr is what an interface's address comes as: this computer's own
// address, with the network's mask.
func interfaceAddr(ip string, prefix int) net.Addr {
	return &net.IPNet{IP: net.ParseIP(ip).To4(), Mask: net.CIDRMask(prefix, 32)}
}

// A home network is swept whole, without its two ends and this computer's own
// address; a wider one is cut down to the block around this computer; and
// loopback and link-local addresses are left out.
func TestLanHosts(t *testing.T) {
	home := lanHosts([]net.Addr{interfaceAddr("192.168.1.20", 24)})
	if len(home) != 253 || home[0] != "192.168.1.1" || home[len(home)-1] != "192.168.1.254" {
		t.Errorf("a /24: %d addresses, %s…%s", len(home), home[0], home[len(home)-1])
	}
	for _, h := range home {
		if h == "192.168.1.20" || h == "192.168.1.0" || h == "192.168.1.255" {
			t.Errorf("%s is tried", h)
		}
	}

	wide := lanHosts([]net.Addr{interfaceAddr("10.1.2.3", 16)})
	if len(wide) != 1021 || wide[0] != "10.1.0.1" || wide[len(wide)-1] != "10.1.3.254" {
		t.Errorf("a /16 around 10.1.2.3: %d addresses, %s…%s", len(wide), wide[0], wide[len(wide)-1])
	}

	if got := lanHosts([]net.Addr{interfaceAddr("127.0.0.1", 8), interfaceAddr("169.254.7.7", 16)}); len(got) != 0 {
		t.Errorf("loopback and link-local: %d addresses", len(got))
	}
}
