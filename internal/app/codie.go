package app

import (
	"github.com/egoist/mygo"

	"mygo-music/internal/codieout"
	"mygo-music/internal/music"
)

// codiePairState is where the codie settings section is: what's being paired.
type codiePairState struct {
	addr       string // the address being paired
	code       string // the pairing code being typed
	challenge  string // the pairing challenge, once the device shows its code
	busy       bool   // a pair step is on its way
	status     string // the last thing that happened, for the row to say
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

// codieStatus says how the device is, for the settings row; empty when asked
// nothing yet.
func (a *app) codieStatus() string {
	c := a.codieClient()
	if c == nil {
		return ""
	}
	st, err := c.MusicStorage()
	if err != nil {
		return err.Error()
	}
	if !st.IsLoaded {
		return "在线 · TF 卡未就绪"
	}
	return "在线 · TF 卡就绪"
}

// codiePair asks the device at addr to show its code, and waits for it.
func (a *app) codiePair(addr string) {
	a.codiePairing.busy, a.codiePairing.status = true, ""
	a.codiePairing.addr = addr
	a.redraw()
	c := codieout.New(&codieout.Creds{Addr: addr})
	challenge, err := c.RequestPairing()
	mygo.RunOnMain(func() {
		a.codiePairing.busy = false
		if err != nil {
			a.codiePairing.status = err.Error()
			a.codiePairing.challenge = ""
		} else {
			a.codiePairing.challenge = challenge
			a.codiePairing.status = "看设备屏幕上的 6 位配对码，填到下面"
		}
		a.redraw()
	})
}

// codieConfirm trades the code the listener read on the device for a client
// credential, and remembers the device.
func (a *app) codieConfirm(code string) {
	challenge := a.codiePairing.challenge
	if challenge == "" {
		return
	}
	a.codiePairing.busy, a.codiePairing.status = true, "确认中…"
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
			a.codiePairing.status = "已配对 " + creds.Addr
			a.changed()
		}
		a.redraw()
	})
}

// codieForget drops the pairing.
func (a *app) codieForget() {
	a.settings.Codie = nil
	a.changed()
}

// codieTest asks the device how it is, in a moment, for the row to say.
func (a *app) codieTest() {
	a.codiePairing.busy = true
	a.redraw()
	c := a.codieClient()
	var status string
	if c == nil {
		status = "未配对"
	} else if st, err := c.MusicStorage(); err != nil {
		status = err.Error()
	} else if !st.IsLoaded {
		status = "在线 · TF 卡未就绪"
	} else {
		status = "在线 · TF 卡就绪"
	}
	mygo.RunOnMain(func() {
		a.codiePairing.busy = false
		a.codiePairing.status = status
		a.redraw()
	})
}
