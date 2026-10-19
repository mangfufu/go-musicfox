package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/anhoder/foxful-cli/model"
	"github.com/charmbracelet/x/ansi"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	desktop "github.com/go-musicfox/go-musicfox/internal/desktop_lyrics"
	"github.com/go-musicfox/go-musicfox/internal/keybindings"
)

func TestDesktopLyricsContextNavigationPreservesPage(t *testing.T) {
	a, n := newFormPageTestApp(t)
	c := &terminalLyricsController{cfg: configs.DesktopLyricsConfig{Enable: true, FontSize: 36}}
	n.desktopLyrics = c
	previous := a.CurPage()
	request := model.OpenPageMsg{Build: n.openDesktopLyricsSettings}
	a.Update(request)
	p, ok := a.CurPage().(*desktopSettingsPage)
	if !ok || p.back != previous {
		t.Fatal("context entry lost the previous page")
	}
	p.selected, p.editing = 1, true
	p.input.SetValue("尚未保存的字体")
	a.Update(request)
	if a.CurPage() != p || !p.editing || p.input.Value() != "尚未保存的字体" {
		t.Fatal("repeated right click reset an edit")
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.CurPage() != previous {
		t.Fatal("closing settings did not restore previous page")
	}
}

type terminalLyricsController struct {
	desktop.Controller
	cfg    configs.DesktopLyricsConfig
	err    error
	resets int
}

func (c *terminalLyricsController) Settings() configs.DesktopLyricsConfig { return c.cfg }
func (c *terminalLyricsController) ApplySettings(cfg configs.DesktopLyricsConfig) error {
	if c.err != nil {
		return c.err
	}
	c.cfg = cfg
	return nil
}
func (c *terminalLyricsController) ResetPosition() error { c.resets++; return c.err }

func TestTerminalLyricsSettingsInteraction(t *testing.T) {
	a, n := newFormPageTestApp(t)
	c := &terminalLyricsController{cfg: configs.DesktopLyricsConfig{Enable: true, FontName: "Fusion Pixel 12px Mono zh-Hans", FontSize: 36, Draggable: true, WindowAlpha: 1}}
	n.desktopLyrics = c
	h := &EventHandler{keyToOperateMap: keybindings.BuildKeyToOperateTypeMap(keybindings.BuildEffectiveBindings(nil, true))}
	p := newDesktopSettingsPage(n, a.MustMain(), h)
	finish := func(cmd tea.Cmd) {
		t.Helper()
		if cmd == nil {
			t.Fatal("missing save command")
		}
		p.Update(cmd(), a)
	}
	key := func(code rune) tea.Cmd { _, cmd := p.Update(tea.KeyPressMsg{Code: code}, a); return cmd }
	finish(key(tea.KeyRight))
	if c.cfg.FontSize != 48 {
		t.Fatal("pixel size did not increment")
	}
	p.View(a)
	_, cmd := p.Update(tea.MouseClickMsg(tea.Mouse{X: 20, Y: p.rowStart + 7, Button: tea.MouseLeft}), a)
	finish(cmd)
	if !c.cfg.Vertical {
		t.Fatal("mouse selection did not enable vertical lyrics")
	}
	p.selected = 8
	finish(key(tea.KeyEnter))
	if c.cfg.Draggable {
		t.Fatal("lock did not apply")
	}
	p.selected = 4
	key(tea.KeyEnter)
	p.input.SetValue("bad color")
	if cmd := key(tea.KeyEnter); cmd != nil || !p.editing {
		t.Fatal("invalid color was accepted")
	}
	p.input.SetValue("#123ABC")
	finish(key(tea.KeyEnter))
	if c.cfg.TextColor != "#123ABC" {
		t.Fatal("color not saved")
	}
	c.err = errors.New("配置无法写入")
	p.selected = 2
	finish(key(tea.KeyEnter))
	if p.config.FontBold || !strings.Contains(p.status, c.err.Error()) {
		t.Fatal("failed save was not shown or changed state")
	}
	c.err = nil
	p.selected = 10
	finish(key(tea.KeyEnter))
	if c.resets != 1 {
		t.Fatal("position reset was not requested")
	}
	for _, size := range [][2]int{{80, 24}, {40, 14}, {20, 8}} {
		a.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := ansi.Strip(p.View(a))
		if strings.Count(view, "\n")+1 > size[1] {
			t.Fatal("settings overflow window height")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("settings overflow window width")
			}
		}
	}
	next, _ := p.Update(tea.KeyPressMsg{Code: tea.KeyF10}, a)
	if next != a.MustMain() {
		t.Fatal("F10 did not return to original page")
	}
}

func TestTerminalLyricsSettingsReturnsToCassette(t *testing.T) {
	a, n := newFormPageTestApp(t)
	h := &EventHandler{keyToOperateMap: keybindings.BuildKeyToOperateTypeMap(keybindings.BuildEffectiveBindings(nil, true))}
	back := &cassettePage{netease: n, handler: h, pending: true}
	next, _ := back.Update(tea.KeyPressMsg{Code: tea.KeyF10}, a)
	p, ok := next.(*desktopSettingsPage)
	if !ok || back.pending {
		t.Fatal("F10 did not suspend cassette refresh")
	}
	if p.status == "" {
		t.Fatal("disabled controller should explain how to enable lyrics")
	}
	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape}, a)
	if next != back || cmd == nil {
		t.Fatal("return lost cassette page or wake command")
	}
}
