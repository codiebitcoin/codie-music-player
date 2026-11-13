// REST API for the frontend: devices (pairing), plugins (install/search/login),
// queue/playback, settings. Served on the same origin as the embedded SPA.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"codie-music-play/internal/codie"
	"codie-music-play/internal/player"
	"codie-music-play/internal/pluginhost"
	"codie-music-play/internal/store"

	qrcode "github.com/skip2/go-qrcode"
)

// Device persisted in devices.json.
type Device struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Addr   string       `json:"addr"`
	Creds  *codie.Creds `json:"creds,omitempty"`
	Active bool         `json:"active"`
}

// App wires every subsystem together.
type App struct {
	mu      sync.Mutex
	st      *store.Store
	Reg     *pluginhost.Registry
	Player  *player.Player
	devices map[string]*Device

	pendingMu sync.Mutex
	pending   map[string]string // deviceID -> challengeID

	settingsMu sync.Mutex
	settings   map[string]map[string]string // pluginID -> key -> value
}

func New(st *store.Store, reg *pluginhost.Registry, pl *player.Player) (*App, error) {
	a := &App{st: st, Reg: reg, Player: pl, devices: map[string]*Device{}, pending: map[string]string{},
		settings: map[string]map[string]string{}}
	if err := st.Load("devices", &a.devices); a.devices == nil {
		a.devices = map[string]*Device{}
	} else if err != nil {
		return nil, err
	}
	_ = st.Load("settings", &a.settings)
	if a.settings == nil {
		a.settings = map[string]map[string]string{}
	}
	return a, nil
}

func (a *App) settingFn() func(pluginID, key string) string {
	return func(pluginID, key string) string {
		a.settingsMu.Lock()
		defer a.settingsMu.Unlock()
		if m, ok := a.settings[pluginID]; ok {
			return m[key]
		}
		return ""
	}
}

// Setting reads a plugin setting override (exported for main's wiring).
func (a *App) Setting(pluginID, key string) string { return a.settingFn()(pluginID, key) }

// ActiveClient returns the paired, active device client (or nil).
func (a *App) ActiveClient() *codie.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, d := range a.devices {
		if d.Active && d.Creds != nil {
			return codie.New(d.Creds)
		}
	}
	// fall back to any paired device
	for _, d := range a.devices {
		if d.Creds != nil {
			return codie.New(d.Creds)
		}
	}
	return nil
}

func (a *App) saveDevices() {
	a.st.Save("devices", a.devices)
}

// ---- HTTP plumbing ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"success": false, "message": err.Error()})
}

func ok(w http.ResponseWriter, data any) {
	writeJSON(w, 200, map[string]any{"success": true, "data": data})
}

