package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/anhoder/foxful-cli/model"
	"github.com/charmbracelet/x/ansi"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	desktop "github.com/go-musicfox/go-musicfox/internal/desktop_lyrics"
	"github.com/go-musicfox/go-musicfox/internal/keybindings"
)

type panelTestController struct {
	desktop.Controller
	cfg    configs.DesktopLyricsConfig
	err    error
	resets int
}

func (c *panelTestController) Settings() configs.DesktopLyricsConfig { return c.cfg }
func (c *panelTestController) ApplySettings(cfg configs.DesktopLyricsConfig) error {
	if c.err != nil {
		return c.err
	}
	c.cfg = cfg
	return nil
}
func (c *panelTestController) ResetPosition() error { c.resets++; return c.err }

func TestTerminalLyricsPanelActionsAndLayout(t *testing.T) {
	a, n := newFormPageTestApp(t)
	c := &panelTestController{cfg: desktop.StylePresets[0].Apply(configs.DesktopLyricsConfig{Enable: true, Draggable: true})}
	n.desktopLyrics = c
	h := &EventHandler{netease: n, keyToOperateMap: keybindings.BuildKeyToOperateTypeMap(keybindings.BuildEffectiveBindings(nil, true))}
	_, page, _ := h.KeyMsgHandle(tea.KeyPressMsg{Code: tea.KeyF10}, a)
	p, ok := page.(*desktopSettingsPage)
	if !ok {
		t.Fatal("F10 did not open terminal panel")
	}
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 42})
	finish := func(cmd tea.Cmd) {
		t.Helper()
		if cmd == nil {
			t.Fatal("missing save command")
		}
		p.Update(cmd(), a)
	}
	click := func(id int) tea.Cmd {
		t.Helper()
		p.selected = id
		p.ensureFocus = true
		p.View(a)
		for _, b := range p.buttons {
			if b.id == id {
				_, cmd := p.Update(tea.MouseClickMsg(tea.Mouse{X: 2 + b.x - p.scrollX + b.width/2, Y: 4 + b.y - p.scrollY + 1, Button: tea.MouseLeft}), a)
				return cmd
			}
		}
		t.Fatal("missing button")
		return nil
	}
	finish(click(lsVertical))
	finish(click(lsLock))
	if !c.cfg.Vertical || c.cfg.Draggable {
		t.Fatal("direction/lock controls did not apply")
	}
	for i, preset := range desktop.StylePresets {
		finish(click(lsPreset + i))
		if !preset.Matches(c.cfg) || !c.cfg.Vertical || c.cfg.Draggable {
			t.Fatal("preset did not apply or changed layout")
		}
	}
	click(lsText)
	p.input.SetValue("bad")
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, a)
	if cmd != nil || !p.editing {
		t.Fatal("invalid color accepted")
	}
	p.input.SetValue("#FFCC88")
	_, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, a)
	finish(cmd)
	if c.cfg.TextColor != "#FFCC88" {
		t.Fatal("color edit not saved")
	}
	c.err = errors.New("配置无法写入")
	before := c.cfg
	finish(click(lsBold))
	if c.cfg != before || !strings.Contains(p.status, c.err.Error()) {
		t.Fatal("save failure not preserved")
	}
	c.err = nil
	finish(click(lsReset))
	if c.resets != 1 {
		t.Fatal("reset not requested")
	}
	finish(click(lsPreset))
	p.status = "已保存，桌面歌词已更新"
	if folder := os.Getenv("MUSICFOX_LYRICS_ARTIFACT_DIR"); folder != "" {
		if err := os.WriteFile(filepath.Join(folder, "lyrics-settings.ansi"), []byte(p.View(a)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Every button remains reachable and hit-testable after narrow/short resizes.
	for _, size := range [][2]int{{100, 42}, {80, 24}, {48, 18}} {
		a.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for id := 0; id < lsPreset+len(desktop.StylePresets); id++ {
			p.selected = id
			p.ensureFocus = true
			view := p.View(a)
			if strings.Count(view, "\n")+1 > size[1] {
				t.Fatal("vertical overflow")
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > size[0] {
					t.Fatal("horizontal overflow")
				}
			}
			for _, b := range p.buttons {
				if b.id == id && p.buttonAt(2+b.x-p.scrollX+b.width/2, 4+b.y-p.scrollY+1) != id {
					t.Fatalf("button %d is unreachable at %v", id, size)
				}
			}
		}
	}
	p.selected = lsDone
	next, _ := p.activate(a)
	if next != a.MustMain() {
		t.Fatal("Done did not return")
	}
}

func TestLyricsContextEntryPreservesEditingAndCassette(t *testing.T) {
	a, n := newFormPageTestApp(t)
	c := &panelTestController{cfg: desktop.StylePresets[0].Apply(configs.DesktopLyricsConfig{Enable: true})}
	n.desktopLyrics = c
	previous := a.CurPage()
	request := model.OpenPageMsg{Build: n.openDesktopLyricsSettings}
	a.Update(request)
	p, ok := a.CurPage().(*desktopSettingsPage)
	if !ok || p.back != previous {
		t.Fatal("right click did not navigate")
	}
	p.editing = true
	p.input.SetValue("尚未保存")
	a.Update(request)
	if a.CurPage() != p || p.input.Value() != "尚未保存" {
		t.Fatal("repeated right click reset editing")
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.CurPage() != previous {
		t.Fatal("return lost original page")
	}
	h := &EventHandler{netease: n, keyToOperateMap: keybindings.BuildKeyToOperateTypeMap(keybindings.BuildEffectiveBindings(nil, true))}
	cassette := &cassettePage{netease: n, handler: h, pending: true}
	page, _ := cassette.Update(tea.KeyPressMsg{Code: tea.KeyF10}, a)
	settings, ok := page.(*desktopSettingsPage)
	if !ok || cassette.pending {
		t.Fatal("cassette did not suspend")
	}
	page, cmd := settings.Update(tea.KeyPressMsg{Code: tea.KeyEscape}, a)
	if page != cassette || cmd == nil {
		t.Fatal("cassette did not resume")
	}
}
