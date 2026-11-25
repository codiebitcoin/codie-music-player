package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygo-music/internal/codieout"
	"mygo-music/internal/music"
)

// codiePairState is where the codie settings section is: what's being paired.
type codiePairState struct {
	addr      string // the address being paired
	code      string // the pairing code being typed
	challenge string // the pairing challenge, once the device shows its code
	busy      bool   // a pair step is on its way
	status    string // the last thing that happened, in English, for the row to say

	scanning bool              // the network is being swept for devices
	scanned  bool              // a sweep has ended, so that finding none can be said
	found    []codieout.Device // what the last sweep found
	scanErr  string            // why the last sweep couldn't look, in English
}

// requestPairing asks the device at addr to show its code; tests replace it.
var requestPairing = func(addr string) (string, error) {
	return codieout.New(&codieout.Creds{Addr: addr}).RequestPairing()
}

// scanCodieDevices looks for codie devices on the network; tests replace it.
var scanCodieDevices = func() ([]codieout.Device, error) {
	return codieout.Scan(context.Background())
}

// codiePoll is how often the paired device is asked how it is, for the
// sidebar's row: the device answers in a moment on the same network.
const codiePoll = 30 * time.Second

// codieRefindEvery is how often a paired device that stopped answering is
// looked for at a new address: a sweep of the network takes a few seconds,
// and an address that moved won't move again the same minute.
const codieRefindEvery = time.Minute

// codieProbeNow asks the paired device how it is, off the main thread, and
// keeps the answer for the sidebar's row. Main thread only.
func (a *app) codieProbeNow() {
	if a.settings.Codie == nil {
		return
	}
	c := a.codieClient()
	if c == nil {
		return
	}
	go func() {
		state := "offline"
		if st, err := c.MusicStorage(); err == nil {
			state = "no-card"
			if st.IsLoaded {
				state = "online"
			}
		} else {
			// An address that stops answering is often a device that
			// reconnected and was given another one; look for it again.
			a.post(a.codieRefind)
		}
		a.post(func() {
			a.codieState = state
		})
	}()
}

// codieRefind looks for the paired device at a new address, for one that
// stopped answering: a device that reconnects to the network often gets
// another address, and the pair is its secret, so it's found again without
// pairing anew. Main thread only.
func (a *app) codieRefind() {
	if a.codieRefinding || a.settings.Codie == nil {
		return
	}
	if !a.codieRefoundAt.IsZero() && time.Since(a.codieRefoundAt) < codieRefindEvery {
		return
	}
	creds := *a.settings.Codie
	a.codieRefinding, a.codieRefoundAt = true, time.Now()
	go func() {
		addr, ok := codieout.FindPaired(context.Background(), &creds)
		a.post(func() {
			a.codieRefinding = false
			if !ok || a.settings.Codie == nil || a.settings.Codie.Addr == addr {
				return
			}
			log.Printf("codie: found the paired device at %s", addr)
			a.settings.Codie.Addr = addr
			a.changed()
			a.codieProbeNow()
		})
	}()
}

// codieWatch keeps that row fresh: asked at first, then every codiePoll, for
// as long as the app runs, so a device paired later needs no start of its own.
func (a *app) codieWatch() {
	for {
		time.Sleep(codiePoll)
		mygo.RunOnMain(a.codieProbeNow)
	}
}

// codieRow says how the paired device is, at the sidebar's foot: a dot in its
// state's color and what it says, which opens the Codie settings.
func (a *app) codieRow(c *ui.Context) {
	if a.settings.Codie == nil {
		return
	}
	p := a.pal
	// Green for a device that answers, amber for one whose card isn't in,
	// faint for one that doesn't answer: the accent is the app's, and says
	// nothing of how a device is.
	dot, said := p.faint, T("Offline")
	switch a.codieState {
	case "online":
		dot, said = ui.Hex("#2fb457"), T("Online")
	case "no-card":
		dot, said = ui.Hex("#d99324"), T("No card")
	}
	b := ui.ButtonBase(c.Key("codie-state")).Height(34).PaddingX(8).Gap(8).Radius(8).AlignItems(ui.Center).
		Justify(ui.Start).FocusRing(false).Label(T("Codie device"))
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Children(func() {
		ui.Box(c).Size(7, 7).Radius(4).Background(dot)
		ui.Text(c, "Codie").FontSize(12.5).FontWeight(500).TextColor(p.text).Shrink(0)
		ui.Text(c, " · "+said).FontSize(12).TextColor(p.muted).SingleLine().Shrink(1).MinWidth(0)
	})
	if b.Clicked() {
		a.settingsPage = "codie"
		a.openSettings()
	}
}

// codieClient answers the paired device's client, or nil.
func (a *app) codieClient() *codieout.Client {
	if c := a.settings.Codie; c != nil && c.Secret != "" {
		return codieout.New(c)
	}
	return nil
}

