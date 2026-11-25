package codieout

import "context"

// FindPaired looks for the device a paired credential belongs to on the
// networks this computer is on. A device's address can move — DHCP hands it a
// new one when it reconnects, and after a restart it often does — while the
// pair itself is the secret, not the address. Every device found is asked for
// its music storage with the pair's own credential, and the one that takes it
// is the paired device.
//
// Only a device whose music page is open answers a signed ask, so a device
// with its music closed isn't found: it couldn't be played to anyway.
func FindPaired(ctx context.Context, creds *Creds) (string, bool) {
	if creds == nil || creds.Secret == "" {
		return "", false
	}
	devices, err := Scan(ctx)
	if err != nil {
		return "", false
	}
	return findPaired(ctx, creds, devices)
}

// findPaired is FindPaired over devices already found.
func findPaired(ctx context.Context, creds *Creds, devices []Device) (string, bool) {
	for _, d := range devices {
		if ctx.Err() != nil {
			return "", false
		}
		c := *creds
		c.Addr = d.Addr
		if _, err := New(&c).MusicStorage(); err == nil {
			return d.Addr, true
		}
	}
	return "", false
}
