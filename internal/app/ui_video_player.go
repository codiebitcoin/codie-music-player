package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The player page: the episode on the window's dark stage, under it what
// plays, pauses, moves about it, and how loud it is.

// videoPlayerContent is the episode playing, or why it isn't yet.
func (a *app) videoPlayerContent(c *ui.Context, p palette) {
	playing := a.video.playing
	ui.Column(c).Fill().Children(func() {
		// The stage, black as a cinema's, the episode contained in it.
		ui.Box(c).Grow(1).MinHeight(0).Background(ui.RGB(0, 0, 0)).Center().Children(func() {
			player := a.video.player
			switch {
			case player != nil:
				ui.Video(c, player).Fill().Fit(ui.Contain)
				if !player.Ready() {
					a.videoOpening(c, p)
				}
			case a.video.openErr != "":
				ui.Column(c).Gap(12).Center().Children(func() {
					ui.Text(c, a.video.openErr).TextColor(ui.RGB(255, 255, 255)).FontSize(13).TextAlign(ui.Center).MaxWidth(420)
					if ui.Button(c.Key("retry"), T("Try Again")).Clicked() {
						a.videoPlayEpisode(playing.item, playing.ep, playing.at)
					}
				})
			default:
				a.videoOpening(c, p)
			}
		})
		a.videoControls(c, p, playing)
	})
}

// videoOpening is the while of an episode on its way to the stage.
func (a *app) videoOpening(c *ui.Context, p palette) {
	ui.Row(c).Gap(10).Center().Children(func() {
		ui.Spinner(c).TextColor(ui.RGBA(255, 255, 255, 0.7))
		ui.Text(c, T("Opening…")).FontSize(13).TextColor(ui.RGBA(255, 255, 255, 0.7))
	})
}

// videoControls is what an episode answers: its name and the one playing,
// how far it is, and the buttons of playing it.
func (a *app) videoControls(c *ui.Context, p palette, playing videoItemRef) {
	ui.Column(c).Shrink(0).Padding(10, 16, 14, 16).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Baseline).Children(func() {
			ui.Text(c, playing.item.Title).FontSize(15).FontWeight(600).Grow(1).Shrink(1).MinWidth(0).SingleLine()
			if playing.ep.Title != "" && playing.ep.Title != playing.item.Title {
				ui.Text(c, playing.ep.Title).FontSize(12.5).TextColor(p.muted).Shrink(0).SingleLine()
			}
		})
		player := a.video.player
		if player != nil && player.Ready() && player.Err() == nil {
			a.videoScrubber(c, p, player)
		}
		ui.Row(c).Height(38).Gap(6).AlignItems(ui.Center).Children(func() {
			// The episode before, when there is one.
			if a.videoHasEpisode(-1) {
				if b := iconButton(c.Key("video-prev"), iconPrev, 15, p.text, T("Previous")); b.Clicked() {
					a.videoNextEpisode(true)
				}
			}
			if player != nil {
				if b := playButton(c.Key("video-play"), player.Playing(), 17, p.text); b.Clicked() {
					if player.Playing() {
						player.Pause()
					} else {
						player.Play()
					}
				}
			} else {
				// Nothing to play yet; the button waits, dim.
				playButton(c.Key("video-play"), false, 17, p.faint)
			}
			if a.videoHasEpisode(1) {
				if b := iconButton(c.Key("video-next"), iconNext, 15, p.text, T("Next")); b.Clicked() {
					a.videoNextEpisode(false)
				}
			}
			ui.Spacer(c)
			a.videoVolume(c, p, player)
		})
	})
}

// videoHasEpisode reports whether there's an episode to go to, back or
// forward by dir.
func (a *app) videoHasEpisode(dir int) bool {
	d := a.video.item
	if d == nil || d.video == nil {
		return false
	}
	at := a.video.playing.at + dir
	return at >= 0 && at < len(d.video.Episodes)
}

// videoScrubber is how far the episode is, which seeks where it is dragged
// or clicked, as the player bar's does without its smoke.
func (a *app) videoScrubber(c *ui.Context, pal palette, player *ui.VideoPlayer) {
	st := &a.video.scrub
	dur := player.Duration().Seconds()
	if !st.active && !st.held {
		st.v = player.Position().Seconds()
	}
	s := ui.SliderBase(c, &st.v, 0, max(dur, 1)).Height(20).FocusRing(false).Label(T("Progress")).Cursor(ui.CursorPointer)
	s.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputPointerDown:
			st.held = ev.Button == 0
		case ui.InputPointerUp:
			st.held = false
		}
		return false
	})
	seek := func() {
		player.Seek(time.Duration(min(st.v, max(0, dur-1)) * float64(time.Second)))
	}
	switch {
	case st.held:
		st.active = true
	case st.active:
		st.active = false
		seek()
	case s.Changed():
		seek()
	}
	at := st.v
	if !st.active {
		at = player.Position().Seconds()
	}
	frac := float32(0)
	if dur > 0 {
		frac = float32(min(1, max(0, at/dur)))
	}
	thick := float32(4)
	if hot := s.Hovered() || st.held; hot {
		thick = 6
	}
	thick = s.AnimateTo("thick", thick, ui.Snappy)
	s.Draw(func(p *ui.Painter, r ui.Rect) {
		y := r.Y + (r.H-thick)/2
		p.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: thick}, pal.hairline, thick/2)
		p.Fill(ui.Rect{X: r.X, Y: y, W: max(thick, r.W*frac), H: thick}, c.Theme().Accent, thick/2)
	})
	ui.Row(c).Justify(ui.End).Children(func() {
		ui.Text(c, clock(player.Position())+" / "+clock(player.Duration())).FontSize(11.5).TextColor(pal.muted)
	})
}

// videoVolume is how loud the episode is, its icon saying whether it's
// muted. The app keeps the level, as the main player does: each episode
// plays as loud as the last was.
func (a *app) videoVolume(c *ui.Context, p palette, player *ui.VideoPlayer) {
	if player == nil {
		return
	}
	icon := iconVolume
	if player.Muted() || a.video.vol == 0 {
		icon = iconMute
	} else if a.video.vol < 50 {
		icon = iconVolumeLow
	}
	if b := iconButton(c.Key("video-mute"), icon, 14, p.muted, T("Mute")); b.Clicked() {
		player.SetMuted(!player.Muted())
	}
	sl := ui.Slider(c.Key("video-vol"), &a.video.vol, 0, 100).Width(90)
	if sl.Changed() {
		player.SetVolume(a.video.vol / 100)
	}
}
