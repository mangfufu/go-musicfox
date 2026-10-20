package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	desktop "github.com/go-musicfox/go-musicfox/internal/desktop_lyrics"
	"github.com/go-musicfox/go-musicfox/internal/keybindings"
)

type terminalSettingsController struct {
	desktopLyricSink
	cfg     configs.DesktopLyricsConfig
	change  desktop.SettingsChange
	err     error
	handler func()
}

func (c *terminalSettingsController) Settings() configs.DesktopLyricsConfig { return c.cfg }
func (c *terminalSettingsController) SetSettingsHandler(fn func())          { c.handler = fn }
func (c *terminalSettingsController) EditSettings(change desktop.SettingsChange) (configs.DesktopLyricsConfig, error) {
	c.change = change
	if c.err == nil {
		switch change.Key {
		case "textAlign":
			c.cfg.TextAlign = change.Value
		case "textColor":
			c.cfg.TextColor = change.Value
		}
	}
	return c.cfg, c.err
}

func TestLyricsSettingsEntriesOpenTerminalPageAndReturn(t *testing.T) {
	a, n := newFormPageTestApp(t)
	c := &terminalSettingsController{cfg: configs.DesktopLyricsConfig{FontSize: 36, TextAlign: "center", TextColor: "#E5C1B3"}}
	n.desktopLyrics = c
	n.desktopLyricsDisabled = true
	bindings := keybindings.BuildEffectiveBindings(map[keybindings.OperateType][]string{keybindings.OpDesktopLyricsSettings: {"f11"}}, true)
	h := &EventHandler{netease: n, keyToOperateMap: keybindings.BuildKeyToOperateTypeMap(bindings)}
	if h.keyToOperateMap["f10"] != keybindings.OpToggleDesktopLyrics {
		t.Fatal("F10 should toggle lyrics")
	}
	_, page, _ := h.KeyMsgHandle(tea.KeyPressMsg{Code: tea.KeyF11}, a)
	settings, ok := page.(*lyricsSettingsPage)
	if !ok || settings.from != a.CurPage() || !n.desktopLyricsDisabled {
		t.Fatal("settings did not preserve return page and disabled state")
	}
	page, _ = handleGenericContextAction(n, a, "generic:desktopLyricsSettings")
	if _, ok := page.(*lyricsSettingsPage); !ok {
		t.Fatal("context menu did not open terminal settings")
	}
	cassette := &cassettePage{netease: n, handler: h, pending: true}
	page, _ = cassette.Update(tea.KeyPressMsg{Code: tea.KeyF11}, a)
	settings, ok = page.(*lyricsSettingsPage)
	if !ok || settings.from != cassette || cassette.pending {
		t.Fatal("cassette settings entry failed")
	}
	page, cmd := settings.Update(tea.KeyPressMsg{Code: tea.KeyEscape}, a)
	if page != cassette || cmd == nil {
		t.Fatal("back did not resume cassette page")
	}
	// Returning emits the page wake-up message so its animation can resume.
	if _, ok := cmd().(cassetteWakeMsg); !ok {
		t.Fatal("cassette wake-up message missing")
	}
}

func TestTerminalLyricsSettingsColorAlignmentAndFailure(t *testing.T) {
	a, n := newFormPageTestApp(t)
	c := &terminalSettingsController{cfg: configs.DesktopLyricsConfig{FontSize: 36, TextAlign: "center", TextColor: "#E5C1B3", RecentColors: []string{"#123456"}}}
	p := newLyricsSettingsPage(n, a.CurPage(), c)
	p.selected = 9
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyRight}, a)
	if cmd == nil || !p.busy {
		t.Fatal("alignment did not start an async edit")
	}
	p.Update(cmd(), a)
	if p.cfg.TextAlign != "right" || p.busy {
		t.Fatal("alignment was not applied")
	}
	p.selected = 6
	p.activate(1, true)
	if p.colorKey != "textColor" {
		t.Fatal("color selection did not open palette")
	}
	p.View(a)
	_, cmd = p.Update(tea.MouseClickMsg{X: 8, Y: 7, Button: tea.MouseLeft}, a)
	if cmd == nil {
		t.Fatal("mouse did not select a color")
	}
	p.Update(cmd(), a)
	if c.change.Key != "textColor" || c.change.Value != "#FFFFFF" {
		t.Fatalf("wrong color hit: %+v", c.change)
	}
	p.colorSelected = len(p.colors()) - 1
	cmd = p.activate(1, true)
	p.Update(cmd(), a)
	if c.change.Key != "textColorDialog" {
		t.Fatal("custom color did not open native chooser")
	}
	found := false
	for _, color := range p.colors() {
		if color.value == "#123456" {
			found = true
		}
	}
	if !found {
		t.Fatal("recent custom color not available")
	}
	p.colorKey = ""
	p.selected = 9
	c.err = errors.New("保存失败")
	cmd = p.activate(-1, true)
	p.Update(cmd(), a)
	if p.cfg.TextAlign != "right" || p.status != "保存失败" {
		t.Fatal("failed edit changed displayed settings")
	}
}

func TestTerminalLyricsSettingsFitsSmallWindows(t *testing.T) {
	a, n := newFormPageTestApp(t)
	c := &terminalSettingsController{cfg: configs.DesktopLyricsConfig{FontSize: 36, TextColor: "#FFFFFF", TextAlign: "left", FontName: strings.Repeat("长字体", 40)}}
	p := newLyricsSettingsPage(n, a.CurPage(), c)
	for _, size := range [][2]int{{120, 40}, {80, 24}, {40, 16}, {24, 10}, {12, 6}} {
		a.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		p.selected = len(p.rows()) - 1
		for _, colorMode := range []bool{false, true} {
			p.colorKey = ""
			if colorMode {
				p.colorKey = "textColor"
				p.colorSelected = len(p.colors()) - 1
			}
			lines := strings.Split(p.View(a), "\n")
			if len(lines) != size[1] {
				t.Fatalf("height at %v = %d", size, len(lines))
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("overflow at %v: %q", size, line)
				}
			}
		}
	}
}
