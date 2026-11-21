package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	mygovideo "github.com/egoist/mygo/ui/video"

	"mygo-music/internal/video"
)

// How an episode comes to the player: directly, where the system plays the
// stream as it is, and through ffmpeg where it can't — a stream whose picture
// and sound come apart gets joined, and one the system won't read, or that
// asks for headers, lands in a file first. An episode is short, so a pass of
// ffmpeg takes a moment, not a wait.

// videoFFmpegTimeout is how long making an episode's file may take.
const videoFFmpegTimeout = 10 * time.Minute

// openEpisodeStream opens what an episode plays through. The second answer is
// a file made for the player, which the caller gives up when another comes.
// It's a variable so tests can put a fake in.
var openEpisodeStream = func(s *video.Stream) (*ui.VideoPlayer, string, error) {
	switch {
	case s.AudioURL != "", len(s.Headers) > 0, s.Type == "hls" && runtime.GOOS == "linux":
		return episodeThroughFFmpeg(s)
	default:
		p, err := mygovideo.OpenURL(s.URL)
		return p, "", err
	}
}

// episodeThroughFFmpeg makes the episode into one mp4 for the player, going
// through ffmpeg.
func episodeThroughFFmpeg(s *video.Stream) (*ui.VideoPlayer, string, error) {
	ffmpeg := findFFmpeg()
	if ffmpeg == "" {
		return nil, "", errors.New(T("This video plays through ffmpeg, which isn't installed. Install it, then play again."))
	}
	f, err := os.CreateTemp("", "codie-video-*.mp4")
	if err != nil {
		return nil, "", err
	}
	dst := f.Name()
	f.Close()
	ctx, cancel := context.WithTimeout(context.Background(), videoFFmpegTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, videoFFmpegArgs(s, dst)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(dst)
		if ctx.Err() != nil {
			return nil, "", errors.New(T("This episode took too long to open. Try again."))
		}
		return nil, "", fmt.Errorf("%s", tailLine(out))
	}
	p, err := mygovideo.Open(dst)
	if err != nil {
		os.Remove(dst)
		return nil, "", err
	}
	return p, dst, nil
}

// videoFFmpegArgs builds ffmpeg's arguments for making dst of the stream: a
// stream whose picture and sound are apart comes out joined, and its headers
// go with every input it names.
func videoFFmpegArgs(s *video.Stream, dst string) []string {
	args := []string{"-hide_banner", "-loglevel", "error"}
	hdr := ffmpegHeaders(s.Headers)
	appendInput := func(url string) {
		if hdr != "" {
			args = append(args, "-headers", hdr)
		}
		args = append(args, "-i", url)
	}
	appendInput(s.URL)
	if s.AudioURL != "" {
		appendInput(s.AudioURL)
		args = append(args, "-map", "0:v:0", "-map", "1:a:0?")
	}
	return append(args, "-c", "copy", "-movflags", "+faststart", "-y", dst)
}

// ffmpegHeaders is headers as ffmpeg's -headers takes them: one line to each,
// in a steady order.
func ffmpegHeaders(headers map[string]string) string {
	if len(headers) == 0 {
		return ""
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	slices.Sort(names)
	var b strings.Builder
	for _, name := range names {
		b.WriteString(name)
		b.WriteString(": ")
		b.WriteString(headers[name])
		b.WriteString("\r\n")
	}
	return b.String()
}

// findFFmpeg finds ffmpeg as the plugins do: on PATH, then the usual places.
func findFFmpeg() string {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	for _, p := range []string{"/opt/homebrew/bin/ffmpeg", "/usr/local/bin/ffmpeg"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// tailLine is the end of what a program wrote, to say why it stopped.
func tailLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return lines[len(lines)-1]
}

// videoPlayEpisode opens an episode in the player. It runs off the main
// thread until the player is ready to show.
func (a *app) videoPlayEpisode(item video.Item, ep video.Episode, at int) {
	lib := a.videoLibOfItem(item)
	if lib == nil {
		return
	}
	// One at a time: what was opening before is gone, its file with it.
	a.videoStopPlayer()
	a.video.openErr = ""
	a.video.playing = videoItemRef{item: item, ep: ep, at: at}
	a.video.page = videoPlay
	a.redrawVideo()
	ask := videoItemRef{item: item, ep: ep}
	go func() {
		s, err := lib.src.Play(item.ID, ep.ID)
		if err != nil {
			a.post(func() {
				if a.video.playing != ask {
					return
				}
				a.video.openErr = videoProblem(err, T("Couldn't play this episode"))
				a.redrawVideo()
			})
			return
		}
		p, file, err := openEpisodeStream(s)
		a.post(func() {
			if a.video.playing != ask {
				if file != "" {
					os.Remove(file)
				}
				return
			}
			if err != nil {
				a.video.openErr = videoProblem(err, T("Couldn't play this episode"))
				a.redrawVideo()
				return
			}
			a.video.player, a.video.playerFile = p, file
			p.SetVolume(a.video.vol / 100)
			a.redrawVideo()
		})
	}()
}

// videoNextEpisode opens the episode after the one playing, or the one
// before with back, for a video that has it.
func (a *app) videoNextEpisode(back bool) {
	d := a.video.item
	if d == nil || d.video == nil {
		return
	}
	eps := d.video.Episodes
	at := a.video.playing.at + 1
	if back {
		at = a.video.playing.at - 1
	}
	if at < 0 || at >= len(eps) {
		return
	}
	a.videoPlayEpisode(d.video.Item, eps[at], at)
}

// redrawVideo redraws the video window, if it shows.
func (a *app) redrawVideo() {
	if a.videoWin != nil {
		a.videoWin.Invalidate()
	}
}
