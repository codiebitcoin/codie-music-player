// codie-music-play: play NetEase / QQ / Spotify / Bilibili music on a codie device.
//
// Run modes (same as codie-life-connect):
//   - service mode (default): web UI + REST API at http://127.0.0.1:8808
//   - desktop mode (-desktop / CMP_DESKTOP=1): a MyGo desktop window loading the same UI.
package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"codie-music-play/internal/api"
	"codie-music-play/internal/codie"
	"codie-music-play/internal/player"
	"codie-music-play/internal/pluginhost"
	"codie-music-play/internal/store"

	"github.com/egoist/mygo"
)

//go:embed all:front/dist
var webFS embed.FS

var version = "dev"

func main() {
	defaultAddr := "127.0.0.1:8808"
	if os.Getenv("CMP_DEV") == "1" {
		defaultAddr = ":8808" // dev run: reachable from other hosts for tooling
	}
	var (
		addr      = flag.String("addr", envOr("LISTEN_ADDR", defaultAddr), "HTTP listen address")
		dataDir   = flag.String("data", "", "data directory")
		desktop   = flag.Bool("desktop", os.Getenv("CMP_DESKTOP") == "1", "run in desktop-window mode (MyGo)")
		showVer   = flag.Bool("version", false, "print version")
	)
	flag.Parse()
	if *showVer {
		log.Printf("codie-music-play %s (%s/%s)", version, runtime.GOOS, runtime.GOARCH)
		return
	}

	if *dataDir == "" {
		if *desktop {
			*dataDir = desktopDataDir()
		} else {
			*dataDir = envOr("DATA_DIR", "data")
		}
	}

	st, err := store.New(*dataDir)
	if err != nil {
		log.Fatalf("data dir: %v", err)
	}

	// App wiring
	var app *api.App
	settingsFn := func(pluginID, key string) string { return app.Setting(pluginID, key) }
	reg, err := pluginhost.NewRegistry(*dataDir, settingsFn)
	if err != nil {
		log.Fatalf("plugin registry: %v", err)
	}
	pl, err := player.New(filepath.Join(*dataDir, "cache"), reg, func() *codie.Client { return app.ActiveClient() })
	if err != nil {
		log.Fatalf("player: %v", err)
	}
	app, err = api.New(st, reg, pl)
	if err != nil {
		log.Fatalf("app: %v", err)
	}
	reg.LoadAll()

	// HTTP server: API + embedded SPA
	mux := http.NewServeMux()
	mux.Handle("/api/", app.Handler())
	dist, err := fs.Sub(webFS, "front/dist")
	if err != nil {
		log.Fatalf("embed: %v", err)
	}
	mux.Handle("/", spaHandler(dist))

	listen := *addr
	go func() {
		log.Printf("codie-music-play %s listening on http://%s (data: %s)", version, listen, *dataDir)
		if err := http.ListenAndServe(listen, mux); err != nil {
			log.Fatalf("http: %v", err)
		}
	}()

	if *desktop {
		mygo.App.WhenReady(func() {
			mygo.NewWindow(mygo.WindowOptions{
				Title:    "Codie Music Play",
				URL:      "http://" + listen,
				Width:    1180,
				Height:   800,
				MinWidth: 880,
				MinHeight: 600,
				StateKey: "main",
			})
		})
		if err := mygo.App.Run(); err != nil {
			log.Fatalf("desktop: %v", err)
		}
		return
	}
	select {} // service mode: serve forever
}

func spaHandler(dist fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(dist, path); err != nil {
			// SPA fallback
			data, _ := fs.ReadFile(dist, "index.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func desktopDataDir() string {
	switch runtime.GOOS {
	case "darwin":
		dir, _ := os.UserHomeDir()
		return filepath.Join(dir, "Library", "Application Support", "CodieMusicPlay")
	case "windows":
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "CodieMusicPlay")
		}
	}
	dir, _ := os.UserHomeDir()
	return filepath.Join(dir, ".codie-music-play")
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
