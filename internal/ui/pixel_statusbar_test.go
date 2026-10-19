package ui

import (
	"regexp"
	"testing"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestCassetteStatusBarUsesFontSafeClock(t *testing.T) {
	app, _, _ := newCassetteDashboardFixture(t, "top")
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	bar := app.MustMain().StatusBar().View(app, app.MustMain())
	plain := ansi.Strip(bar)
	if !regexp.MustCompile(`[0-9]{2}:[0-9]{2}`).MatchString(plain) {
		t.Fatalf("clock missing: %q", plain)
	}
	for _, r := range plain {
		if unicode.Is(unicode.Co, r) {
			t.Fatalf("private-use icon in pixel status bar: %U", r)
		}
	}
}
