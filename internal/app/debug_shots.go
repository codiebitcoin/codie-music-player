package app

import (
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
)

// With MM_SHOTS set to a directory, the app walks through its views and saves
// what each window shows there, for checking the design. It leaves the
// listener's settings as they were.
func init() {
	dir := os.Getenv("MM_SHOTS")
	if dir == "" {
		return
	}
	shotHook = func(a *app) {
		a.settings.path = ""
		go func() {
			shot := func(name string, w *mygo.Window) {
				png, err := w.CapturePage()
				if err == nil {
					os.WriteFile(filepath.Join(dir, name+".png"), png, 0o644)
				}
			}
			step := func(d time.Duration, fn func()) {
				time.Sleep(d)
				mygo.RunOnMain(func() { fn(); a.redraw() })
			}
			mygo.RunOnMain(func() { a.win.Show(); a.win.Focus() })
			time.Sleep(3 * time.Second)
			shot("1-home", a.win)
			step(0, func() {
				a.volume = 0.15
				a.pl.SetVolume(a.volume)
				if tracks := a.library.recent; len(tracks) > 0 {
					a.pl.PlayQueue(tracks, 0)
				}
			})
			time.Sleep(3 * time.Second)
			shot("2-playing", a.win)
			if a.pl.Current() != nil {
				step(0, func() { a.nowPlaying = true })
				time.Sleep(3 * time.Second)
				shot("3-now-playing", a.win)
				step(0, func() { a.npPane = paneQueue })
				time.Sleep(2 * time.Second)
				shot("4-queue", a.win)
			}
			// The window of the sources of videos, when there's one on: its
			// first page as it loaded, and an episode really playing.
			if len(a.videoLibs) > 0 {
				step(0, func() { a.openVideoWindow() })
				time.Sleep(3 * time.Second)
				if a.videoWin != nil {
					shot("5-videos", a.videoWin)
					step(0, func() {
						for _, lib := range a.videoLibs {
							if lib.id == "bilibili" {
								a.videoUse(lib)
							}
						}
						a.videoSearch("mixingjiao") // 迷星叫, romanized: no Chinese in code outside the catalogs
					})
					time.Sleep(4 * time.Second)
					step(0, func() {
						if d := a.video.search; d != nil && len(d.items) > 0 {
							a.videoOpenItem(d.items[0])
						}
					})
					time.Sleep(2 * time.Second)
					step(0, func() {
						if d := a.video.item; d != nil && d.video != nil && len(d.video.Episodes) > 0 {
							a.videoPlayEpisode(d.video.Item, d.video.Episodes[0], 0)
						}
					})
					time.Sleep(20 * time.Second)
					shot("6-video-playing", a.videoWin)
				}
			}
			mygo.RunOnMain(mygo.App.Quit)
		}()
	}
}
