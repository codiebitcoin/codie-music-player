package main

import (
	"fmt"
	"os"

	"mygo-music/internal/plugin"
)

func main() {
	p, err := plugin.Load(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Println("load:", err)
		return
	}
	res, err := p.Search(os.Args[3], "tracks", 0)
	if err != nil {
		fmt.Printf("search err: %v\n", err)
		return
	}
	for i, t := range res.Tracks {
		fmt.Printf("track[%d] %s\n", i, t.Name)
	}
	if len(res.Tracks) == 0 {
		fmt.Println("search ok: 0 tracks")
	}
}
