// Package web is the browser face of the player: the same sources the
// desktop app knows, over HTTP — browse, search, and play in the page. It
// borrows the family's server shape (codie-video-player): a plain net/http
// mux, a cookie login, the web app embedded and served as files.
package web

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"mygo-music/internal/local"
	"mygo-music/internal/music"
	"mygo-music/internal/plugin"
)

// Options is how the server is built.
type Options struct {
	DataDir    string // settings and the plugins' data
	PluginsDir string // plugin folders
	Version    string
	AuthUser   string // empty user or pass makes a random pair, logged once
	AuthPass   string
	Static     fs.FS // the built web app, at /
}

// source is one place music comes from.
type source struct {
	id   string
	name string
	src  music.Source
	p    *plugin.Plugin // nil for the built-in local files
}

// Server is the whole web app.
type Server struct {
	dataDir string
	version string
	sources []source
	local   *local.Source
	auth    *auth
	static  fs.FS
}

// New assembles the server: the built-in source, the plugins of the folder,
// and the rest. It doesn't listen yet.
func New(opt Options) (*Server, error) {
	s := &Server{
		dataDir: opt.DataDir,
		version: opt.Version,
		auth:    newAuth(opt.AuthUser, opt.AuthPass),
		static:  opt.Static,
	}
	// Plugins talk like the desktop's. Signing in opens a page the server
	// can't show, so a source that needs one says so.
	plugin.SetOpenURL(func(string) error { return errors.New("the web app can't open pages") })
	plugin.SetAppVersion(opt.Version)
	plugin.SetWebSignIn(func(plugin.WebSignIn) (string, error) {
		return "", errors.New("sign in to this source in the desktop app; the web app has no window for it")
	})
	music.ImageHeaders = s.imageHeaders

	s.local = local.New(filepath.Join(opt.DataDir, "local"))
	s.sources = append(s.sources, source{id: s.local.Info().ID, name: s.local.Info().Name, src: s.local})

	dataDir := filepath.Join(opt.DataDir, "plugin-data")
	for _, p := range plugin.Discover(opt.PluginsDir, dataDir) {
		// A server runner has already said yes to what plugins run; the
		// desktop's grant banner has no page here.
		if len(p.Permissions().Process) > 0 && !p.Granted() {
			if err := p.Grant(true); err != nil {
				log.Printf("web: grant %s: %v", p.Info().ID, err)
			} else {
				log.Printf("web: granted process permission to %s", p.Info().ID)
			}
		}
		info := p.Info()
		s.sources = append(s.sources, source{id: info.ID, name: info.Name, src: p, p: p})
	}
	return s, nil
}

// Handler is everything the server answers, in one handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		s.writeJSON(w, map[string]any{"ok": true, "version": s.version})
	})
	mux.HandleFunc("POST /api/login", s.auth.handleLogin)
	mux.HandleFunc("POST /api/logout", s.auth.handleLogout)

	api := http.NewServeMux()
	api.HandleFunc("GET /api/sources", s.handleSources)
	api.HandleFunc("GET /api/source/{id}/home", s.handleHome)
	api.HandleFunc("GET /api/source/{id}/browse", s.handleBrowse)
	api.HandleFunc("GET /api/source/{id}/playlists", s.handlePlaylists)
	api.HandleFunc("GET /api/source/{id}/playlist", s.handlePlaylist)
	api.HandleFunc("GET /api/source/{id}/album", s.handleAlbum)
	api.HandleFunc("GET /api/source/{id}/artist", s.handleArtist)
	api.HandleFunc("GET /api/source/{id}/liked", s.handleLiked)
	api.HandleFunc("GET /api/source/{id}/recent", s.handleRecent)
	api.HandleFunc("GET /api/source/{id}/stations", s.handleStations)
	api.HandleFunc("GET /api/source/{id}/station", s.handleStation)
	api.HandleFunc("GET /api/source/{id}/account", s.handleAccount)
	api.HandleFunc("GET /api/search", s.handleSearch)
	api.HandleFunc("GET /api/suggest", s.handleSuggest)
	api.HandleFunc("GET /api/lyrics", s.handleLyrics)
	api.HandleFunc("GET /api/stream", s.handleStream)
	api.HandleFunc("GET /api/cover", s.handleCover)
	api.HandleFunc("GET /api/folders", s.handleFolders)
	api.HandleFunc("POST /api/folders", s.handleAddFolder)
	api.HandleFunc("DELETE /api/folders", s.handleRemoveFolder)
	mux.Handle("/api/", s.auth.guard(api))
	// The web app's shell is public — it has nothing in it; the viewer's
	// data lives behind the API, and the page itself shows the login when
	// an ask comes back 401.
	mux.Handle("/", s.staticHandler())
	return mux
}

