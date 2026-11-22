package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The playback-source log button opens the viewer and its clear action removes buffered entries.
func TestSourceLogViewerActions(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settingsView.open = true
	a.settingsView.tab = settingsPlayback
	a.source = newSourceManager()
	a.source.addLog("info", "test entry")
	tester := ui.NewTester(a.view, 1100, 760)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true})
	tester.Frame()
	if err := tester.Click("运行日志"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !a.settingsView.sourceLogs {
		t.Fatal("the log viewer did not open")
	}
	if err := tester.Click("清空"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if got := len(a.source.logEntries()); got != 0 {
		t.Fatalf("clear left %d log entries", got)
	}
}
