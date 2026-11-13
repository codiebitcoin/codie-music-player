package codieout

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

	mu     sync.Mutex
	byCard map[string]string // card path -> original track id, of what was uploaded
}

// Wrap makes src play on the codie device of client.
func Wrap(src music.Source, client *Client, cacheDir string, quality func() string) *Sink {
	return &Sink{Source: src, client: client, cacheDir: cacheDir, quality: quality, byCard: map[string]string{}}
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
		return err
	}
	if !storage.IsLoaded {
		return fmt.Errorf("codie: the TF card isn't ready")
	}
	card, err := s.client.UploadFile(local)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.byCard[card] = id
	s.mu.Unlock()
	if err := s.client.EnsureSource(); err != nil {
		return err
	}
	if err := s.client.PlayCardPaths(storage.RootPath, []string{card}, ""); err != nil {
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
	st := &music.RemoteState{
		Playing:  pb.IsPlaying,
		Position: pb.PositionMs,
		Duration: pb.DurationMs,
	}
	s.mu.Lock()
	if id, ok := s.byCard[strings.TrimPrefix(pb.TrackID, "local:")]; ok {
		st.TrackID = id
	} else if pb.TrackID == "" {
		st.TrackID = ""
	} else {
		st.TrackID = "" // someone else's track: don't claim it
	}
	s.mu.Unlock()
	return st, nil
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
