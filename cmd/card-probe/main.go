package main

import (
	"encoding/json"
	"fmt"
	"os"

	"mygo-music/internal/codieout"
)

func main() {
	b, _ := os.ReadFile("/tmp/.m0-creds.json")
	var raw map[string]string
	json.Unmarshal(b, &raw)
	c := codieout.New(&codieout.Creds{Addr: "192.168.1.20:8081", Secret: raw["secret"], SessionID: raw["session_id"], UserID: raw["user_id"], Kind: "client"})
	n, err := c.ClearCardUploads()
	fmt.Printf("removed=%d err=%v\n", n, err)
	st, _ := c.MusicStorage()
	fmt.Printf("files left=%d used=%dMB\n", len(st.Files), st.CapacityU/(1<<20))
	for _, f := range st.Files {
		fmt.Println("  still:", f.Name[:min(50, len(f.Name))])
	}
}
