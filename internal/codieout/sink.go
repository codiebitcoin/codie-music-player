package codieout

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// Sink plays a stream source's tracks on a codie device. It wraps the source
// and answers as one of PlaybackRemote, so the player drives it with the very
// machinery it uses for Spotify Connect: it only tells it what to do, and asks
// how it's going, while it plays. Every catalog question goes to the wrapped
// source unchanged.
type Sink struct {
	music.Source // the wrapped stream source (netease, qq music, …)

	client   *Client
	cacheDir string
	quality  func() string // the app's quality setting

	mu      sync.Mutex
	byCard  map[string]string // card path -> original track id, of what was uploaded
	titles  map[string]string // track id -> display name, for the uploaded file's name
	segNext map[string]string // split part on card -> its successor ("" ends it)
}

// SetTrackTitle remembers how to name a track's upload on the card.
func (s *Sink) SetTrackTitle(id, title string) {
	s.mu.Lock()
	s.titles[id] = title
	s.mu.Unlock()
}

// segPartOf says whether cardPath belongs to the split parts of id.
func (s *Sink) segPartOf(id, cardPath string) bool {
	t := s.titles[id]
	if t == "" {
		return false
	}
	base := cardPath[strings.LastIndex(cardPath, "/")+1:]
	return strings.HasPrefix(base, sanitizeName(t)+"_")
}

// titleOf answers the remembered name of id, or a fallback derived from it.
func (s *Sink) titleOf(id string) string {
	s.mu.Lock()
	t := s.titles[id]
	s.mu.Unlock()
	if t == "" {
		t = s.Source.Info().ID + "-" + shortHash(id)
	}
	return sanitizeName(t)
}

