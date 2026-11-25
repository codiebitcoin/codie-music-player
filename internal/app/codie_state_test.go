package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/codieout"
)

// The sidebar says how the paired device is, and says nothing of a device
// when none is paired; choosing it opens the Codie settings.
func TestSidebarCodieRow(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if _, ok := tt.Find("Codie device"); ok {
		t.Fatal("a device is said of, with none paired")
	}

	a.settings.Codie = &codieout.Creds{Addr: "192.168.1.23:8081", Secret: "s"}
	a.codieState = "online"
	tt.Frame()
	if _, ok := tt.Find("Codie device"); !ok {
		t.Fatalf("the paired device isn't said of: %q", tt.Texts())
	}
	if err := tt.Click("Codie device"); err != nil {
		t.Fatal(err)
	}
	if a.settingsPage != "codie" {
		t.Errorf("it opened %q", a.settingsPage)
	}
}