// Handler returns the /api routes.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()

	// ---- devices ----
	mux.HandleFunc("GET /api/devices", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		list := make([]map[string]any, 0, len(a.devices))
		for _, d := range a.devices {
			list = append(list, map[string]any{
				"id": d.ID, "name": d.Name, "addr": d.Addr, "paired": d.Creds != nil, "active": d.Active,
			})
		}
		a.mu.Unlock()
		sort.Slice(list, func(i, j int) bool { return list[i]["id"].(string) < list[j]["id"].(string) })
		ok(w, list)
	})
	mux.HandleFunc("POST /api/devices", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name, Addr string
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Addr) == "" {
			fail(w, 400, fmt.Errorf("name and addr required"))
			return
		}
		addr := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(req.Addr), "http://"), "/")
		c := codie.New(&codie.Creds{Addr: addr})
		challengeID, err := c.RequestPairing()
		if err != nil {
			fail(w, 502, fmt.Errorf("配对请求失败（设备在线吗？）: %v", err))
			return
		}
		id := fmt.Sprintf("d%d", time.Now().UnixNano())
		a.mu.Lock()
		a.devices[id] = &Device{ID: id, Name: strings.TrimSpace(req.Name), Addr: addr}
		a.saveDevices()
		a.mu.Unlock()
		a.pendingMu.Lock()
		a.pending[id] = challengeID
		a.pendingMu.Unlock()
		ok(w, map[string]any{"id": id, "challenge_id": challengeID, "hint": "看设备屏幕上的 6 位配对码"})
	})
	mux.HandleFunc("POST /api/devices/{id}/confirm", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req struct {
			Code string
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Code) == "" {
			fail(w, 400, fmt.Errorf("code required"))
			return
		}
		a.pendingMu.Lock()
		challengeID := a.pending[id]
		a.pendingMu.Unlock()
		if challengeID == "" {
			fail(w, 409, fmt.Errorf("没有进行中的配对，先添加设备"))
			return
		}
		a.mu.Lock()
		d := a.devices[id]
		a.mu.Unlock()
		if d == nil {
			fail(w, 404, fmt.Errorf("device not found"))
			return
		}
		c := codie.New(&codie.Creds{Addr: d.Addr})
		creds, err := c.ConfirmPairing(challengeID, strings.TrimSpace(req.Code), "codie-music-play")
		if err != nil {
			fail(w, 401, err)
			return
		}
		a.pendingMu.Lock()
		delete(a.pending, id)
		a.pendingMu.Unlock()
		a.mu.Lock()
		d.Creds = creds
		for _, other := range a.devices {
			other.Active = other.ID == id
		}
		a.saveDevices()
		a.mu.Unlock()
		ok(w, map[string]any{"id": id, "name": creds.Name, "kind": creds.Kind})
	})
	mux.HandleFunc("GET /api/devices/status", func(w http.ResponseWriter, r *http.Request) {
		cl := a.ActiveClient()
		if cl == nil {
			ok(w, map[string]any{"connected": false, "reason": "未配对设备"})
			return
		}
		st, err := cl.MusicStorage()
		if err != nil {
			ok(w, map[string]any{"connected": false, "reason": err.Error()})
			return
		}
		pb, _ := cl.Playback()
		ok(w, map[string]any{
			"connected": true, "tf_loaded": st.IsLoaded, "tf_root": st.RootPath,
			"tf_total": st.CapacityT, "tf_used": st.CapacityU, "playback": pb,
		})
	})
	mux.HandleFunc("DELETE /api/devices/{id}", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		delete(a.devices, r.PathValue("id"))
		a.saveDevices()
		a.mu.Unlock()
		ok(w, true)
	})

	// ---- plugins ----
	mux.HandleFunc("GET /api/plugins", func(w http.ResponseWriter, r *http.Request) {
		installed := map[string]bool{}
		for _, item := range a.Reg.List() {
			if id, _ := item["id"].(string); id != "" {
				installed[id] = true
			}
		}
		presets := []map[string]any{}
		for _, p := range pluginhost.Presets {
			presets = append(presets, map[string]any{
				"id": p.ID, "name": p.Name, "repo": p.Repo, "branch": p.Branch, "note": p.Note,
				"installed": installed[p.ID],
			})
		}
		ok(w, map[string]any{"plugins": a.Reg.List(), "presets": presets})
	})
	mux.HandleFunc("POST /api/plugins/install", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Repo, Branch, ID string
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, 400, err)
			return
		}
		if req.Repo == "" { // preset install by id
			for _, p := range pluginhost.Presets {
				if p.ID == req.ID {
					req.Repo, req.Branch = p.Repo, p.Branch
				}
			}
		}
		if req.Branch == "" {
			req.Branch = "main"
		}
		m, err := a.Reg.InstallFromGitHub(req.Repo, req.Branch)
		if err != nil {
			fail(w, 502, err)
			return
		}
		ok(w, m)
	})
	mux.HandleFunc("DELETE /api/plugins/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Reg.Uninstall(r.PathValue("id")); err != nil {
			fail(w, 500, err)
			return
		}
		ok(w, true)
	})
	mux.HandleFunc("GET /api/plugins/{id}/account", func(w http.ResponseWriter, r *http.Request) {
		h := a.Reg.Get(r.PathValue("id"))
		if h == nil || !h.Loaded() {
			fail(w, 404, fmt.Errorf("plugin not loaded"))
			return
		}
		if !hasExport(h, "account") {
			ok(w, map[string]any{"supported": false})
			return
		}
		acc, err := h.Call(r.Context(), "account", 30*time.Second)
		if err != nil {
			ok(w, map[string]any{"supported": true, "account": nil, "error": err.Error()})
			return
		}
		ok(w, map[string]any{"supported": true, "account": acc})
	})
	mux.HandleFunc("POST /api/plugins/{id}/signin", func(w http.ResponseWriter, r *http.Request) {
		h := a.Reg.Get(r.PathValue("id"))
		if h == nil || !h.Loaded() {
			fail(w, 404, fmt.Errorf("plugin not loaded"))
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			_, _ = h.Call(ctx, "signIn", 5*time.Minute)
		}()
		ok(w, map[string]any{"started": true})
	})
	mux.HandleFunc("GET /api/plugins/{id}/signin/prompt", func(w http.ResponseWriter, r *http.Request) {
		h := a.Reg.Get(r.PathValue("id"))
		if h == nil {
			fail(w, 404, fmt.Errorf("plugin not loaded"))
			return
		}
		ok(w, h.Prompt())
	})
	mux.HandleFunc("GET /api/plugins/{id}/signin/qr.png", func(w http.ResponseWriter, r *http.Request) {
		h := a.Reg.Get(r.PathValue("id"))
		if h == nil {
			http.NotFound(w, r)
			return
		}
		p := h.Prompt()
		if p.QR == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		png, err := qrcode.Encode(p.QR, qrcode.Medium, 320)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(png)
	})
	mux.HandleFunc("POST /api/plugins/{id}/signout", func(w http.ResponseWriter, r *http.Request) {
		h := a.Reg.Get(r.PathValue("id"))
		if h == nil || !h.Loaded() {
			fail(w, 404, fmt.Errorf("plugin not loaded"))
			return
		}
		if _, err := h.Call(r.Context(), "signOut", 30*time.Second); err != nil {
			fail(w, 500, err)
			return
		}
		ok(w, true)
	})

	// ---- search / stream ----
	mux.HandleFunc("GET /api/plugins/{id}/search", func(w http.ResponseWriter, r *http.Request) {
		h := a.Reg.Get(r.PathValue("id"))
		if h == nil || !h.Loaded() {
			fail(w, 404, fmt.Errorf("plugin not loaded（先安装并启动）"))
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			fail(w, 400, fmt.Errorf("q required"))
			return
		}
		kind := r.URL.Query().Get("kind")
		if kind == "" {
			kind = "tracks"
		}
		res, err := h.Call(r.Context(), "search", 60*time.Second, map[string]any{"query": q, "kind": kind, "offset": 0})
		if err != nil {
			fail(w, 500, err)
			return
		}
		ok(w, normalizeSearch(res, h.ID))
	})
	mux.HandleFunc("GET /api/plugins/{id}/lyrics", func(w http.ResponseWriter, r *http.Request) {
		h := a.Reg.Get(r.PathValue("id"))
		if h == nil || !h.Loaded() || !hasExport(h, "lyrics") {
			fail(w, 404, fmt.Errorf("not available"))
			return
		}
		res, err := h.Call(r.Context(), "lyrics", 60*time.Second, map[string]any{"id": r.URL.Query().Get("id")})
		if err != nil {
			fail(w, 500, err)
			return
		}
		ok(w, res)
	})

	// ---- queue & playback ----
	mux.HandleFunc("GET /api/queue", func(w http.ResponseWriter, r *http.Request) {
		ok(w, a.Player.Snapshot())
	})
	mux.HandleFunc("POST /api/queue", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Tracks []player.Track
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, 400, err)
			return
		}
		added := a.Player.Enqueue(req.Tracks)
		ok(w, map[string]any{"added": len(added)})
	})
	mux.HandleFunc("DELETE /api/queue/{index}", func(w http.ResponseWriter, r *http.Request) {
		var idx int
		fmt.Sscanf(r.PathValue("index"), "%d", &idx)
		a.Player.Remove(idx)
		ok(w, true)
	})
	mux.HandleFunc("POST /api/queue/clear", func(w http.ResponseWriter, r *http.Request) {
		a.Player.Clear()
		ok(w, true)
	})
	mux.HandleFunc("POST /api/queue/play", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Index *int
		}
		json.NewDecoder(r.Body).Decode(&req)
		var err error
		if req.Index != nil {
			err = a.Player.PlayNow(r.Context(), *req.Index)
		} else {
			err = a.Player.PlayAll(r.Context())
		}
		if err != nil {
			fail(w, 500, err)
			return
		}
		ok(w, true)
	})
	mux.HandleFunc("POST /api/playback/control", func(w http.ResponseWriter, r *http.Request) {
		cl := a.ActiveClient()
		if cl == nil {
			fail(w, 409, fmt.Errorf("未连接设备"))
			return
		}
		var req struct {
			Action     string
			PositionMs *int64 `json:"position_ms"`
			Volume     *int32 `json:"volume_level"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Action == "" {
			fail(w, 400, fmt.Errorf("action required"))
			return
		}
		extra := map[string]any{}
		if req.PositionMs != nil {
			extra["progress_ms"] = *req.PositionMs
		}
		if req.Volume != nil {
			extra["volume_level"] = *req.Volume
		}
		if err := cl.Control(req.Action, extra); err != nil {
			fail(w, 502, err)
			return
		}
		ok(w, true)
	})
	mux.HandleFunc("GET /api/playback", func(w http.ResponseWriter, r *http.Request) {
		cl := a.ActiveClient()
		if cl == nil {
			fail(w, 409, fmt.Errorf("未连接设备"))
			return
		}
		pb, err := cl.Playback()
		if err != nil {
			fail(w, 502, err)
			return
		}
		ok(w, pb)
	})

	// ---- settings ----
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		a.settingsMu.Lock()
		defer a.settingsMu.Unlock()
		ok(w, a.settings)
	})
	mux.HandleFunc("POST /api/settings/{plugin}", func(w http.ResponseWriter, r *http.Request) {
		var m map[string]string
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			fail(w, 400, err)
			return
		}
		plugin := r.PathValue("plugin")
		a.settingsMu.Lock()
		if a.settings[plugin] == nil {
			a.settings[plugin] = map[string]string{}
		}
		for k, v := range m {
			a.settings[plugin][k] = v
		}
		a.settingsMu.Unlock()
		a.st.Save("settings", a.settings)
		// settings can change plugin behaviour (e.g. login method) — reload
		a.Reg.Reload(plugin)
		ok(w, true)
	})

	return mux
}

func hasExport(h *pluginhost.Host, name string) bool {
	for _, e := range h.Exports() {
		if e == name {
			return true
		}
	}
	return false
}

// normalizeSearch maps plugin track objects to our Track shape.
func normalizeSearch(res any, pluginID string) map[string]any {
	m, ok := res.(map[string]any)
	if !ok {
		return map[string]any{"tracks": []player.Track{}}
	}
	raw, _ := m["tracks"].([]any)
	tracks := make([]player.Track, 0, len(raw))
	for _, it := range raw {
		t, ok := it.(map[string]any)
		if !ok {
			continue
		}
		tracks = append(tracks, player.Track{
			ID:       toS(t["id"]),
			Name:     toS(t["name"]),
			Artist:   joinArtists(t["artists"]),
			Album:    albumName(t["album"]),
			ImageURL: toS(t["imageUrl"]),
			Duration: toI(t["duration"]),
			PluginID: pluginID,
		})
	}
	more := false
	if mv, ok := m["more"].(bool); ok {
		more = mv
	}
	return map[string]any{"tracks": tracks, "more": more}
}

func joinArtists(v any) string {
	list, ok := v.([]any)
	if !ok {
		return toS(v)
	}
	names := make([]string, 0, len(list))
	for _, a := range list {
		if am, ok := a.(map[string]any); ok {
			names = append(names, toS(am["name"]))
		}
	}
	return strings.Join(names, " / ")
}

func albumName(v any) string {
	if am, ok := v.(map[string]any); ok {
		return toS(am["name"])
	}
	return ""
}

func toS(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", s)
	}
}

func toI(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	default:
		return 0
	}
}