// codieSink wraps a stream source to play on the codie device, when the
// settings ask for it; nil answers for anything that plays as it would.
func (a *app) codieSink(src music.Source) music.Source {
	if !a.settings.CodieOn || a.settings.Codie == nil {
		return nil
	}
	if src == nil || src.Info().Playback != music.PlaybackStream {
		return nil
	}
	dir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return nil
	}
	return codieout.Wrap(src, a.codieClient(), dir+"/codie", func() string { return a.settings.Quality })
}

// codiePair asks the device at addr to show its code, and waits for it.
func (a *app) codiePair(addr string) {
	addr = codieout.NormalizeAddr(addr) // a bare IP is enough; the port defaults
	a.codiePairing.busy, a.codiePairing.status = true, ""
	a.codiePairing.addr = addr
	a.redraw()
	go func() {
		challenge, err := requestPairing(addr)
		a.post(func() {
			a.codiePairing.busy = false
			if err != nil {
				a.codiePairing.status = err.Error()
				a.codiePairing.challenge = ""
			} else {
				a.codiePairing.challenge = challenge
				a.codiePairing.status = "Enter the 6-digit code shown on the device's screen"
			}
		})
	}()
}

// codieScan sweeps the network for codie devices, to pair one without knowing
// its address, and lists what it found under the pairing.
func (a *app) codieScan() {
	s := &a.codiePairing
	s.scanning, s.scanned, s.found, s.scanErr = true, false, nil, ""
	a.redraw()
	go func() {
		found, err := scanCodieDevices()
		a.post(func() {
			// Asked for again meanwhile, as the setup guide does: whatever
			// it asks now is what's shown.
			s := &a.codiePairing
			s.scanning, s.scanned = false, true
			if err != nil {
				s.found, s.scanErr = nil, err.Error()
				return
			}
			s.found, s.scanErr = found, ""
		})
	}()
}

// codiePick starts pairing a device the sweep found.
func (a *app) codiePick(d codieout.Device) {
	a.codiePair(d.Addr)
}

// codieConfirm trades the code the listener read on the device for a client
// credential, and remembers the device.
func (a *app) codieConfirm(code string) {
	challenge := a.codiePairing.challenge
	if challenge == "" {
		return
	}
	a.codiePairing.busy, a.codiePairing.status = true, "Confirming…"
	a.redraw()
	c := codieout.New(&codieout.Creds{Addr: a.codiePairing.addr})
	creds, err := c.ConfirmPairing(challenge, code)
	mygo.RunOnMain(func() {
		a.codiePairing.busy = false
		a.codiePairing.challenge = ""
		if err != nil {
			a.codiePairing.status = err.Error()
		} else {
			a.settings.Codie = creds
			a.settings.CodieOn = true
			a.codieStatus = "Paired, checking…"
			a.codiePairing.status = "Paired to " + creds.Addr
			a.codiePairing.found, a.codiePairing.scanned = nil, false // no longer choosing
			a.changed()
			go a.codieTest()
		}
		a.redraw()
	})
}

// codieClear deletes every song this app left on the card, off the main thread.
func (a *app) codieClear() {
	c := a.codieClient()
	if c == nil {
		return
	}
	a.codiePairing.busy, a.codiePairing.status = true, "Clearing device songs…"
	a.redraw()
	n, err := c.ClearCardUploads()
	mygo.RunOnMain(func() {
		a.codiePairing.busy = false
		if err != nil {
			a.codieStatus = fmt.Sprintf("Clearing stopped after %d deleted: %v", n, err)
		} else {
			a.codieStatus = fmt.Sprintf("Cleared %d device songs", n)
		}
		a.redraw()
	})
}

// codieClearToasted 清理设备歌曲并把结果以 toast 告知（供播放栏按钮）。
func (a *app) codieClearToasted() {
	c := a.codieClient()
	if c == nil {
		a.post(func() { a.toast = T("No codie device paired") })
		return
	}
	n, err := c.ClearCardUploads()
	a.post(func() {
		if err != nil {
			a.toast = Tf("Clearing stopped after %d deleted: %v", n, err)
		} else {
			a.toast = Tf("Cleared %d songs from codie", n)
		}
	})
}

// codieForget drops the pairing.
func (a *app) codieForget() {
	a.settings.Codie = nil
	a.changed()
}

// codieTest asks the device how it is — always called off the main thread —
// and says it in the cached status the settings row shows.
func (a *app) codieTest() {
	mygo.RunOnMain(func() {
		a.codiePairing.busy = true
		a.redraw()
	})
	c := a.codieClient()
	var status string
	switch {
	case c == nil:
		status = "Not paired"
	default:
		st, err := c.MusicStorage()
		switch {
		case err != nil:
			status = "Offline: " + err.Error()
		case !st.IsLoaded:
			status = "Online · TF card not ready"
		default:
			status = "Online · TF card ready"
		}
	}
	mygo.RunOnMain(func() {
		a.codiePairing.busy = false
		a.codieStatus = status
		a.codiePairing.status = status
		a.redraw()
	})
}
