package main

import (
	"encoding/json"
	"fmt"
	"os"

	"mygo-music/internal/codieout"
)

func main() {
	var creds codieout.Creds
	b, _ := os.ReadFile("/tmp/.m0-creds.json")
	var raw map[string]string
	json.Unmarshal(b, &raw)
	creds = codieout.Creds{Addr: "192.168.1.160:8081", Secret: raw["secret"], SessionID: raw["session_id"], UserID: raw["user_id"], Kind: "client"}
	c := codieout.New(&creds)
	target := os.Args[1]
	if err := c.DeleteCardFile(target); err != nil {
		fmt.Println("DELETE FAILED:", err)
		return
	}
	fmt.Println("deleted:", target)
	st, _ := c.MusicStorage()
	fmt.Println("files left:", len(st.Files), "used:", st.CapacityU/(1<<20), "MB")
}
