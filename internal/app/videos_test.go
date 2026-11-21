package app

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
	"mygo-music/internal/video"
)

// fakeVideoSource is a video.VideoSource that does what its tests set, and
// answers ErrUnsupported to the rest. Can says true of what's in caps, which
// newFakeVideoSource fills with everything; a test deletes what it wants hidden.
type fakeVideoSource struct {
	info video.Info
	caps map[string]bool

	searchMu  sync.Mutex
	searched  string // the last query asked for
	playMu    sync.Mutex
	playAsked []string // the episodes play was asked for, "id\x00ep"

	home   func() (*video.Browse, error)
	browse func(page string) (*video.Browse, error)
	search func(query string, offset int) (*video.VideoResult, error)
	item   func(id string) (*video.Video, error)
	play   func(id, ep string) (*video.Stream, error)
}

func newFakeVideoSource(id string) *fakeVideoSource {
	s := &fakeVideoSource{info: video.Info{ID: id, Name: id}, caps: map[string]bool{}}
	for _, name := range videoCapabilities {
		s.caps[name] = true
	}
	return s
}

func (s *fakeVideoSource) Info() video.Info     { return s.info }
func (s *fakeVideoSource) Can(what string) bool { return s.caps[what] }
func (s *fakeVideoSource) Home() (*video.Browse, error) {
	if s.home == nil {
		return nil, music.ErrUnsupported
	}
	return s.home()
}
func (s *fakeVideoSource) Browse(page string) (*video.Browse, error) {
	if s.browse == nil {
		return nil, music.ErrUnsupported
	}
	return s.browse(page)
}
func (s *fakeVideoSource) Search(query string, offset int) (*video.VideoResult, error) {
	s.searchMu.Lock()
	s.searched = query
	s.searchMu.Unlock()
	if s.search == nil {
		return nil, music.ErrUnsupported
	}
	return s.search(query, offset)
}
func (s *fakeVideoSource) Video(id string) (*video.Video, error) {
	if s.item == nil {
		return nil, music.ErrUnsupported
	}
	return s.item(id)
}
func (s *fakeVideoSource) Play(id, ep string) (*video.Stream, error) {
	s.playMu.Lock()
	s.playAsked = append(s.playAsked, id+"\x00"+ep)
	s.playMu.Unlock()
	if s.play == nil {
		return nil, music.ErrUnsupported
	}
	return s.play(id, ep)
}

var _ video.VideoSource = (*fakeVideoSource)(nil)

// addFakeVideo puts src among the sources of videos of an app, showing.
func addFakeVideo(a *app, src video.VideoSource) *videoLib {
	lib := newVideoLib(src)
	a.allVideoLibs = append(a.allVideoLibs, lib)
	a.videoLibs = append(a.videoLibs, lib)
	if a.video.lib == nil {
		a.video.lib = lib
	}
	return lib
}

// videoLibrary is a source of videos with a drama of two episodes, for the
// window's tests.
func videoLibrary(a *app) *fakeVideoSource {
	src := newFakeVideoSource("demo")
	src.home = func() (*video.Browse, error) {
		return &video.Browse{Title: "Home", Rows: []video.Row{{Title: "Now on", Items: []video.Item{
			{ID: "d1", Title: "Drama 1", Subtitle: "2 episodes"},
		}}}}, nil
	}
	src.search = func(query string, offset int) (*video.VideoResult, error) {
		return &video.VideoResult{Items: []video.Item{{ID: "d1", Title: "Drama 1"}}, More: false}, nil
	}
	src.play = func(id, ep string) (*video.Stream, error) {
		return &video.Stream{URL: "https://v.example/" + ep + ".mp4", Type: "mp4"}, nil
	}
	src.item = func(id string) (*video.Video, error) {
		return &video.Video{Item: video.Item{ID: id, Title: "Drama 1", Source: "demo"}, Episodes: []video.Episode{
			{ID: "e1", Title: "Episode 1", Duration: 90000},
			{ID: "e2", Title: "Episode 2"},
		}}, nil
	}
	addFakeVideo(a, src)
	return src
}

