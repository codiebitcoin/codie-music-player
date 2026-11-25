package app

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// Shipped plugins are addressed with slashes. On Windows a filepath join made
// plugins\netease, WalkDir found nothing inside the app, and the first launch
// installed none of the sources.
func TestBundledPluginPathsStaySlashSeparated(t *testing.T) {
	if got := bundledRoot("netease"); got != "plugins/netease" {
		t.Fatalf("bundled root is %q, want plugins/netease", got)
	}
}

// A source already on disk is left as the listener has it, and one that isn't
// is copied out of the app, a file in a nested folder included.
func TestInstallBundledPlugins(t *testing.T) {
	bundled := fstest.MapFS{
		"plugins/netease/plugin.json":         {Data: []byte(`{"id":"netease"}`)},
		"plugins/netease/index.js":            {Data: []byte("netease")},
		"plugins/qqmusic/plugin.json":         {Data: []byte(`{"id":"qqmusic"}`)},
		"plugins/qqmusic/dist/index.js":       {Data: []byte("qq")},
		"plugins/bilibili/plugin.json":        {Data: []byte(`{"id":"bilibili"}`)},
		"plugins/bilibili/locales/zh-CN.json": {Data: []byte("zh")},
		"plugins/youtube-music/plugin.json":   {Data: []byte(`{"id":"youtube-music"}`)},
		"plugins/mygotify/plugin.json":        {Data: []byte(`{"id":"mygotify"}`)},
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "qqmusic"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "qqmusic", "plugin.json"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	installBundledPlugins(bundled, dir)

	got, err := os.ReadFile(filepath.Join(dir, "netease", "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "netease" {
		t.Fatalf("netease index is %q", got)
	}
	kept, err := os.ReadFile(filepath.Join(dir, "qqmusic", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(kept) != "keep" {
		t.Fatalf("existing qq plugin was replaced with %q", kept)
	}
	if _, err := os.Stat(filepath.Join(dir, "qqmusic", "dist", "index.js")); !os.IsNotExist(err) {
		t.Fatal("an existing plugin had its missing files filled in")
	}
	for _, id := range []string{"bilibili", "youtube-music", "mygotify"} {
		if _, err := os.Stat(filepath.Join(dir, id, "plugin.json")); err != nil {
			t.Errorf("shipped source %s was not installed: %v", id, err)
		}
	}
	nested, err := os.ReadFile(filepath.Join(dir, "bilibili", "locales", "zh-CN.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(nested) != "zh" {
		t.Fatalf("nested file is %q", nested)
	}

	// embed.FS only answers slash paths. The path Windows' filepath.Join would
	// build is not one of them, which is why the lookup stays on path.Join.
	if _, err := fs.ReadFile(bundled, "plugins\\netease\\plugin.json"); err == nil {
		t.Fatal("a backslash path was answered")
	}
	if _, err := fs.ReadFile(bundled, bundledRoot("netease")+"/plugin.json"); err != nil {
		t.Fatal(err)
	}
}

// The app ships NetEase, QQ Music, Bilibili, YouTube Music and Spotify, each
// with the file its manifest names.
func TestShippedMusicSources(t *testing.T) {
	for _, id := range []string{"netease", "qqmusic", "bilibili", "youtube-music", "mygotify"} {
		dir := filepath.Join("..", "..", "plugins", id)
		b, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
		if err != nil {
			t.Errorf("shipped source %s: %v", id, err)
			continue
		}
		var m struct {
			ID   string `json:"id"`
			Main string `json:"main"`
		}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Errorf("%s plugin.json: %v", id, err)
			continue
		}
		if m.ID != id {
			t.Errorf("%s plugin.json id is %q", id, m.ID)
		}
		main := m.Main
		if main == "" {
			main = "index.js"
		}
		if _, err := os.Stat(filepath.Join(dir, main)); err != nil {
			t.Errorf("%s main %s: %v", id, main, err)
		}
	}
}