// staticHandler serves the built web app, falling back to its index for the
// paths the app routes itself.
func (s *Server) staticHandler() http.Handler {
	file := http.FileServer(http.FS(s.static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.static, p); err != nil {
			r.URL.Path = "/"
		}
		file.ServeHTTP(w, r)
	})
}

// ---- the sources' API ----

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	out := make([]map[string]any, 0, len(s.sources))
	for _, so := range s.sources {
		info := so.src.Info()
		out = append(out, map[string]any{
			"id":         info.ID,
			"name":       info.Name,
			"navigation": info.Navigation,
			"can":        s.abilities(so.src),
		})
	}
	s.writeJSON(w, map[string]any{"sources": out})
}

// abilities are the things the views ask about before showing them.
var abilities = []string{"search", "suggest", "home", "browse", "playlists", "liked", "recent", "stations", "lyrics", "like"}

func (s *Server) abilities(src music.Source) []string {
	var out []string
	for _, what := range abilities {
		if src.Can(what) {
			out = append(out, what)
		}
	}
	return out
}

func (s *Server) sourceOf(id string) *source {
	for i := range s.sources {
		if s.sources[i].id == id {
			return &s.sources[i]
		}
	}
	return nil
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	so := s.needSource(w, r)
	if so == nil {
		return
	}
	b, err := so.src.Home()
	s.answer(w, b, err)
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	so := s.needSource(w, r)
	if so == nil {
		return
	}
	b, err := so.src.Browse(r.URL.Query().Get("page"))
	s.answer(w, b, err)
}

func (s *Server) handlePlaylists(w http.ResponseWriter, r *http.Request) {
	so := s.needSource(w, r)
	if so == nil {
		return
	}
	ps, err := so.src.Playlists()
	s.answer(w, ps, err)
}

func (s *Server) handlePlaylist(w http.ResponseWriter, r *http.Request) {
	so := s.needSource(w, r)
	if so == nil {
		return
	}
	p, err := so.src.Playlist(r.URL.Query().Get("id"))
	s.answer(w, p, err)
}

func (s *Server) handleAlbum(w http.ResponseWriter, r *http.Request) {
	so := s.needSource(w, r)
	if so == nil {
		return
	}
	p, err := so.src.Album(r.URL.Query().Get("id"))
	s.answer(w, p, err)
}

func (s *Server) handleArtist(w http.ResponseWriter, r *http.Request) {
	so := s.needSource(w, r)
	if so == nil {
		return
	}
	p, err := so.src.Artist(r.URL.Query().Get("id"))
	s.answer(w, p, err)
}

func (s *Server) handleLiked(w http.ResponseWriter, r *http.Request) {
	so := s.needSource(w, r)
	if so == nil {
		return
	}
	p, err := so.src.Liked(queryInt(r, "offset"))
	s.answer(w, p, err)
}

func (s *Server) handleRecent(w http.ResponseWriter, r *http.Request) {
	so := s.needSource(w, r)
	if so == nil {
		return
	}
	tracks, err := so.src.Recent()
	s.answer(w, tracks, err)
}

func (s *Server) handleStations(w http.ResponseWriter, r *http.Request) {
	so := s.needSource(w, r)
	if so == nil {
		return
	}
	st, err := so.src.Stations()
	s.answer(w, st, err)
}

func (s *Server) handleStation(w http.ResponseWriter, r *http.Request) {
	so := s.needSource(w, r)
	if so == nil {
		return
	}
	tracks, err := so.src.StationNext(r.URL.Query().Get("id"))
	s.answer(w, tracks, err)
}

func (s *Server) handleAccount(w http.ResponseWriter, r *http.Request) {
	so := s.needSource(w, r)
	if so == nil {
		return
	}
	a, err := so.src.Account()
	s.answer(w, a, err)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	so := s.sourceOf(r.URL.Query().Get("src"))
	if so == nil {
		http.Error(w, `{"error":"no such source"}`, http.StatusNotFound)
		return
	}
	res, err := so.src.Search(r.URL.Query().Get("q"), music.SearchKind(r.URL.Query().Get("kind")), queryInt(r, "offset"))
	s.answer(w, res, err)
}

