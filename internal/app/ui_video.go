package app

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/video"
)

// The video window: what a source of videos answers, in a window of its own.
// It goes in pages: what the source makes up, or what a search found; a
// video's page, with its episodes; and the episode playing.

// videoView is the content of the video window.
func (a *app) videoView(c *ui.Context) {
	t := *c.Theme()
	p := paletteFor(&t)
	useAccent(&t, p)
	c.SetTheme(&t)
	c.Root().Background(p.bg)
	c.OnShortcut(ui.Cmd, ui.KeyW, func() {
		if a.videoWin != nil {
			a.videoWin.Close()
		}
	})
	ui.Column(c).Fill().Children(func() {
		a.videoBar(c, p)
		ui.Box(c).Grow(1).MinHeight(0).Children(func() {
			switch a.video.page {
			case videoPlay:
				a.videoPlayerContent(c, p)
			case videoItem:
				a.videoItemContent(c, p)
			default:
				a.videoListContent(c, p)
			}
		})
	})
}

// videoBar is the window's top: back, the sources, and the search field.
func (a *app) videoBar(c *ui.Context, p palette) {
	top := max(8, topBar(c))
	ui.Row(c).Height(46).Padding(top, 16, 6, 16).Gap(12).AlignItems(ui.Center).DragWindow().Children(func() {
		if a.video.page != videoList || a.video.browsePage != nil {
			if b := iconButton(c.Key("video-back"), iconBack, 15, p.text, T("Back")); b.Clicked() {
				a.videoBack()
				a.redrawVideo()
			}
		}
		if len(a.videoLibs) == 0 {
			ui.Text(c, T("No source of videos is on. Turn a plugin that plays videos on in the settings.")).FontSize(13).TextColor(p.muted)
			return
		}
		names := make([]string, len(a.videoLibs))
		at := 0
		for i, lib := range a.videoLibs {
			names[i] = lib.name
			if a.video.lib == lib {
				at = i
			}
		}
		if i, changed := choice(c, "video-source", T("Source"), names, at); changed {
			a.videoUse(a.videoLibs[i])
		}
		a.videoSearchField(c, p)
	})
}

// videoSearchField is the search field, which searches where Enter is pressed.
func (a *app) videoSearchField(c *ui.Context, p palette) {
	field := ui.Row(c.Key("video-search")).Width(280).Height(30).Radius(8).AlignItems(ui.Stretch)
	if c.Focused(a.video.searchBox) {
		field.Background(p.bg).Shadow(0, 0, 0, 3, p.accent.Alpha(0.25)).Border(1, p.accent.Alpha(0.6))
	} else {
		field.Background(p.hover).Border(1, p.hairline)
	}
	field.Children(func() {
		in := ui.TextInputBase(c.Key("query"), &a.video.query).Bind(&a.video.searchBox).Grow(1).MinWidth(0).
			PaddingStart(29).PaddingEnd(9).Placeholder(T("Search")).FontSize(13).Label(T("Search Videos"))
		if in.Submitted() {
			a.videoSearch(a.video.query)
		}
		ui.Icon(c, iconSearch).Size(13, 13).TextColor(p.muted).Absolute().Left(9).Top(7.5).IgnoreInput(true)
	})
}

// videoListContent is the list page: a page the source makes up, a page of a
// search, or the source's first page.
func (a *app) videoListContent(c *ui.Context, p palette) {
	switch {
	case a.video.browsePage != nil:
		a.videoBrowseContent(c, p, a.video.browsePage.d)
	case a.video.search != nil:
		a.videoSearchContent(c, p, a.video.search)
	default:
		a.videoHomeContent(c, p)
	}
}

// videoHomeContent is the source's own first page, loading as it needs to.
func (a *app) videoHomeContent(c *ui.Context, p palette) {
	if a.video.lib == nil {
		return
	}
	if a.video.lib.can("videoHome") {
		a.videoLoadHome()
	}
	d := a.video.home
	switch {
	case d == nil:
		a.videoNoHome(c, p)
	case d.browse != nil:
		a.videoBrowseRows(c, d.browse, nil)
	case d.loading:
		a.videoLoading(c, p)
	default:
		a.centered(c, d.err, func() {
			d.browse, d.loading, d.err = nil, false, ""
			a.videoLoadHome()
		})
	}
}

// videoNoHome is what a source without a page of its own says.
func (a *app) videoNoHome(c *ui.Context, p palette) {
	a.centered(c, T("Search above for what this source has"), nil)
}