// A search of the video window finds the drama, its page lists the episodes,
// and clicking one opens it in the player; back of the player is the drama's
// page, nothing playing.
func TestVideoWindowSearchOpensAndPlays(t *testing.T) {
	a := newTestApp()
	src := videoLibrary(a)
	posts := posts(a)
	realOpen := openEpisodeStream
	opened := make(chan struct{}, 1)
	openEpisodeStream = func(*video.Stream) (*ui.VideoPlayer, string, error) {
		opened <- struct{}{}
		return nil, "", nil
	}
	defer func() { openEpisodeStream = realOpen }()

	tt := ui.NewTester(a.videoView, 900, 700)
	tt.Frame()
	// The source's first page came, and shows the drama of the home row.
	runPost(t, posts)
	settled(tt)
	if err := tt.Click("Drama 1"); err != nil {
		t.Fatal(err)
	}
	runPost(t, posts)
	settled(tt)
	if a.video.page != videoItem || a.video.item == nil || len(a.video.item.video.Episodes) != 2 {
		t.Fatalf("the drama's page: %+v", a.video.item)
	}
	// An episode opens the player.
	if err := tt.Click("Episode 2"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-opened:
	case <-time.After(2 * time.Second):
		t.Fatal("the episode never opened")
	}
	runPost(t, posts)
	if a.video.page != videoPlay || a.video.playing.ep.ID != "e2" || a.video.playing.at != 1 {
		t.Fatalf("what plays: %+v", a.video.playing)
	}
	settled(tt)
	tt.Click("Back")
	if a.video.page != videoItem || a.video.player != nil {
		t.Fatalf("back of the player: page %v player %v", a.video.page, a.video.player)
	}
	settled(tt)

	// The search field searches what's typed, and what it finds replaces
	// the page.
	a.videoBack()
	if err := tt.Click("Search Videos"); err != nil {
		t.Fatal(err)
	}
	tt.Type("drama")
	tt.Key(0, ui.KeyEnter)
	waitSearched(t, src, "drama")
	runPost(t, posts)
	settled(tt)
	if a.video.search == nil || len(a.video.search.items) != 1 {
		t.Fatalf("what the search found: %+v", a.video.search)
	}
	settled(tt)
}

// waitSearched waits until the source is asked for q.
func waitSearched(t *testing.T, src *fakeVideoSource, q string) {
	t.Helper()
	for i := 0; i < 200; i++ {
		src.searchMu.Lock()
		got := src.searched
		src.searchMu.Unlock()
		if got == q {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("nothing searched for %q", q)
}

// The example plugin of videos, dropped in where plugins are discovered,
// becomes a source of videos and none of the sources of music.
func TestVideoPluginIsNoMusicSource(t *testing.T) {
	a := newTestApp()
	dir := filepath.Join(pluginsDir(), "demo-video")
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "examples", "demo-video-plugin"))); err != nil && !os.IsExist(err) {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	a.makeSources()
	if slices.ContainsFunc(a.libs, func(l *library) bool { return l.id == "demo-video" }) {
		t.Error("a source of videos alone is among the sources of music")
	}
	if len(a.videoLibs) != 1 || a.videoLibs[0].id != "demo-video" {
		t.Fatalf("the sources of videos: %+v", a.videoLibs)
	}
	// Off in the settings, it takes its videos with it, and what the window
	// showed of it goes.
	a.video.playing = videoItemRef{item: video.Item{ID: "d1"}, ep: video.Episode{ID: "e1"}}
	a.settings.DisabledPlugins = []string{"demo-video"}
	a.makeVideoLibs()
	if len(a.videoLibs) != 0 || a.video.lib != nil {
		t.Fatalf("the sources of videos with the plugin off: %+v", a.videoLibs)
	}
}

// The arguments of joining an episode's picture and sound name the headers of
// each input, and the ones of a stream alone name its own.
func TestEpisodeFFmpegArgs(t *testing.T) {
	joined := videoFFmpegArgs(&video.Stream{
		URL: "https://v.example/p.m4s", AudioURL: "https://a.example/a.m4a",
		Headers: map[string]string{"Referer": "https://v.example", "User-Agent": "Codie/1"},
	}, "/tmp/e.mp4")
	want := []string{
		"-hide_banner", "-loglevel", "error",
		"-headers", "Referer: https://v.example\r\nUser-Agent: Codie/1\r\n", "-i", "https://v.example/p.m4s",
		"-headers", "Referer: https://v.example\r\nUser-Agent: Codie/1\r\n", "-i", "https://a.example/a.m4a",
		"-map", "0:v:0", "-map", "1:a:0?",
		"-c", "copy", "-movflags", "+faststart", "-y", "/tmp/e.mp4",
	}
	if !slices.Equal(joined, want) {
		t.Errorf("joined: %q", joined)
	}
	one := videoFFmpegArgs(&video.Stream{URL: "https://v.example/e.m3u8", Type: "hls"}, "/tmp/e.mp4")
	if !slices.Equal(one, []string{"-hide_banner", "-loglevel", "error", "-i", "https://v.example/e.m3u8", "-c", "copy", "-movflags", "+faststart", "-y", "/tmp/e.mp4"}) {
		t.Errorf("one: %q", one)
	}
}

// What went wrong with an episode says what the source said of it.
func TestVideoProblem(t *testing.T) {
	err := music.Fail(music.ErrUnavailable, "only encrypted streams here")
	if got := videoProblem(err, T("Couldn't play this episode")); got != "only encrypted streams here" {
		t.Errorf("a reason said: %q", got)
	}
	if got := videoProblem(music.ErrUnavailable, T("Couldn't play this episode")); got != T("Couldn't play this episode") {
		t.Errorf("no reason: %q", got)
	}
	other := errors.New("the CDN fell over")
	if got := videoProblem(other, "msg"); !strings.Contains(got, "the CDN fell over") {
		t.Errorf("a failure of no kind: %q", got)
	}
}
