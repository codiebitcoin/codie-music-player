// Plugin registry: install plugins from GitHub (preset list or any repo), load and
// supervise their hosts.
package pluginhost

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Preset is a known plugin repo offered in the UI's install list.
type Preset struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Repo   string `json:"repo"` // owner/name
	Branch string `json:"branch"`
	Note   string `json:"note,omitempty"`
}

// Presets: the four launch platforms. Spotify streams "remote" (librespot) so playback
// to codie is not supported in v1 — search/login still work.
var Presets = []Preset{
	{ID: "netease", Name: "网易云音乐", Repo: "hubandad/mygo-music-netease", Branch: "main"},
	{ID: "qqmusic", Name: "QQ 音乐", Repo: "Lorpaves/mygo-music-qqmusic", Branch: "main"},
	{ID: "bilibili", Name: "Bilibili", Repo: "GentleM4dman/mygo-music-bilibili", Branch: "main"},
	{ID: "spotify", Name: "Spotify", Repo: "xingkaixin/mygo-music-spotify", Branch: "main", Note: "remote 播放型插件：登录/搜索可用，推送到 codie 播放暂不支持"},
}

// Registry manages installed plugins and their running hosts.
type Registry struct {
	mu       sync.Mutex
	pluginDir string // <data>/plugins
	dataDir  string // <data>/plugin-data
	hosts    map[string]*Host
	settings func(pluginID, key string) string
}

func NewRegistry(dataDir string, settings func(pluginID, key string) string) (*Registry, error) {
	r := &Registry{
		pluginDir: filepath.Join(dataDir, "plugins"),
		dataDir:   filepath.Join(dataDir, "plugin-data"),
		hosts:     map[string]*Host{},
		settings:  settings,
	}
	if err := os.MkdirAll(r.pluginDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(r.dataDir, 0o700); err != nil {
		return nil, err
	}
	return r, nil
}

// InstallFromGitHub downloads repo@branch as a tarball and extracts plugin.json + main file.
func (r *Registry) InstallFromGitHub(repo, branch string) (*Manifest, error) {
	if repo == "" || branch == "" {
		return nil, fmt.Errorf("repo and branch are required")
	}
	if strings.Contains(repo, "..") || strings.ContainsAny(repo, "/") == false || strings.Count(repo, "/") != 1 {
		return nil, fmt.Errorf("repo must look like owner/name")
	}
	url := fmt.Sprintf("https://codeload.github.com/%s/tar.gz/refs/heads/%s", repo, branch)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", repo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download %s: HTTP %d", repo, resp.StatusCode)
	}
	tmpDir, err := os.MkdirTemp("", "cmp-plugin-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	if err := extractTarGz(resp.Body, tmpDir); err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}
	// tarballs have a top-level <repo>-<sha>/ directory; find the one containing plugin.json
	root := ""
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(tmpDir, e.Name(), "plugin.json")); err == nil {
				root = filepath.Join(tmpDir, e.Name())
				break
			}
		}
	}
	if root == "" {
		return nil, fmt.Errorf("plugin.json not found in %s@%s", repo, branch)
	}
	manifest, err := readManifest(root)
	if err != nil {
		return nil, err
	}
	if manifest.ID == "" || manifest.Main == "" {
		return nil, fmt.Errorf("plugin.json missing id or main")
	}
	dest := filepath.Join(r.pluginDir, manifest.ID)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return nil, err
	}
	// Copy everything except dev/test clutter.
	keep := func(name string) bool {
		base := filepath.Base(name)
		switch base {
		case ".git", "node_modules", "tests", "test", ".github":
			return false
		}
		return true
	}
	if err := copyTree(root, dest, keep); err != nil {
		return nil, err
	}
	manifest.Source = repo + "@" + branch
	if err := saveManifest(dest, manifest); err != nil {
		return nil, err
	}
	return manifest, r.Reload(manifest.ID)
}

func readManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return nil, fmt.Errorf("read plugin.json: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse plugin.json: %w", err)
	}
	return &m, nil
}

func saveManifest(dir string, m *Manifest) error {
	data, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0o600)
}

// LoadAll starts hosts for every installed plugin.
func (r *Registry) LoadAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	entries, _ := os.ReadDir(r.pluginDir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		r.startLocked(e.Name())
	}
}

// Reload (re)starts one plugin host.
func (r *Registry) Reload(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.startLocked(id)
}

func (r *Registry) startLocked(id string) error {
	dir := filepath.Join(r.pluginDir, id)
	manifest, err := readManifest(dir)
	if err != nil {
		return err
	}
	if old, ok := r.hosts[id]; ok {
		old.Stop()
		delete(r.hosts, id)
	}
	mainPath := filepath.Join(dir, manifest.Main)
	mainJS, err := os.ReadFile(mainPath)
	if err != nil {
		return fmt.Errorf("read main %s: %w", manifest.Main, err)
	}
	manifest.Source = manifest.Source
	dataDir := filepath.Join(r.dataDir, id)
	os.MkdirAll(dataDir, 0o700)
	h := NewHost(id, *manifest, dataDir, r.settings)
	r.hosts[id] = h
	go func() {
		if err := h.Start(string(mainJS)); err != nil {
			fmt.Printf("[plugin] %s failed to start: %v\n", id, err)
		}
	}()
	// give the VM a moment to evaluate so callers see loaded=true promptly
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if h.Loaded() || h.Err != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if h.Err != nil {
		return h.Err
	}
	return nil
}

// Uninstall stops and removes a plugin.
func (r *Registry) Uninstall(id string) error {
	r.mu.Lock()
	if h, ok := r.hosts[id]; ok {
		h.Stop()
		delete(r.hosts, id)
	}
	r.mu.Unlock()
	return os.RemoveAll(filepath.Join(r.pluginDir, id))
}

// Get returns a running host.
func (r *Registry) Get(id string) *Host {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hosts[id]
}

// List describes installed plugins (manifest + load state).
func (r *Registry) List() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []map[string]any{}
	entries, _ := os.ReadDir(r.pluginDir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := readManifest(filepath.Join(r.pluginDir, e.Name()))
		if err != nil {
			continue
		}
		item := map[string]any{
			"id": m.ID, "name": m.Name, "version": m.Version, "playback": m.Playback,
			"source": m.Source, "settings": m.Settings,
		}
		if h, ok := r.hosts[m.ID]; ok {
			item["loaded"] = h.Loaded()
			if h.Err != nil {
				item["error"] = h.Err.Error()
			}
		}
		out = append(out, item)
	}
	return out
}

// ---- tar helpers ----

func extractTarGz(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		clean := filepath.Clean(hdr.Name)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			continue
		}
		target := filepath.Join(dest, clean)
		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0o700)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0o700)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, io.LimitReader(tr, 64<<20)); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
}

func copyTree(src, dest string, keep func(string) bool) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if rel == "." {
			return nil
		}
		if !keep(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
}
