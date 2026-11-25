package codieout

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Device is a codie device on a network this computer is on.
type Device struct {
	Addr string // where it answers, host:port
}

// How a sweep is kept to a few seconds.
const (
	// scanConcurrency is how many connections are tried at once.
	scanConcurrency = 256
	// connectTimeout is how long a host that says nothing is waited for.
	connectTimeout = 700 * time.Millisecond
	// scanPrefix is the widest network swept whole, 1,022 addresses. A wider
	// one, an office's or a virtual machine's, is cut down to the block of
	// that size around this computer, where its devices are likeliest.
	scanPrefix = 22
)

// Scan finds the codie devices on the networks this computer is on. Each one
// is swept for something listening on the codie port, and whatever answers
// there is asked to say it's a codie device. It takes a few seconds at most,
// and answers an error only when there's no network to look on.
func Scan(ctx context.Context) ([]Device, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var hosts []string
	seen := map[string]bool{}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, h := range lanHosts(addrs) {
			if !seen[h] {
				seen[h] = true
				hosts = append(hosts, h)
			}
		}
	}
	if len(hosts) == 0 {
		// Said in English, as the app translates what it shows when it draws it.
		return nil, errors.New("This computer isn't on a network to look on")
	}
	return find(ctx, hosts, DefaultPort), nil
}

// lanHosts is every address to try on the networks of addrs, an interface's:
// IPv4 only, without this computer's own addresses.
func lanHosts(addrs []net.Addr) []string {
	var hosts []string
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := n.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		ones, bits := n.Mask.Size()
		if bits != 32 {
			continue
		}
		// A network wider than scanPrefix is cut down to the block of that size
		// holding this computer's address.
		ones = max(ones, scanPrefix)
		base := binary.BigEndian.Uint32(ip.Mask(net.CIDRMask(ones, 32)))
		for i := uint32(1); i < 1<<(32-ones)-1; i++ {
			if base+i == binary.BigEndian.Uint32(ip) {
				continue
			}
			h := make(net.IP, 4)
			binary.BigEndian.PutUint32(h, base+i)
			hosts = append(hosts, h.String())
		}
	}
	return hosts
}

// find is the codie devices among hosts, by address.
func find(ctx context.Context, hosts []string, port string) []Device {
	var found []Device
	for _, h := range openPorts(ctx, hosts, port) {
		addr := net.JoinHostPort(h, port)
		if isCodie(ctx, addr) {
			found = append(found, Device{Addr: addr})
		}
	}
	sort.Slice(found, func(i, j int) bool { return lessIP(found[i].Addr, found[j].Addr) })
	return found
}

// openPorts is the hosts with something listening on port, by address.
func openPorts(ctx context.Context, hosts []string, port string) []string {
	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		open []string
		in   = make(chan string)
	)
	for range min(scanConcurrency, len(hosts)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := net.Dialer{Timeout: connectTimeout}
			for h := range in {
				conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(h, port))
				if err != nil {
					continue
				}
				conn.Close()
				mu.Lock()
				open = append(open, h)
				mu.Unlock()
			}
		}()
	}
	for _, h := range hosts {
		in <- h
	}
	close(in)
	wg.Wait()
	sort.Slice(open, func(i, j int) bool { return lessIP(open[i], open[j]) })
	return open
}

// isCodie asks what listens at addr to say it's a codie device: its ping
// answers pong. A device whose firmware is from before the ping answers its
// bootstrap instead.
func isCodie(ctx context.Context, addr string) bool {
	c := &http.Client{Timeout: 2 * time.Second}
	if data, ok := publicData(ctx, c, addr, "/v1/api/auth/ping"); ok {
		var said string
		return json.Unmarshal(data, &said) == nil && said == "pong"
	}
	data, ok := publicData(ctx, c, addr, "/v1/api/auth/bootstrap")
	if !ok {
		return false
	}
	var b map[string]json.RawMessage
	if json.Unmarshal(data, &b) != nil {
		return false
	}
	_, user := b["user_id"]
	_, loggedIn := b["device_logged_in"]
	return user || loggedIn
}

// publicData is what a device's public endpoint put in data, when it answered
// with success.
func publicData(ctx context.Context, c *http.Client, addr, path string) (json.RawMessage, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL(addr)+path, nil)
	if err != nil {
		return nil, false
	}
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, false
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, false
	}
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &env) != nil || !env.Success || len(env.Data) == 0 {
		return nil, false
	}
	return env.Data, true
}

// lessIP orders two addresses by the number of their IP, so a list reads in
// the order of the network.
func lessIP(a, b string) bool {
	ah, ap, _ := net.SplitHostPort(a)
	bh, bp, _ := net.SplitHostPort(b)
	ai, bi := net.ParseIP(ah), net.ParseIP(bh)
	if len(ai) != len(bi) {
		return len(ai) < len(bi)
	}
	for i := range ai {
		if ai[i] != bi[i] {
			return ai[i] < bi[i]
		}
	}
	return ap < bp
}
