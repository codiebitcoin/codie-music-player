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
	a.wizard.testBusy, a.wizard.testSaid = true, "正在搜索测试曲…"
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
		a.wizard.testBusy, a.wizard.testSaid = false, "没有可用的音源"
		a.redraw()
		return
	}
	src, id := lib.src, lib.id
	go func() {
		res, err := src.Search("晴天", music.SearchTracks, 0)
		a.post(func() {
			a.wizard.testBusy = false
			if err != nil {
				a.wizard.testSaid = "搜索失败了：" + err.Error()
				a.redraw()
				return
			}
			if res == nil || len(res.Tracks) == 0 {
				a.wizard.testSaid = "没有搜到歌，换一步登录后再试"
				a.redraw()
				return
			}
			tracks := res.Tracks
			for i := range tracks {
				tracks[i].Source = id
			}
			a.wizard.testSaid = tracks[0].Name + " — 在听吗？声音从 codie 设备出来"
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

		steps := []string{"① 绑定机器", "② 登录音源", "③ 尝试播放", "完成"}
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
			ui.Text(c, "绑定 Codie 设备").FontSize(17).FontWeight(700)
			ui.Text(c, "输入设备的 IP 和端口（设备设置页可见，如 192.168.1.160:8081），点「开始配对」后设备屏幕会显示 6 位配对码。").
				FontSize(13).TextColor(p.muted).LineHeight(1.5)
			if a.settings.Codie != nil {
				ui.Text(c, "已配对："+a.settings.Codie.Addr).FontSize(13).TextColor(p.accent)
			}
			ui.Text(c, a.codiePairing.status).FontSize(12.5).TextColor(p.muted)
			if a.codiePairing.challenge == "" {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c.Key("wiz-addr"), &a.codiePairing.addr).Placeholder("IP:端口").Width(230).FontSize(13)
					if ui.Button(c.Key("wiz-pair"), "开始配对").Disabled(a.codiePairing.busy || a.codiePairing.addr == "").Clicked() {
						go a.codiePair(a.codiePairing.addr)
					}
				})
			} else {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c.Key("wiz-code"), &a.codiePairing.code).Placeholder("6 位配对码").Width(130).FontSize(13).AutoFocus()
					if ui.Button(c.Key("wiz-confirm"), "确认配对").Disabled(a.codiePairing.busy || a.codiePairing.code == "").Clicked() {
						go a.codieConfirm(a.codiePairing.code)
					}
				})
			}
			wizardFooter(c, p, a.settings.Codie != nil, a.settings.Codie != nil, "下一步", next, w)

		case 1: // sign in
			ui.Text(c, "登录音源").FontSize(17).FontWeight(700)
			ui.Text(c, "网易云 / QQ / Bilibili 用各自 App 扫码登录；Spotify（MyGotify）不登录也能搜索播放。登录后可听会员曲库和自己的歌单。").
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
							if ui.Button(c.Key("in"), "登录").Clicked() {
								a.promptSignIn(l)
							}
						}
					})
				}
			})
			wizardFooter(c, p, true, false, "下一步", next, w)

		case 2: // try a song on the device
			ui.Text(c, "尝试播放").FontSize(17).FontWeight(700)
			ui.Text(c, "播放一首测试曲，声音应该从 codie 设备出来（设备需打开音乐页面并插好 TF 卡）。").
				FontSize(13).TextColor(p.muted).LineHeight(1.5)
			if w.testSaid != "" {
				ui.Text(c, w.testSaid).FontSize(13).TextColor(p.text).LineHeight(1.4)
			}
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c.Key("test"), "播放测试曲").Disabled(w.testBusy).Clicked() {
					a.wizardTestSong()
				}
			})
			wizardFooter(c, p, true, false, "听到啦，完成", func() { w.step = 3; a.redraw() }, w)

		default: // done
			ui.Text(c, "设置完成 🎉").FontSize(17).FontWeight(700)
			ui.Text(c, "搜索任何歌都会在 codie 设备上播放。设备管理在 设置 → Codie；想重走向导也从那里开始。").
				FontSize(13).TextColor(p.muted).LineHeight(1.5)
			ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
				if ui.Button(c.Key("done"), "开始使用").Clicked() {
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
		if pillButton(c.Key("skip"), nil, "跳过", p.hover.Over(p.bg), p.text).Clicked() {
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
