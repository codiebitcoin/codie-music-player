// Player pipeline: takes tracks resolved by plugins, downloads their stream, uploads
// to the codie TF card and drives playback (the M0-proven flow).
package player

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"codie-music-play/internal/codie"
	"codie-music-play/internal/pluginhost"
)

// Track is the normalized track shape shared by plugins (mygo-music music.Info subset).
type Track struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Artist   string `json:"artist"`
	Album    string `json:"album,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
	Duration int64  `json:"duration"`
	PluginID string `json:"pluginId"`
}

func (t Track) Key() string { return t.PluginID + ":" + t.ID }

// QueueItem tracks one track through the pipeline.
type QueueItem struct {
	Track     Track  `json:"track"`
	State     string `json:"state"` // pending → resolving → downloading → uploading → ready | failed
	Error     string `json:"error,omitempty"`
	CardPath  string `json:"cardPath,omitempty"` // path on the TF card once uploaded
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	AddedAt   int64  `json:"addedAt"`
}

// Player owns the queue and the device client.
type Player struct {
	mu       sync.Mutex
	items    []*QueueItem
	Current  int // index of the playing item
	client   func() *codie.Client // nil until a device is paired
	cacheDir string
	http     *http.Client
	registry *pluginhost.Registry
	OnUpdate func()
}

func New(cacheDir string, registry *pluginhost.Registry, client func() *codie.Client) (*Player, error) {
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return nil, err
	}
	return &Player{
		cacheDir: cacheDir,
		client:   client,
		registry: registry,
		http:     &http.Client{Timeout: 5 * time.Minute},
		Current:  -1,
	}, nil
}

// Enqueue appends tracks (dedup by plugin+id).
func (p *Player) Enqueue(tracks []Track) []*QueueItem {
	p.mu.Lock()
	defer p.mu.Unlock()
	seen := map[string]bool{}
	for _, it := range p.items {
		seen[it.Track.Key()] = true
	}
	var added []*QueueItem
	for _, t := range tracks {
		if t.ID == "" || seen[t.Key()] {
			continue
		}
		seen[t.Key()] = true
		item := &QueueItem{Track: t, State: "pending", AddedAt: time.Now().UnixMilli()}
		p.items = append(p.items, item)
		added = append(added, item)
	}
	p.notify()
	return added
}

func (p *Player) Remove(index int) {
	p.mu.Lock()
	if index >= 0 && index < len(p.items) {
		p.items = append(p.items[:index], p.items[index+1:]...)
		if p.Current > index {
			p.Current--
		} else if p.Current == index {
			p.Current = -1
		}
	}
	p.mu.Unlock()
	p.notify()
}

func (p *Player) Clear() {
	p.mu.Lock()
	p.items = nil
	p.Current = -1
	p.mu.Unlock()
	p.notify()
}

func (p *Player) Items() []QueueItem {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]QueueItem, 0, len(p.items))
	for _, it := range p.items {
		out = append(out, *it)
	}
	return out
}

func (p *Player) notify() {
	if p.OnUpdate != nil {
		go p.OnUpdate()
	}
}

// resolveStream asks the plugin for a playable stream.
func (p *Player) resolveStream(ctx context.Context, t Track, quality string) (map[string]any, error) {
	h := p.registry.Get(t.PluginID)
	if h == nil {
		return nil, fmt.Errorf("plugin %s not loaded", t.PluginID)
	}
	if h.Manifest.Playback == "remote" {
		return nil, fmt.Errorf("该插件是 remote 播放型（本机播放），暂不支持推送到 codie")
	}
	res, err := h.Call(ctx, "stream", 60*time.Second, map[string]any{"id": t.ID, "quality": quality})
	if err != nil {
		return nil, err
	}
	m, ok := res.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("stream(): unexpected result")
	}
	return m, nil
}

// download fetches the stream URL (with plugin-supplied headers) into the cache dir,
// named by content key so repeated plays skip re-downloading.
func (p *Player) download(ctx context.Context, t Track, stream map[string]any) (string, error) {
	url, _ := stream["url"].(string)
	if url == "" {
		return "", fmt.Errorf("stream(): no url")
	}
	ext := ".mp3"
	if ty, _ := stream["type"].(string); ty != "" {
		e := "." + strings.ToLower(strings.TrimSpace(ty))
		switch e {
		case ".mp3", ".flac", ".ogg", ".m4a", ".aac", ".wav":
			ext = e
		}
	}
	sum := sha256.Sum256([]byte(url))
	name := t.PluginID + "-" + hex.EncodeToString(sum[:12]) + ext
	dest := filepath.Join(p.cacheDir, name)
	if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
		return dest, nil // cached
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	if headers, ok := stream["headers"].(map[string]any); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprintf("%v", v))
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	size, err := io.Copy(f, io.LimitReader(resp.Body, 1<<30)) // hard cap 1 GiB
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	if size == 0 {
		os.Remove(tmp)
		return "", fmt.Errorf("download: empty body")
	}
	if err := os.Rename(tmp, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// Prepare pushes one queue item all the way to the TF card. Idempotent for ready items.
func (p *Player) Prepare(ctx context.Context, index int) (*QueueItem, error) {
	p.mu.Lock()
	if index < 0 || index >= len(p.items) {
		p.mu.Unlock()
		return nil, fmt.Errorf("queue index out of range")
	}
	item := p.items[index]
	p.mu.Unlock()

	if item.State == "ready" && item.CardPath != "" {
		return item, nil
	}
	cl := p.client()
	if cl == nil {
		return nil, fmt.Errorf("未连接 codie 设备")
	}
	st, err := cl.MusicStorage()
	if err != nil {
		return nil, err
	}
	if !st.IsLoaded {
		return nil, fmt.Errorf("设备 TF 卡未就绪")
	}

	setState := func(s, e string) {
		p.mu.Lock()
		item.State, item.Error = s, e
		p.mu.Unlock()
		p.notify()
	}
	setState("resolving", "")
	stream, err := p.resolveStream(ctx, item.Track, "standard")
	if err != nil {
		setState("failed", err.Error())
		return item, err
	}
	setState("downloading", "")
	local, err := p.download(ctx, item.Track, stream)
	if err != nil {
		setState("failed", err.Error())
		return item, err
	}
	if info, err := os.Stat(local); err == nil {
		p.mu.Lock()
		item.SizeBytes = info.Size()
		p.mu.Unlock()
	}
	setState("uploading", "")
	cardPath, err := cl.UploadFile(local)
	if err != nil {
		setState("failed", err.Error())
		return item, err
	}
	p.mu.Lock()
	item.CardPath = cardPath
	item.State = "ready"
	p.mu.Unlock()
	p.notify()
	return item, nil
}

// PlayNow prepares the item (if needed) and starts playback on the device.
func (p *Player) PlayNow(ctx context.Context, index int) error {
	item, err := p.Prepare(ctx, index)
	if err != nil {
		return err
	}
	cl := p.client()
	if cl == nil {
		return fmt.Errorf("未连接 codie 设备")
	}
	if err := cl.EnsureSource(); err != nil {
		return fmt.Errorf("切换音源失败（设备需打开音乐页）: %w", err)
	}
	p.mu.Lock()
	p.Current = index
	title := item.Track.Artist + " - " + item.Track.Name
	p.mu.Unlock()
	if err := cl.PlayCardPaths(cardRootOf(cl), []string{item.CardPath}, title); err != nil {
		return err
	}
	p.notify()
	go p.watchAdvance()
	return nil
}

// PlayAll prepares everything (sequentially) then plays the queue as one device queue.
func (p *Player) PlayAll(ctx context.Context) error {
	cl := p.client()
	if cl == nil {
		return fmt.Errorf("未连接 codie 设备")
	}
	p.mu.Lock()
	n := len(p.items)
	p.mu.Unlock()
	if n == 0 {
		return fmt.Errorf("队列是空的")
	}
	paths := make([]string, 0, n)
	for i := 0; i < n; i++ {
		item, err := p.Prepare(ctx, i)
		if err != nil {
			continue // skip failures
		}
		paths = append(paths, item.CardPath)
	}
	if len(paths) == 0 {
		return fmt.Errorf("没有可播放的曲目（全部准备失败）")
	}
	if err := cl.EnsureSource(); err != nil {
		return fmt.Errorf("切换音源失败（设备需打开音乐页）: %w", err)
	}
	p.mu.Lock()
	p.Current = 0
	p.mu.Unlock()
	if err := cl.PlayCardPaths(cardRootOf(cl), paths, "codie-music-play"); err != nil {
		return err
	}
	p.notify()
	go p.watchAdvance()
	return nil
}

// watchAdvance follows device playback to keep the queue cursor in sync
// (auto_advance on the device plays the uploaded queue in order).
func (p *Player) watchAdvance() {
	deadline := time.Now().Add(3 * time.Hour)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		cl := p.client()
		if cl == nil {
			return
		}
		st, err := cl.Playback()
		if err != nil || st.TrackID == "" {
			continue
		}
		p.mu.Lock()
		for i, it := range p.items {
			if it.CardPath != "" && "local:"+it.CardPath == st.TrackID {
				if p.Current != i {
					p.Current = i
					p.notify()
				}
				break
			}
		}
		p.mu.Unlock()
	}
}

func cardRootOf(cl *codie.Client) string {
	if st, err := cl.MusicStorage(); err == nil && st.IsLoaded {
		return st.RootPath
	}
	return ""
}

// MarshalSnapshot for the API.
func (p *Player) Snapshot() map[string]any {
	return map[string]any{"items": p.Items(), "current": p.Current}
}

var _ = json.Marshal
