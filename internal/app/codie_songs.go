package app

import (
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// CodieSong is one song sitting on the paired device's card.
type CodieSong struct {
	Name string
	Path string
}

// codieLoadSongs asks the device for its card's song list, off the main
// thread, and keeps it for the long-press menu.
func (a *app) codieLoadSongs() {
	c := a.codieClient()
	if c == nil {
		a.post(func() { a.codieSongsErr = "未配对设备" })
		return
	}
	st, err := c.MusicStorage()
	a.post(func() {
		if err != nil {
			a.codieSongsErr = err.Error()
			a.redraw()
			return
		}
		a.codieSongs, a.codieSongsErr = nil, ""
		if st.IsLoaded {
			for _, f := range st.Files {
				a.codieSongs = append(a.codieSongs, CodieSong{Name: f.Name, Path: f.Path})
			}
		} else {
			a.codieSongsErr = "TF 卡未就绪"
		}
		a.codieSongsAsked = true
		a.redraw()
	})
}

// codieDeleteOne removes one song from the card, dropping it from the menu's
// list as it goes.
func (a *app) codieDeleteOne(path string) {
	c := a.codieClient()
	if c == nil {
		return
	}
	if err := c.DeleteCardFile(path); err != nil {
		a.post(func() { a.toast = fmt.Sprintf("删除失败：%v", err) })
		return
	}
	a.post(func() {
		for i, s := range a.codieSongs {
			if s.Path == path {
				a.codieSongs = append(a.codieSongs[:i], a.codieSongs[i+1:]...)
				break
			}
		}
		a.toast = "已从设备删除"
		a.redraw()
	})
}

// codieClearAll 清空设备歌曲列表缓存（全部删除后调用）。
func (a *app) codieClearAllToasted() { a.codieClearToasted() }

// codieSongsMenu is the trash button's long-press menu: every song on the
// card, each deletable on its own, and the sweep at the foot.
func (a *app) codieSongsMenu(m *ui.Menu) {
	if !a.codieSongsAsked {
		a.codieSongsAsked = true
		go a.codieLoadSongs()
		m.Item("读取设备歌曲中…").Disabled(true)
		return
	}
	if a.codieSongsErr != "" {
		m.Item("读取失败：" + a.codieSongsErr).Disabled(true)
		return
	}
	if len(a.codieSongs) == 0 {
		m.Item("设备上没有歌曲").Disabled(true)
		return
	}
	for _, song := range a.codieSongs {
		s := song
		if m.Item(s.Name).Chosen() {
			go a.codieDeleteOne(s.Path)
		}
	}
	m.Separator()
	if m.Item("删除全部").Chosen() {
		a.post(func() { a.toast = "正在清理 codie 设备歌曲…" })
		go func() {
			a.codieClearToasted()
			mygo.RunOnMain(func() { a.redraw() })
		}()
	}
}
