package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/codieout"
)

// fakeCodieAskers swaps how the app looks for devices and asks one to pair —
// neither reaches a network — telling asked each address, and puts the real
// ones back when the test ends.
func fakeCodieAskers(t *testing.T, found []codieout.Device, asked chan<- string) {
	t.Helper()
	oldScan, oldPair := scanCodieDevices, requestPairing
	scanCodieDevices = func() ([]codieout.Device, error) { return found, nil }
	requestPairing = func(addr string) (string, error) {
		asked <- addr
		return "challenge-1", nil
	}
	t.Cleanup(func() { scanCodieDevices, requestPairing = oldScan, oldPair })
}

// Scanning lists the codie devices found on the network, and choosing one
// starts pairing it, so its address never has to be typed.
func TestCodieScanChoosesDeviceToPair(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	asked := make(chan string, 4)
	fakeCodieAskers(t, []codieout.Device{{Addr: "192.168.1.23:8081"}, {Addr: "192.168.1.24:8081"}}, asked)

	a.settingsPage = "codie"
	settings := ui.NewTester(a.settingsView, 760, 1400)
	settings.Frame()
	if err := settings.Click("Scan for devices"); err != nil {
		t.Fatal(err)
	}
	if !a.codiePairing.scanning {
		t.Fatal("it isn't scanning")
	}
	until(t, ch, func() bool { return !a.codiePairing.scanning })
	settings.Frame()
	for _, want := range []string{"Codie device · 192.168.1.23:8081", "Codie device · 192.168.1.24:8081"} {
		if _, ok := settings.Find(want); !ok {
			t.Errorf("%q isn't offered: %q", want, settings.Texts())
		}
	}

	if err := settings.Click("Codie device · 192.168.1.24:8081"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, asked, "the device to pair"); got != "192.168.1.24:8081" {
		t.Errorf("asked to pair %s", got)
	}
	runPost(t, ch)
	if s := a.codiePairing; s.challenge != "challenge-1" || s.addr != "192.168.1.24:8081" || s.busy {
		t.Errorf("pairing state afterwards: %+v", s)
	}
}
