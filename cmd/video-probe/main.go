package main

import (
	"fmt"
	"os"

	"mygo-music/internal/plugin"
)

// What a source of videos answers, against the live service: the first of a
// search, its first episode's stream, and a page of its catalog.
//
//	go run ./cmd/video-probe plugins/huangguo /tmp/huangguo-data 爱
//	go run ./cmd/video-probe plugins/bilibili /tmp/bilibili-data 迷星叫
func main() {
	p, err := plugin.Load(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Println("load:", err)
		return
	}
	defer p.Close()
	v := p.Video()
	if v == nil {
		fmt.Println("no source of videos in", os.Args[1])
		return
	}
	query := ""
	if len(os.Args) > 3 {
		query = os.Args[3]
	}

	if v.Can("videoHome") {
		home, err := v.Home()
		if err != nil {
			fmt.Printf("home err: %v\n", err)
		} else {
			fmt.Printf("home %q: %d rows\n", home.Title, len(home.Rows))
			for _, row := range home.Rows {
				n := len(row.Items)
				if row.MorePage != "" {
					n++
				}
				fmt.Printf("  row %q: %d\n", row.Title, n)
			}
		}
	}

	if query == "" {
		return
	}
	found, err := v.Search(query, 0)
	if err != nil {
		fmt.Printf("search err: %v\n", err)
		return
	}
	fmt.Printf("search %q: %d items\n", query, len(found.Items))
	for i, item := range found.Items {
		if i == 5 {
			break
		}
		fmt.Printf("  [%d] %s (%s) cover=%t\n", i, item.Title, item.ID, item.Cover != "")
	}
	if len(found.Items) == 0 {
		return
	}
	item := found.Items[0]
	d, err := v.Video(item.ID)
	if err != nil {
		fmt.Printf("item err: %v\n", err)
		return
	}
	fmt.Printf("item %q: %d episodes\n", d.Title, len(d.Episodes))
	if len(d.Episodes) == 0 {
		return
	}
	ep := d.Episodes[0]
	s, err := v.Play(d.ID, ep.ID)
	if err != nil {
		fmt.Printf("play err: %v\n", err)
		return
	}
	kind := s.URL
	if len(kind) > 90 {
		kind = kind[:90]
	}
	fmt.Printf("play %s audio=%t headers=%d: %s\n", s.Type, s.AudioURL != "", len(s.Headers), kind)
}
