package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// codieWizard is the first-run setup: bind a codie device, sign in to a
// source, try a song on the device. It shows until it's finished (or skipped),
// and the settings' Codie section can start it again.
type wizardState struct {
	open     bool
	step     int // 0 bind, 1 sign in, 2 try a song, 3 done
	testBusy bool
	testSaid string // what trying a song said
}

// startWizard shows the setup guide; from the settings it can be walked again.
func (a *app) startWizard() {
	a.wizard = wizardState{open: true}
	a.codiePairing = codiePairState{}
	a.redraw()
}

// wizardFinished says whether the listener has been through it.
func (a *app) wizardFinished() bool { return a.settings.CodieSetupDone }

// wizardTestSong plays a song through the whole way — source, codie — for the
// listener to hear that it works. Netease answers without an account; the
// Spotify-catalog plugin does too.
func (a *app) wizardTestSong() {
	a.wizard.testBusy, a.wizard.testSaid = true, T("Searching for a test song…")
	a.redraw()
	lib := a.libOf("netease")
	if lib == nil || !lib.can("search") {
		lib = nil
		for _, l := range a.libs {
			if l.can("search") {
				lib = l
				break
			}
		}
	}
	if lib == nil {
		a.wizard.testBusy, a.wizard.testSaid = false, T("No source available")
		a.redraw()
		return
	}
	src, id := lib.src, lib.id
	go func() {
		res, err := src.Search(T("Sunny Day"), music.SearchTracks, 0)
		a.post(func() {
			a.wizard.testBusy = false
			if err != nil {
				a.wizard.testSaid = Tf("Search failed: %s", err.Error())
				a.redraw()
				return
			}
			if res == nil || len(res.Tracks) == 0 {
				a.wizard.testSaid = T("No songs found; sign in on the next step and try again")
				a.redraw()
				return
			}
			tracks := res.Tracks
			for i := range tracks {
				tracks[i].Source = id
			}
			a.wizard.testSaid = Tf("%s — hearing it? The sound comes from the codie device", tracks[0].Name)
			a.pl.PlayQueue(tracks, 0)
			a.redraw()
		})
	}()
}

