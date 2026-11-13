package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"mygo-music/internal/codieout"
	"mygo-music/internal/plugin"
)

func main() {
	p, err := plugin.Load(os.Args[1], "/tmp/bili-data")
	if err != nil {
		fmt.Println("load:", err)
		return
	}
	var creds codieout.Creds
	b, err := os.ReadFile("/tmp/.m0-creds.json")
	if err != nil {
		fmt.Println("creds:", err)
		return
	}
	var raw map[string]string
	json.Unmarshal(b, &raw)
	creds = codieout.Creds{Addr: "192.168.1.160:8081", Secret: raw["secret"], SessionID: raw["session_id"], UserID: raw["user_id"], Kind: "client"}
	client := codieout.New(&creds)
	st, err := client.MusicStorage()
	fmt.Printf("tf=%v files=%d\n", st.IsLoaded, len(st.Files))

	sink := codieout.Wrap(p, client, "/tmp/bili-cache", func() string { return "high" })
	sink.SetTrackTitle("BV1BZbSzZEGT", "周杰伦 - 晴天 (B站测试)")
	if err := sink.RemotePlay([]string{"BV1BZbSzZEGT"}, 0, 0); err != nil {
		fmt.Println("RemotePlay FAILED:", err)
		return
	}
	fmt.Println("RemotePlay OK")
	for i := 0; i < 5; i++ {
		time.Sleep(3 * time.Second)
		pb, err := client.Playback()
		if err != nil {
			fmt.Printf("[%ds] playback err: %v\n", (i+1)*3, err)
			continue
		}
		fmt.Printf("[%ds] status=%s track=%s pos=%d playing=%v\n", (i+1)*3, pb.Status, pb.TrackName, pb.PositionMs, pb.IsPlaying)
	}
	st2, _ := client.MusicStorage()
	fmt.Printf("files now=%d\n", len(st2.Files))
}