// sanitizeName keeps a name fit for a card: no separators, length-capped.
func sanitizeName(name string) string {
	r := strings.NewReplacer("/", "／", "\\", "＼", ":", "：", "*", "＊", "?", "？", "\"", "＂", "<", "《", ">", "》", "|", "｜", "\n", " ")
	name = strings.TrimSpace(r.Replace(name))
	if name == "" {
		name = "song"
	}
	const max = 80 // bytes on card names, UTF-8
	if len(name) > max {
		for len(name) > max {
			name = name[:len(name)-3] // peel whole runes
		}
	}
	return name
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// Wrap makes src play on the codie device of client.
func Wrap(src music.Source, client *Client, cacheDir string, quality func() string) *Sink {
	return &Sink{Source: src, client: client, cacheDir: cacheDir, quality: quality,
		byCard: map[string]string{}, titles: map[string]string{}, segNext: map[string]string{}}
}

// Info is the wrapped source's, saying it plays remotely.
func (s *Sink) Info() music.Info {
	info := s.Source.Info()
	info.Playback = music.PlaybackRemote
	return info
}

// RemotePlay plays ids[start]: the track is asked of the wrapped source,
// downloaded, uploaded to the device's card and played there. The queue's
// other tracks are told to it one at a time as they're reached, which keeps
// the wait before music starts to one song.
func (s *Sink) RemotePlay(ids []string, start int, position int64) error {
	if start < 0 || start >= len(ids) {
		return fmt.Errorf("codie: nothing to play")
	}
	id := ids[start]
	quality := ""
	if s.quality != nil {
		quality = s.quality()
	}
	stream, err := s.Source.Stream(id, quality)
	if err != nil {
		return err
	}
	local, err := s.download(stream)
	if err != nil {
		return err
	}
	storage, err := s.client.MusicStorage()
	if err != nil {
		return err // already friendly: offline / credential hints
	}
	if !storage.IsLoaded {
		return errors.New(i18n.T("The device's TF card isn't ready — put the card in and try again"))
	}
	name := s.titleOf(id) + filepath.Ext(local)
	cards, err := s.client.UploadAudio(local, name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	for i, card := range cards {
		s.byCard[card] = id
		if i > 0 {
			prev := cards[i-1]
			s.segNext[prev] = card
		}
	}
	if len(cards) > 1 {
		s.segNext[cards[len(cards)-1]] = ""
	}
	s.mu.Unlock()
	if err := s.client.EnsureSource(); err != nil {
		return err
	}
	time.Sleep(2 * time.Second) // the device's file cache needs a beat to see the upload
	// A split track plays its pieces on (autoAdvance) — the queue holds only
	// those pieces; single tracks stop there, and the player moves on.
	if err := s.client.PlayCard(cards[0]); err != nil {
		return err
	}
	if position > 3000 {
		time.Sleep(700 * time.Millisecond) // let the command land before seeking
		return s.RemoteSeek(position)
	}
	return nil
}

func (s *Sink) RemotePause() error  { return s.client.Control("PAUSE", nil) }
func (s *Sink) RemoteResume() error { return s.client.Control("PLAY", nil) }
func (s *Sink) RemoteSeek(position int64) error {
	return s.client.Control("SEEK", map[string]any{"progress_ms": position})
}
func (s *Sink) RemoteVolume(percent int) error {
	return s.client.Control("SET_VOLUME", map[string]any{"volume_level": percent})
}

// RemoteState says how the device is going, translated back to the wrapped
// source's track ids. What plays there that this sink didn't put there says
// as nothing playing, and the player moves on.
func (s *Sink) RemoteState() (*music.RemoteState, error) {
	pb, err := s.client.Playback()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	id, ours := s.byCard[strings.TrimPrefix(pb.TrackID, "local:")]
	segNext := s.segNext[strings.TrimPrefix(pb.TrackID, "local:")]
	knownSegment := segNext != "" || s.segPartOf(id, strings.TrimPrefix(pb.TrackID, "local:"))
	s.mu.Unlock()
	if pb.TrackID == "" && !pb.IsPlaying && pb.PositionMs <= 0 {
		// Nothing plays there.
		return &music.RemoteState{}, nil
	}
	if !ours {
		// The device went off on its own (its old queue's auto-advance, or a
		// hand on the device): don't claim it — the player waits a moment and
		// moves the queue on itself.
		return nil, nil
	}
	// A split track's piece just ended: play the next part here, so the song
	// continues whatever the device's own folder queue would do.
	if pb.TrackID != "" && knownSegment && !pb.IsPlaying && segNext != "" {
		next := segNext
		go func() { _ = s.client.PlayCard(next) }()
	}
	return &music.RemoteState{
		Playing:  pb.IsPlaying,
		TrackID:  id,
		Position: pb.PositionMs,
		Duration: pb.DurationMs,
	}, nil
}

// download keeps the stream's audio in the cache dir, by its URL, so the same
// song uploads at once next time.
func (s *Sink) download(stream *music.Stream) (string, error) {
	if stream == nil || stream.URL == "" {
		return "", fmt.Errorf("codie: no stream url")
	}
	sum := sha256.Sum256([]byte(stream.URL))
	name := fmt.Sprintf("%s-%s%s", s.Source.Info().ID, hex.EncodeToString(sum[:12]), ext(stream.Type))
	dest := filepath.Join(s.cacheDir, name)
	if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
		return dest, nil
	}
	if err := os.MkdirAll(s.cacheDir, 0o700); err != nil {
		return "", err
	}
	req, err := http.NewRequest("GET", stream.URL, nil)
	if err != nil {
		return "", err
	}
	for k, v := range stream.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
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
	size, err := io.Copy(f, io.LimitReader(resp.Body, 1<<30))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil || size == 0 {
		os.Remove(tmp)
		if err == nil {
			err = fmt.Errorf("empty download")
		}
		return "", err
	}
	return dest, os.Rename(tmp, dest)
}

func ext(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "flac":
		return ".flac"
	case "ogg":
		return ".ogg"
	case "m4a", "aac":
		return ".m4a"
	case "wav":
		return ".wav"
	default:
		return ".mp3"
	}
}
