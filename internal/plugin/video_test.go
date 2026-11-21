package plugin

import (
	"path/filepath"
	"strings"
	"testing"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// The example the video window's tests copy from does all that a video source
// can, through the real host, so copying it starts from something that works.
func TestDemoVideoPlugin(t *testing.T) {
	p, err := Load(filepath.Join("..", "..", "examples", "demo-video-plugin"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)

	if !p.VideoOnly() {
		t.Error("a kind of video plugin isn't a source of videos alone")
	}
	if p.Can("search") {
		t.Error("a video plugin isn't a source of songs")
	}
	if _, err := p.Search("drama", music.SearchTracks, 0); err != music.ErrUnsupported {
		t.Errorf("searching a video plugin: %v", err)
	}

	v := p.Video()
	if v == nil {
		t.Fatal("no video source in the video plugin")
	}
	if !v.Can("videoHome") || !v.Can("videoBrowse") || !v.Can("videoSearch") || !v.Can("videoItem") || !v.Can("videoPlay") {
		t.Errorf("can: %v %v %v %v %v", v.Can("videoHome"), v.Can("videoBrowse"), v.Can("videoSearch"), v.Can("videoItem"), v.Can("videoPlay"))
	}
	if v.Can("lyrics") {
		t.Error("said it can do what no video source exports")
	}
	if v.Info().ID != "demo-video" || v.Info().Name != "Demo Video" {
		t.Errorf("info: %+v", v.Info())
	}

	home, err := v.Home()
	if err != nil {
		t.Fatal(err)
	}
	if len(home.Rows) != 1 || len(home.Rows[0].Items) != 3 || home.Rows[0].Items[0].Source != "demo-video" {
		t.Errorf("home: %+v", home)
	}
	if b, err := v.Browse("all"); err != nil || len(b.Rows[0].Items) != 3 {
		t.Errorf("browse: %+v, %v", b, err)
	}

	r, err := v.Search("drama", 0)
	if err != nil || len(r.Items) != 3 || r.Items[0].Source != "demo-video" || r.More {
		t.Errorf("search: %+v, %v", r, err)
	}

	d, err := v.Video("d1")
	if err != nil || d.Title != "Drama 1" || len(d.Episodes) != 3 || d.Source != "demo-video" {
		t.Errorf("video: %+v, %v", d, err)
	}

	s, err := v.Play("d1", "d1-e2")
	if err != nil || s.Type != "mp4" || s.URL != "https://demo.video/d1/d1-e2.mp4" {
		t.Errorf("play: %+v, %v", s, err)
	}
	if _, err := v.Play("d1", "nope"); err == nil || !strings.Contains(err.Error(), music.ErrUnavailable.Error()) {
		t.Errorf("playing what isn't there: %v", err)
	}
}

// A music plugin may export the video functions too, and be both.
func TestMusicPluginWithVideos(t *testing.T) {
	p := load(t, `{"id":"both","playback":"stream"}`, `
export async function search() { return { tracks: [] }; }
export async function stream({ id }) { return { url: "https://x/"+id, type: "mp3" }; }
export async function videoSearch() { return { items: [{ id: "v1", title: "One" }] }; }
export async function videoItem({ id }) { return { id, title: "One", episodes: [{ id: "v1-e1", title: "One" }] }; }
export async function videoPlay({ id }) { return { url: "https://x/"+id+".mp4", type: "mp4" }; }
`)
	if p.VideoOnly() {
		t.Error("a music plugin isn't a source of videos alone")
	}
	v := p.Video()
	if v == nil {
		t.Fatal("no video source in the music plugin")
	}
	if _, err := p.Stream("t1", "high"); err != nil {
		t.Errorf("stream: %v", err)
	}
	r, err := v.Search("one", 0)
	if err != nil || len(r.Items) != 1 || r.Items[0].Source != "both" {
		t.Errorf("search: %+v, %v", r, err)
	}
}

// A plugin of videos alone isn't a source of music, whatever else its answers
// would have the app believe.
func TestVideoOnlyPluginIsNoMusicSource(t *testing.T) {
	p := load(t, `{"id":"vid","kind":"video"}`, `
export async function videoSearch() { return { items: [] }; }
export async function videoItem({ id }) { return { id, title: "One" }; }
export async function videoPlay() { return { url: "https://x/1.mp4", type: "mp4" }; }
`)
	if !p.VideoOnly() {
		t.Error("not known for a source of videos alone")
	}
	v := p.Video()
	if v == nil {
		t.Fatal("no video source")
	}
	for _, name := range []string{"videoHome", "videoBrowse"} {
		if v.Can(name) {
			t.Errorf("said it can %s", name)
		}
	}
	if _, err := v.Home(); err != music.ErrUnsupported {
		t.Errorf("home of what isn't exported: %v", err)
	}
	if _, err := v.Browse("all"); err != music.ErrUnsupported {
		t.Errorf("browse of what isn't exported: %v", err)
	}
}

// The kind says music or video, and nothing else.
func TestUnknownKindDoesntLoad(t *testing.T) {
	dir, data := write(t, `{"id":"odd","kind":"film"}`, "export async function videoSearch() {}")
	if _, err := Load(dir, data); err == nil || !strings.Contains(err.Error(), "music or video") {
		t.Errorf("loading a kind of nothing: %v", err)
	}
}

// The plugins that ship with the app compile, and the ones of videos do what
// a source of videos has to. bilibili and YouTube Music are sources of songs
// that also play videos; huangguo is a source of videos alone.
func TestBundledVideoPluginsCompile(t *testing.T) {
	for _, it := range []struct {
		name      string
		videoOnly bool
		inChinese string // what its name is asked for in Chinese; as it is for a brand
	}{
		{"huangguo", true, "黄果短剧"},
		{"bilibili", false, "哔哩哔哩"},
		{"youtube-music", false, "YouTube Music"},
	} {
		t.Run(it.name, func(t *testing.T) {
			p, err := Load(filepath.Join("..", "..", "plugins", it.name), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(p.Close)
			if p.VideoOnly() != it.videoOnly {
				t.Errorf("video only: %v", p.VideoOnly())
			}
			v := p.Video()
			if v == nil {
				t.Fatal("no source of videos")
			}
			for _, what := range []string{"videoSearch", "videoItem", "videoPlay"} {
				if !v.Can(what) {
					t.Errorf("doesn't export %s", what)
				}
			}
			// Asking it in Chinese has its name in Chinese, as it translated it.
			defer i18n.SetLanguage(i18n.Language())
			i18n.SetLanguage(i18n.Chinese)
			if got := v.Info().Name; got != it.inChinese {
				t.Errorf("the name in Chinese: %q, want %q", got, it.inChinese)
			}
		})
	}
}
