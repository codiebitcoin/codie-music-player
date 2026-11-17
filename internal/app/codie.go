package app

import (
	"github.com/egoist/mygo"

	"mygo-music/internal/codieout"
	"mygo-music/internal/music"
)

// codiePairState is where the codie settings section is: what's being paired.
type codiePairState struct {
	addr      string // the address being paired
	code      string // the pairing code being typed
	challenge string // the pairing challenge, once the device shows its code
	busy      bool   // a pair step is on its way
	status    string // the last thing that happened, for the row to say
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
	c := codieout.New(&codieout.Creds{Addr: addr})
	challenge, err := c.RequestPairing()
	mygo.RunOnMain(func() {
		a.codiePairing.busy = false
		if err != nil {
			a.codiePairing.status = err.Error()
			a.codiePairing.challenge = ""
		} else {
			a.codiePairing.challenge = challenge
			a.codiePairing.status = T("Enter the 6-digit code shown on the device's screen")
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
	a.codiePairing.busy, a.codiePairing.status = true, T("Confirming…")
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
			a.codieStatus = T("Paired, checking…")
			a.codiePairing.status = Tf("Paired to %s", creds.Addr)
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
	a.codiePairing.busy, a.codiePairing.status = true, T("Clearing device songs…")
	a.redraw()
	n, err := c.ClearCardUploads()
	mygo.RunOnMain(func() {
		a.codiePairing.busy = false
		if err != nil {
			a.codieStatus = Tf("Clearing stopped after %d deleted: %v", n, err)
		} else {
			a.codieStatus = Tf("Cleared %d device songs", n)
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
		status = T("Not paired")
	default:
		st, err := c.MusicStorage()
		switch {
		case err != nil:
			status = Tf("Offline: %s", err.Error())
		case !st.IsLoaded:
			status = T("Online · TF card not ready")
		default:
			status = T("Online · TF card ready")
		}
	}
	mygo.RunOnMain(func() {
		a.codiePairing.busy = false
		a.codieStatus = status
		a.codiePairing.status = status
		a.redraw()
	})
}