// videoBrowseContent is a page a row's More opened, asked for as it shows.
func (a *app) videoBrowseContent(c *ui.Context, p palette, d *videoHomeData) {
	switch {
	case d.browse != nil:
		a.videoBrowseRows(c, d.browse, func(c *ui.Context) { a.heading(c, d.browse.Title, "") })
	case d.loading:
		a.videoLoading(c, p)
	default:
		a.centered(c, d.err, nil)
	}
}

// videoSearchContent is what a search found.
func (a *app) videoSearchContent(c *ui.Context, p palette, d *videoSearchData) {
	ui.Scroll(c.Key("video-search\x00"+d.query)).Fill().Padding(6, 20, 28, 20).Children(func() {
		switch {
		case d.loading && len(d.items) == 0:
			a.videoLoading(c, p)
		case d.err != "" && len(d.items) == 0:
			a.centered(c, d.err, func() { a.videoSearch(d.query) })
		case len(d.items) == 0:
			a.empty(c, T("Nothing found"))
		default:
			a.videoGrid(c, d.items)
			if d.more {
				ui.Row(c).Center().Padding(16, 0, 0, 0).Children(func() {
					if ui.Button(c.Key("more"), T("More")).Clicked() {
						a.videoMoreSearch(d)
					}
				})
			}
		}
	})
}

// videoBrowseRows is a page the source made up, in rows of cards. Head, when
// there is one, scrolls with it; a page with none starts at its first row.
func (a *app) videoBrowseRows(c *ui.Context, b *video.Browse, head func(c *ui.Context)) {
	ui.Scroll(c.Key("video-browse\x00"+b.Title)).Fill().Padding(6, 20, 28, 20).Children(func() {
		if head != nil {
			head(c)
		}
		for i := range b.Rows {
			row := &b.Rows[i]
			first := i == 0 && head == nil
			if row.Title != "" {
				margin := float32(26)
				if first {
					margin = 0
				}
				ui.Row(c).Margin(margin, 0, 14, 0).AlignItems(ui.Baseline).Children(func() {
					ui.Text(c, row.Title).FontSize(19).FontWeight(700).LetterSpacing(-0.3).Grow(1).Shrink(1).MinWidth(0)
					if row.MorePage != "" {
						if ui.Button(c.Key("more"), T("See All")).Clicked() {
							a.videoOpenBrowse(row.MorePage, row.Title)
						}
					}
				})
			}
			if len(row.Items) > 0 {
				a.videoGrid(c, row.Items)
			}
		}
	})
}

// videoOpenBrowse opens a page the source makes up, asked for as it shows.
func (a *app) videoOpenBrowse(page, title string) {
	lib := a.video.lib
	if lib == nil {
		return
	}
	d := &videoHomeData{loading: true}
	a.video.browsePage = &videoBrowseRef{page: page, title: title, d: d}
	go func() {
		b, err := lib.src.Browse(page)
		a.post(func() {
			if a.video.browsePage == nil || a.video.browsePage.page != page {
				return
			}
			d.loading = false
			if err != nil {
				d.err = videoProblem(err, T("Couldn't load this page"))
				return
			}
			d.browse = b
		})
	}()
}

// videoLoading is the while of a page not yet there.
func (a *app) videoLoading(c *ui.Context, p palette) {
	ui.Row(c).Fill().Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		ui.Spinner(c).Size(18, 18).Margin(0, 8, 0, 0)
		ui.Text(c, T("Loading…")).FontSize(13).TextColor(p.muted)
	})
}

// videoGrid is a grid of a video's cards, so many to a row as fit.
func (a *app) videoGrid(c *ui.Context, items []video.Item) {
	ui.Grid(c).Columns(a.columns(c, 176)).GapX(16).GapY(24).Children(func() {
		for i := range items {
			a.videoCard(c.Key(items[i].Source+"\x00"+items[i].ID), &items[i])
		}
	})
}

