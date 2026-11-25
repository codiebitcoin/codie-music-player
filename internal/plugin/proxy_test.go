package plugin

import (
	"os"
	"strings"
	"testing"

	"mygo-music/internal/proxy"
)

// The programs a plugin starts carry the listener's HTTP proxy: ffmpeg and
// yt-dlp read it from the environment, and local addresses never go through
// it.
func TestSpawnPassesTheProxy(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(envEnv, "1")
	t.Cleanup(func() { proxy.Apply("") })
	proxy.Apply("127.0.0.1:7897")

	p := load(t, `{"id":"x","permissions":{"process":true}}`, `
export async function suggest() {
  const proc = host.spawn(host.setting("program"), []);
  proc.closeStdin();
  return [await proc.readLine()];
}`)
	p.Grant(true)
	p.SetSetting("program", self)
	got, err := p.Suggest("")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(got, "")
	if !strings.Contains(line, "https_proxy=http://127.0.0.1:7897") {
		t.Errorf("the program didn't get the proxy: %q", line)
	}
	if !strings.Contains(line, "no_proxy="+proxy.NoProxy) {
		t.Errorf("local addresses aren't kept off it: %q", line)
	}
}
