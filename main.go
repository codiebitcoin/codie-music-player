package main

import (
	"embed"
	"log"

	"mygo-music/internal/app"
)

// plugins are the music sources the app ships with: NetEase Cloud Music,
// QQ Music, Bilibili, YouTube Music and Spotify (MyGotify), installed into
// the plugins folder on first run.
//
//go:embed all:plugins
var bundledPlugins embed.FS

func main() {
	if err := app.Run(bundledPlugins); err != nil {
		log.Fatal(err)
	}
}
