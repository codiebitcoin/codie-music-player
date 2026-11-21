package app

import (
	"errors"
	"os"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
	"mygo-music/internal/video"
)

// The video window: what the plugins that play videos answer, in a window of
// its own. A video source is a plugin, usually one of videos alone, though a
// music plugin may export the video functions too and be both.

// videoCapabilities are the video functions a source's Can is asked about as
// its library is made, ahead of the views that hide what it can't do. A plugin
// knows from compiling it, so the first frame shows the source as it stays.
var videoCapabilities = []string{"videoHome", "videoBrowse", "videoSearch", "videoItem", "videoPlay"}

// videoLib is what the app has of a source of videos.
type videoLib struct {
	id   string
	name string
	src  video.VideoSource
	caps map[string]bool // what it can do
}

func newVideoLib(src video.VideoSource) *videoLib {
	caps := make(map[string]bool, len(videoCapabilities))
	for _, name := range videoCapabilities {
		if src.Can(name) {
			caps[name] = true
		}
	}
	info := src.Info()
	return &videoLib{id: info.ID, name: info.Name, src: src, caps: caps}
}

// can reports whether the source does what's named.
func (l *videoLib) can(what string) bool { return l != nil && l.caps[what] }

// makeVideoLibs lists the sources of videos, of the plugins on: those of a
// kind of video alone, and those of a music plugin that also exports the
// video functions. What's off in the settings is off here too, for the same
// switch turns both of a plugin's halves.
func (a *app) makeVideoLibs() {
	a.allVideoLibs = nil
	for _, p := range a.plugins {
		if v := p.Video(); v != nil {
			a.allVideoLibs = append(a.allVideoLibs, newVideoLib(v))
		}
	}
	a.videoLibs = nil
	for _, lib := range a.allVideoLibs {
		if !slices.Contains(a.settings.DisabledPlugins, lib.id) {
			a.videoLibs = append(a.videoLibs, lib)
		}
	}
	// The source the window showed, gone: back to nothing, on the first that
	// is on, if any.
	if a.video.lib != nil && !a.hasVideoLib(a.video.lib.id) {
		a.videoStopPlayer()
		a.video.lib = nil
		a.video.page = videoList
		a.video.home, a.video.search, a.video.item = nil, nil, nil
		if len(a.videoLibs) > 0 {
			a.video.lib = a.videoLibs[0]
			a.videoLoadHome()
		}
	}
}

func (a *app) hasVideoLib(id string) bool {
	for _, lib := range a.videoLibs {
		if lib.id == id {
			return true
		}
	}
	return false
}

// videoLibOf finds the video library of the source id, or nil.
func (a *app) videoLibOf(id string) *videoLib {
	for _, lib := range a.videoLibs {
		if lib.id == id {
			return lib
		}
	}
	return nil
}

// A page of the video window.
type videoPage int

const (
	videoList videoPage = iota // home, or what a search found
	videoItem                  // a video and its episodes
	videoPlay                  // an episode playing
)

// videoState is what the video window shows, and has of the source showing.
type videoState struct {
	lib  *videoLib // the source showing
	page videoPage

	query     string // what's in the search field
	searchBox ui.Handle

	home   *videoHomeData   // the source's own first page, when it makes one
	search *videoSearchData // the search showing, or the last one
	item   *videoItemData   // the video's page showing
	// browsePage is a page a row's More opened, over the first page.
	browsePage *videoBrowseRef

	playing    videoItemRef // the episode in the player, or on its way there
	player     *ui.VideoPlayer
	playerFile string     // the temp file the player was made of, which goes when another comes
	scrub      scrubState  // how far the episode is, as it's dragged
	vol        float64     // how loud, 0 to 100, kept from episode to episode

	openErr string // why the episode didn't open, said under the player
}

// videoItemRef says what a page or the player is of: the item, and which of
// its episodes.
type videoItemRef struct {
	item video.Item
	ep   video.Episode
	at   int // the episode's place, for previous and next
}

type videoHomeData struct {
	browse  *video.Browse
	loading bool
	err     string
}

type videoSearchData struct {
	query   string
	items   []video.Item
	more    bool
	loading bool
	err     string
}

type videoItemData struct {
	id      string
	video   *video.Video
	loading bool
	err     string
}

// videoBrowseRef is a page a row's More opened, as it loads.
type videoBrowseRef struct {
	page  string
	title string
	d     *videoHomeData
}

// openVideoWindow shows the video window, opening it the first time.
func (a *app) openVideoWindow() {
	if a.win == nil {
		return // no app to show a window in, as in tests
	}
	if a.videoWin != nil {
		a.videoWin.Show()
		a.videoWin.Focus()
		return
	}
	if a.video.lib == nil && len(a.videoLibs) > 0 {
		a.video.lib = a.videoLibs[0]
	}
	a.videoWin = mygo.NewWindow(mygo.WindowOptions{
		Title:       T("Videos"),
		Width:       980,
		Height:      680,
		MinWidth:    640,
		MinHeight:   440,
		StateKey:    "videos",
		AutoHideMenuBar: true,
		BackgroundColor: "light-dark(#ffffff, #141416)",
		Content:     ui.View(a.videoView),
	})
	a.videoWin.OnClosed(func() {
		a.videoWin = nil
		a.videoStopPlayer()
	})
}

