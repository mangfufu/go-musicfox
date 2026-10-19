package ui

import (
	tea "charm.land/bubbletea/v2"
	desktop "github.com/go-musicfox/go-musicfox/internal/desktop_lyrics"
	"github.com/go-musicfox/go-musicfox/internal/keybindings"
	"testing"
)

type nativePanelController struct {
	desktop.Controller
	opened int
}

func (c *nativePanelController) OpenSettings() { c.opened++ }

func TestLyricsSettingsEntriesKeepTerminalPage(t *testing.T) {
	a, n := newFormPageTestApp(t)
	c := &nativePanelController{}
	n.desktopLyrics = c
	h := &EventHandler{netease: n, keyToOperateMap: keybindings.BuildKeyToOperateTypeMap(keybindings.BuildEffectiveBindings(nil, true))}
	_, page, cmd := h.KeyMsgHandle(tea.KeyPressMsg{Code: tea.KeyF10}, a)
	if c.opened != 1 || page != nil || cmd != nil {
		t.Fatal("F10 should open a native panel without replacing the terminal page")
	}
	page, cmd = handleGenericContextAction(n, a, "generic:desktopLyricsSettings")
	if c.opened != 2 || page != nil || cmd != nil {
		t.Fatal("terminal context menu did not open the native panel")
	}
	cassette := &cassettePage{netease: n, handler: h, pending: true}
	page, cmd = cassette.Update(tea.KeyPressMsg{Code: tea.KeyF10}, a)
	if c.opened != 3 || page != cassette || !cassette.pending || cmd != nil {
		t.Fatal("settings interrupted the cassette page")
	}
}