// codieWizardView draws the setup guide over whatever page shows.
func (a *app) codieWizardView(c *ui.Context, p palette) {
	w := &a.wizard
	if !w.open {
		return
	}
	ui.DialogBase(c.Key("codie-wizard"), &w.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.45))
		panel.Width(460).Padding(28).Radius(18).Gap(14).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Colors: true, Enter: &ui.Motion{Scale: 0.95}, Exit: &ui.Motion{Scale: 0.96}})

		steps := []string{T("① Pair the device"), T("② Sign in to a source"), T("③ Try a song"), T("Done")}
		ui.Row(c).Gap(6).Children(func() {
			for i, s := range steps {
				t := ui.Text(c.Key(s), s).FontSize(12)
				if i == w.step {
					t.FontWeight(700).TextColor(p.text)
				} else {
					t.TextColor(p.faint)
				}
				if i < len(steps)-1 {
					ui.Text(c.Key(fmt.Sprint(i)), " · ").FontSize(12).TextColor(p.faint)
				}
			}
		})

		next := func() { w.step++; a.redraw() }

		switch w.step {
		case 0: // bind the device
			ui.Text(c, T("Bind the Codie device")).FontSize(17).FontWeight(700)
			ui.Text(c, T("Scan for the device on this network, or type its IP (the port is 8081 by default; a full IP:port works too). Once pairing starts, the device's screen shows a 6-digit code.")).
				FontSize(13).TextColor(p.muted).LineHeight(1.5)
			if a.settings.Codie != nil {
				ui.Text(c, Tf("Paired: %s", a.settings.Codie.Addr)).FontSize(13).TextColor(p.accent)
			}
			ui.Text(c, T(a.codiePairing.status)).FontSize(12.5).TextColor(p.muted)
			if a.codiePairing.challenge == "" {
				// Finding the device comes first: its address is typed only when
				// the sweep doesn't see it.
				ui.Box(c).Gap(8).Children(func() { a.codieScanView(c, p) })
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c.Key("wiz-addr"), &a.codiePairing.addr).Placeholder(T("Device IP")).Width(230).FontSize(13)
					if ui.Button(c.Key("wiz-pair"), T("Start pairing")).Disabled(a.codiePairing.busy || a.codiePairing.addr == "").Clicked() {
						a.codiePair(a.codiePairing.addr)
					}
				})
			} else {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c.Key("wiz-code"), &a.codiePairing.code).Placeholder(T("6-digit code")).Width(130).FontSize(13).AutoFocus()
					if ui.Button(c.Key("wiz-confirm"), T("Confirm pairing")).Disabled(a.codiePairing.busy || a.codiePairing.code == "").Clicked() {
						go a.codieConfirm(a.codiePairing.code)
					}
				})
			}
			wizardFooter(c, p, a.settings.Codie != nil, a.settings.Codie != nil, T("Next step"), next, w)

		case 1: // sign in
			ui.Text(c, T("Sign in to a source")).FontSize(17).FontWeight(700)
			ui.Text(c, T("Netease, QQ and Bilibili sign in by scanning their app's code; Spotify (MyGotify) searches and plays without signing in. Signed in, you get the members' catalog and your own playlists.")).
				FontSize(13).TextColor(p.muted).LineHeight(1.5)
			ui.Box(c).Gap(6).Children(func() {
				for _, lib := range a.libs {
					if lib.id == "local" || !lib.can("signIn") {
						continue
					}
					l := lib
					ui.Row(c.Key(l.id)).Gap(10).AlignItems(ui.Center).Children(func() {
						ui.Text(c.Key("n"), l.name()).FontSize(13).Grow(1)
						if l.account != nil {
							ui.Text(c.Key("acc"), "✓ "+l.account.Name).FontSize(12.5).TextColor(p.accent)
						} else {
							if ui.Button(c.Key("in"), T("Sign in")).Clicked() {
								a.promptSignIn(l)
							}
						}
					})
				}
			})
			wizardFooter(c, p, true, false, T("Next step"), next, w)

		case 2: // try a song on the device
			ui.Text(c, T("Try a song")).FontSize(17).FontWeight(700)
			ui.Text(c, T("Play a test song; the sound should come from the codie device (it needs its music page open and the TF card in).")).
				FontSize(13).TextColor(p.muted).LineHeight(1.5)
			if w.testSaid != "" {
				ui.Text(c, w.testSaid).FontSize(13).TextColor(p.text).LineHeight(1.4)
			}
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c.Key("test"), T("Play the test song")).Disabled(w.testBusy).Clicked() {
					a.wizardTestSong()
				}
			})
			wizardFooter(c, p, true, false, T("Heard it, done"), func() { w.step = 3; a.redraw() }, w)

		default: // done
			ui.Text(c, T("Setup done 🎉")).FontSize(17).FontWeight(700)
			ui.Text(c, T("Any song you search for plays on the codie device. The device lives in Settings → Codie, and the guide can be walked again from there.")).
				FontSize(13).TextColor(p.muted).LineHeight(1.5)
			ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
				if ui.Button(c.Key("done"), T("Start using it")).Clicked() {
					a.settings.CodieSetupDone = true
					a.settings.CodieOn = a.settings.Codie != nil
					a.changed()
					w.open = false
				}
			})
		}
	})
}

// wizardFooter is what each step ends with: a way out, and a way on when can
// (or a hint of why not, for the binding step).
func wizardFooter(c *ui.Context, p palette, canNext, skipAsk bool, nextLabel string, next func(), w *wizardState) {
	ui.Row(c).Gap(10).Justify(ui.End).Margin(6, 0, 0, 0).Children(func() {
		if pillButton(c.Key("skip"), nil, T("Skip"), p.hover.Over(p.bg), p.text).Clicked() {
			if skipAsk && w.step == 0 {
				w.step = 3
			} else {
				w.step++
			}
		}
		b := ui.Button(c.Key("next"), nextLabel)
		if !canNext {
			b.Disabled(true)
		}
		if b.Clicked() {
			next()
		}
	})
}