// videoUse makes lib the video window's source, back at its first page.
func (a *app) videoUse(lib *videoLib) {
	if lib == nil || lib == a.video.lib {
		return
	}
	a.videoStopPlayer()
	a.video.lib = lib
	a.video.page = videoList
	a.video.home, a.video.search, a.video.item, a.video.browsePage = nil, nil, nil, nil
	a.videoLoadHome()
}

// videoLoadHome loads the source's own first page, once.
func (a *app) videoLoadHome() {
	lib := a.video.lib
	if lib == nil || !lib.can("videoHome") {
		return
	}
	if a.video.home != nil && (a.video.home.browse != nil || a.video.home.loading || a.video.home.err != "") {
		return
	}
	d := &videoHomeData{loading: true}
	a.video.home = d
	go func() {
		b, err := lib.src.Home()
		a.post(func() {
			if a.video.home != d {
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

// videoSearch asks the source what q is, going into the list page.
func (a *app) videoSearch(q string) {
	q = strings.TrimSpace(q)
	if q == "" || a.video.lib == nil {
		return
	}
	d := &videoSearchData{query: q, loading: true}
	a.video.search = d
	a.video.page = videoList
	a.video.item, a.video.browsePage = nil, nil
	lib := a.video.lib
	go func() {
		r, err := lib.src.Search(q, 0)
		a.post(func() {
			if a.video.search != d {
				return
			}
			d.loading = false
			if err != nil {
				d.err = videoProblem(err, T("The search failed"))
				return
			}
			d.items, d.more = r.Items, r.More
		})
	}()
}

// videoMoreSearch loads the next page of the search into d.
func (a *app) videoMoreSearch(d *videoSearchData) {
	lib := a.video.lib
	if lib == nil || d.loading || !d.more {
		return
	}
	d.loading, d.err = true, ""
	offset := len(d.items)
	go func() {
		r, err := lib.src.Search(d.query, offset)
		a.post(func() {
			if a.video.search != d {
				return
			}
			d.loading = false
			if err != nil {
				d.err = videoProblem(err, T("The search failed"))
				return
			}
			d.items = append(d.items, r.Items...)
			// A server can claim more while sending none.
			d.more = r.More && len(r.Items) > 0
		})
	}()
}

// videoOpenItem opens a video's page, asking for it anew of where it came.
// From a card, item is what the card showed, so the page can head with
// something while it loads.
func (a *app) videoOpenItem(item video.Item) {
	lib := a.videoLibOfItem(item)
	if lib == nil {
		return
	}
	a.video.lib = lib
	a.video.page = videoItem
	a.video.item = &videoItemData{id: item.ID, loading: true, video: &video.Video{Item: item}}
	go func() {
		v, err := lib.src.Video(item.ID)
		a.post(func() {
			if a.video.item == nil || a.video.item.id != item.ID {
				return
			}
			a.video.item.loading = false
			if err != nil {
				a.video.item.err = videoProblem(err, T("Couldn't load this page"))
				return
			}
			a.video.item.video = v
		})
	}()
}

// videoBack goes back a page of the video window.
func (a *app) videoBack() {
	switch a.video.page {
	case videoItem:
		a.video.page, a.video.item = videoList, nil
	case videoPlay:
		a.videoStopPlayer()
		a.video.page, a.video.playing, a.video.openErr = videoItem, videoItemRef{}, ""
	default:
		a.video.browsePage = nil
	}
}

// videoLibOfItem finds the source an item came from, among those on, or the
// window's for one that doesn't say.
func (a *app) videoLibOfItem(item video.Item) *videoLib {
	if lib := a.videoLibOf(item.Source); lib != nil {
		return lib
	}
	return a.video.lib
}

// videoProblem makes out what went wrong with what a source of videos was
// asked, for a page's error state: msg, with what the source said of it.
func videoProblem(err error, msg string) string {
	switch {
	case errors.Is(err, music.ErrSignIn), errors.Is(err, music.ErrSetup),
		errors.Is(err, music.ErrUnavailable), errors.Is(err, music.ErrUnsupported):
		if reason := music.Reason(err); reason != "" {
			return reason
		}
		return msg
	}
	return msg + ": " + err.Error()
}

// videoStopPlayer ends the episode playing, and its file. Everything that
// touches the player does it on the main thread, as the views read it there.
func (a *app) videoStopPlayer() {
	p, file := a.video.player, a.video.playerFile
	a.video.player, a.video.playerFile = nil, ""
	if p != nil {
		p.Close()
	}
	if file != "" {
		go os.Remove(file)
	}
}
