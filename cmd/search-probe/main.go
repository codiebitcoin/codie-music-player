package main

import (
	"fmt"
	"os"

	"mygo-music/internal/plugin"
)

func main() {
	p, err := plugin.Load(os.Args[1], "/tmp/bili-data")
	if err != nil {
		fmt.Println("load:", err)
		return
	}
	res, err := p.Search("晴天", "tracks", 0)
	if err != nil {
		fmt.Printf("search err: %v\n", err)
		return
	}
	fmt.Printf("search ok: %d tracks, first=%s\n", len(res.Tracks), func() string {
		if len(res.Tracks) > 0 {
			return res.Tracks[0].Name
		}
		return "-"
	}())
}