// videoCard is a video as the lists show it: its cover over its title and a
// line under that, opening its page.
func (a *app) videoCard(c *ui.Context, item *video.Item) {
	p := a.pal
	card := ui.ButtonBase(c).Column().Gap(8).AlignItems(ui.Stretch).Justify(ui.Start).FocusRing(false).Label(item.Title)
	hovered := card.Hovered()
	card.Children(func() {
		art := ui.Box(c).AspectRatio(16.0 / 9.0).Radius(10)
		lift := ui.Pose{}
		if hovered {
			lift = ui.Pose{Y: -3}
			art.Shadow(0, 12, 28, -6, p.cardShade)
		} else {
			art.Shadow(0, 2, 8, -2, p.cardShade)
		}
		art.AnimatePose(lift, ui.Snappy)
		art.Children(func() {
			a.coverFill(c, item.Cover, 480, 10)
			if item.Cover == "" {
				ui.Row(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconPlay).Size(28, 28).TextColor(p.faint)
				})
			}
		})
		ui.Text(c, item.Title).FontSize(13).FontWeight(500).MaxLines(2).LineHeight(1.35)
		if item.Subtitle != "" {
			ui.Text(c, item.Subtitle).FontSize(12).TextColor(p.muted).SingleLine().Margin(-6, 0, 0, 0)
		}
	})
	if card.Clicked() {
		a.videoOpenItem(*item)
	}
}

// videoItemContent is a video's page: what it is, and its episodes one to a
// row, the one playing marked.
func (a *app) videoItemContent(c *ui.Context, p palette) {
	d := a.video.item
	if d == nil {
		return
	}
	ui.Scroll(c.Key("video-item\x00"+d.id)).Fill().Padding(6, 20, 28, 20).Children(func() {
		if d.video == nil {
			switch {
			case d.loading:
				a.videoLoading(c, p)
			default:
				a.centered(c, d.err, func() { a.videoOpenItem(d.video.Item) })
			}
			return
		}
		v := d.video
		ui.Row(c).Gap(24).AlignItems(ui.Start).Padding(0, 0, 8, 0).Children(func() {
			ui.Box(c).Width(280).Shrink(0).AspectRatio(16.0 / 9.0).Radius(12).Shadow(0, 8, 24, -6, p.cardShade).Children(func() {
				a.coverFill(c, v.Cover, 480, 12)
				if v.Cover == "" {
					ui.Row(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, iconPlay).Size(32, 32).TextColor(p.faint)
					})
				}
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Padding(6, 0, 0, 0).Children(func() {
				ui.Text(c, v.Title).FontSize(24).FontWeight(800).LetterSpacing(-0.4).MaxLines(2).LineHeight(1.2)
				if v.Subtitle != "" {
					ui.Text(c, v.Subtitle).FontSize(13).TextColor(p.muted)
				}
				if v.Intro != "" {
					ui.Text(c, v.Intro).FontSize(13).TextColor(p.muted).MaxLines(4).LineHeight(1.45).Margin(4, 0, 0, 0)
				}
			})
		})
		if len(v.Episodes) == 0 {
			a.empty(c, T("This video has no episodes to play"))
			return
		}
		ui.Row(c).Margin(18, 0, 10, 0).Children(func() {
			ui.Text(c, T("Episodes")).FontSize(17).FontWeight(700)
		})
		for i := range v.Episodes {
			a.videoEpisodeRow(c.Key(v.Episodes[i].ID), v, &v.Episodes[i], i, p)
		}
	})
}

// videoEpisodeRow is one episode, as a row: its number, its title, how long
// it is, and the mark of the one playing.
func (a *app) videoEpisodeRow(c *ui.Context, v *video.Video, ep *video.Episode, at int, p palette) {
	nowPlaying := a.video.playing.item.Source == v.Source && a.video.playing.item.ID == v.ID && a.video.playing.ep.ID == ep.ID
	row := ui.ButtonBase(c).Height(52).Padding(6, 10).Gap(12).Radius(9).AlignItems(ui.Center).Justify(ui.Start).
		FocusRing(false).Label(ep.Title)
	if hovered := row.Hovered(); hovered && !ep.Unavailable {
		row.Background(p.hover)
	}
	row.Children(func() {
		number := ui.Text(c.Key("at"), strconv.Itoa(at+1)).FontSize(13).TextColor(p.faint).Width(26).TextAlign(ui.Center).SingleLine()
		if nowPlaying {
			number.TextColor(p.accent).FontWeight(600)
		}
		title := ui.Text(c, ep.Title).FontSize(13.5).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		if ep.Unavailable {
			title.TextColor(p.faint)
		} else if nowPlaying {
			title.TextColor(p.accent).FontWeight(600)
		}
		if ep.Duration > 0 {
			ui.Text(c, clock(time.Duration(ep.Duration)*time.Millisecond)).FontSize(12).TextColor(p.muted)
		}
		if ep.Unavailable {
			ui.Text(c, T("Unavailable")).FontSize(12).TextColor(p.faint)
		}
	})
	if row.Clicked() && !ep.Unavailable {
		a.videoPlayEpisode(v.Item, *ep, at)
	}
}
