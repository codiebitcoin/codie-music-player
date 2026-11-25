package app

import (
	"github.com/egoist/mygo/ui"
)

// codieScanView is the way to pair without typing an address: a button that
// sweeps the network for codie devices, beside what the sweep says, and one
// row per device found, which starts pairing it.
func (a *app) codieScanView(c *ui.Context, p palette) {
	s := &a.codiePairing
	label := T("Scan for devices")
	if s.scanning {
		label = T("Scanning…")
	} else if s.scanned {
		label = T("Scan again")
	}
	said := ""
	switch {
	case s.scanning:
		said = T("Looking for codie devices on this network…")
	case s.scanErr != "":
		said = T(s.scanErr)
	case s.scanned && len(s.found) == 0:
		said = T("No codie device found on this network. Check it's on the same WiFi as this computer.")
	case len(s.found) > 0:
		said = T("Found on this network. Choose one: its screen shows the code to enter.")
	default:
		said = T("Looks for codie devices on this computer's network")
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		if ui.Button(c.Key("codie-scan"), label).Disabled(s.scanning || s.busy).Clicked() {
			a.codieScan()
		}
		ui.Text(c, said).FontSize(12).TextColor(p.muted).LineHeight(1.4).Grow(1).Shrink(1).MinWidth(0)
	})
	for _, d := range s.found {
		paired := a.settings.Codie != nil && a.settings.Codie.Addr == d.Addr
		name := T("Codie device") + " · " + d.Addr
		if paired {
			name += " · " + T("Paired")
		}
		b := ui.ButtonBase(c.Key(d.Addr)).Fill().Height(34).PaddingX(10).Gap(10).Radius(8).AlignItems(ui.Center).
			FocusRing(false).Label(name).Disabled(s.busy)
		if b.Hovered() {
			b.Background(p.hover)
		}
		b.Children(func() {
			ui.Text(c, T("Codie device")).FontSize(13).FontWeight(500)
			ui.Text(c, d.Addr).FontSize(12).TextColor(p.muted).Grow(1)
			if paired {
				ui.Text(c, T("Paired")).FontSize(12).TextColor(p.accent)
			}
		})
		if b.Clicked() {
			a.codiePick(d)
		}
	}
}