func (s *Server) handleSuggest(w http.ResponseWriter, r *http.Request) {
	so := s.sourceOf(r.URL.Query().Get("src"))
	if so == nil {
		http.Error(w, `{"error":"no such source"}`, http.StatusNotFound)
		return
	}
	out, err := so.src.Suggest(r.URL.Query().Get("q"))
	s.answer(w, out, err)
}

func (s *Server) handleLyrics(w http.ResponseWriter, r *http.Request) {
	so := s.sourceOf(r.URL.Query().Get("src"))
	if so == nil {
		http.Error(w, `{"error":"no such source"}`, http.StatusNotFound)
		return
	}
	l, err := so.src.Lyrics(r.URL.Query().Get("id"))
	s.answer(w, l, err)
}

// handleStream answers what to play a track with: the address, and the
// headers its source asks to send with it. A file of this machine is served
// from here, so the page never asks for one by path.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	so := s.sourceOf(r.URL.Query().Get("src"))
	if so == nil {
		http.Error(w, `{"error":"no such source"}`, http.StatusNotFound)
		return
	}
	st, err := so.src.Stream(r.URL.Query().Get("id"), r.URL.Query().Get("quality"))
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if path, ok := music.PathOf(st.URL); ok {
		st.URL = "/api/cover?src=" + url.QueryEscape(so.id) + "&file=" + url.QueryEscape(path)
	}
	s.writeJSON(w, st)
}

// handleCover fetches a picture, or a local file, for the page: covers sit
// behind a source's own session, which the browser can't send.
func (s *Server) handleCover(w http.ResponseWriter, r *http.Request) {
	so := s.sourceOf(r.URL.Query().Get("src"))
	if file := r.URL.Query().Get("file"); file != "" {
		http.ServeFile(w, r, file)
		return
	}
	u := r.URL.Query().Get("u")
	if u == "" {
		http.Error(w, "no picture", http.StatusBadRequest)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), "GET", music.SizedImage(u, 600), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Header.Set("User-Agent", music.Agent)
	var headers map[string]string
	if so != nil && so.p != nil {
		headers = so.p.HeadersFor(req.URL, plugin.KindImage)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := music.HeadersStay(http.DefaultClient, req.URL.Hostname(), headers).Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		http.Error(w, res.Status, http.StatusBadGateway)
		return
	}
	if ct := res.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	io.Copy(w, io.LimitReader(res.Body, 64<<20))
}

func (s *Server) handleFolders(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, map[string]any{"folders": s.local.Folders(), "tracks": s.local.TrackCount()})
}

func (s *Server) handleAddFolder(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil || strings.TrimSpace(q.Path) == "" {
		http.Error(w, `{"error":"a path is needed"}`, http.StatusBadRequest)
		return
	}
	s.local.AddFolders(q.Path)
	s.writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleRemoveFolder(w http.ResponseWriter, r *http.Request) {
	s.local.RemoveFolder(r.URL.Query().Get("path"))
	s.writeJSON(w, map[string]any{"ok": true})
}

// ---- helpers ----

func (s *Server) needSource(w http.ResponseWriter, r *http.Request) *source {
	so := s.sourceOf(r.PathValue("id"))
	if so == nil {
		http.Error(w, `{"error":"no such source"}`, http.StatusNotFound)
	}
	return so
}

// answer writes v, or the source's complaint.
func (s *Server) answer(w http.ResponseWriter, v any, err error) {
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, v)
}

func (s *Server) writeErr(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	switch {
	case errors.Is(err, music.ErrUnsupported):
		status = http.StatusNotFound
	case errors.Is(err, music.ErrSignIn), errors.Is(err, music.ErrSetup):
		status = http.StatusConflict
	case errors.Is(err, music.ErrUnavailable):
		status = http.StatusGone
	}
	msg := err.Error()
	if reason := music.Reason(err); reason != "" {
		msg = reason
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *Server) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func queryInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(name))
	return n
}

// imageHeaders is what music.Fetch sends for a cover: the headers of the
// plugin whose hosts the picture is of.
func (s *Server) imageHeaders(u *url.URL) map[string]string {
	for i := range s.sources {
		if p := s.sources[i].p; p != nil {
			if h := p.HeadersFor(u, plugin.KindImage); len(h) > 0 {
				return h
			}
		}
	}
	return nil
}
