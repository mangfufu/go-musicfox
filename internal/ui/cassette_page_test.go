package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	"github.com/go-musicfox/go-musicfox/internal/keybindings"
	"github.com/go-musicfox/go-musicfox/internal/lyric"
	backend "github.com/go-musicfox/go-musicfox/internal/player"
	"github.com/go-musicfox/go-musicfox/internal/playlist"
	"github.com/go-musicfox/go-musicfox/internal/structs"
	"github.com/go-musicfox/go-musicfox/internal/types"
)

type cassetteTestPlayer struct {
	backend.Player
	state    types.State
	position time.Duration
}

func (p *cassetteTestPlayer) State() types.State        { return p.state }
func (p *cassetteTestPlayer) PassedTime() time.Duration { return p.position }
func (p *cassetteTestPlayer) Volume() int               { return 60 }
func (p *cassetteTestPlayer) Toggle() {
	if p.state == types.Playing {
		p.state = types.Paused
	} else {
		p.state = types.Playing
	}
}

func TestCassetteLifecycleAndRealState(t *testing.T) {
	a, n := newFormPageTestApp(t)
	previousBindings := configs.EffectiveKeybindings
	t.Cleanup(func() { configs.EffectiveKeybindings = previousBindings })
	configs.EffectiveKeybindings = keybindings.BuildEffectiveBindings(nil, true)
	engine := &cassetteTestPlayer{state: types.Playing, position: 24 * time.Second}
	n.player = &Player{Player: engine, playlistManager: playlist.NewPlaylistManager()}
	n.lyricService = lyric.NewService(nil, true, 0, true)
	_ = n.player.playlistManager.Initialize(0, []structs.Song{{Id: 1, Name: "真实状态标题", Duration: 4 * time.Minute}, {Id: 2, Name: "第二首"}})
	p := newCassettePage(n, NewEventHandler(n))
	_, cmd := p.Update(p.Msg(), a)
	if cmd == nil || !p.pending {
		t.Fatal("entry did not start timer")
	}
	generation := p.generation
	for i := 0; i < 10; i++ {
		_, cmd = p.Update(p.Msg(), a)
		if cmd != nil || p.generation != generation {
			t.Fatal("duplicate refresh started another timer")
		}
		_, cmd = p.Update(tea.WindowSizeMsg{Width: 100, Height: 30}, a)
		if cmd != nil {
			t.Fatal("resize started another timer")
		}
	}
	p.Update(cassetteTickMsg{p, generation}, a)
	if p.frame != 1 {
		t.Fatal("playing reel did not advance")
	}
	// Replayed ticks must not fork a second chain.
	_, cmd = p.Update(cassetteTickMsg{p, generation}, a)
	if cmd != nil || p.frame != 1 {
		t.Fatal("stale tick changed animation")
	}
	p.Update(tea.KeyPressMsg{Code: ' ', Text: " "}, a)
	if engine.State() != types.Paused {
		t.Fatal("space did not pause player")
	}
	p.Update(cassetteTickMsg{p, p.generation}, a)
	if p.frame != 1 {
		t.Fatal("paused reel moved")
	}
	view := ansi.Strip(p.View(a))
	if !strings.Contains(view, "真实状态标题") || !strings.Contains(view, "PAUSED") || !strings.Contains(view, "03:36") {
		t.Fatal("view is not reading playback state")
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyDown}, a)
	if p.selected != 1 || n.player.CurSongIndex() != 0 {
		t.Fatal("selection changed playing song")
	}
	next, _ := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape}, a)
	if next != a.MustMain() || p.pending {
		t.Fatal("back did not restore main page")
	}
	p2 := newCassettePage(n, p.handler)
	_, cmd = p2.Update(cassetteTickMsg{p, p.generation}, a)
	if cmd != nil || p2.pending {
		t.Fatal("old page tick started new page timer")
	}
}

func TestCassetteLyricsTranslationAndYRC(t *testing.T) {
	s := lyric.State{Fragments: []lyric.LRCFragment{{StartTimeMs: 1000, Content: "原文"}}, TranslatedFragments: map[int64]string{1000: "译文"}, ShowTranslation: true}
	lines, index := cassetteLyrics(s)
	if index != 0 || lines[0].Translation != "译文" {
		t.Fatal("LRC translation missing")
	}
	s.YRCEnabled = true
	s.YRCLines = []lyric.YRCLine{{Words: []lyric.YRCWord{{Word: "Hello "}, {Word: "world"}}, TranslatedLyric: "你好世界"}}
	lines, _ = cassetteLyrics(s)
	if lines[0].Text != "Hello world" || lines[0].Translation != "你好世界" {
		t.Fatal("YRC text missing")
	}
	s.ShowTranslation = false
	lines, _ = cassetteLyrics(s)
	if lines[0].Translation != "" {
		t.Fatal("translation setting ignored")
	}
}

func TestCassetteCustomKeysDriveHintsAndControls(t *testing.T) {
	a, n := newFormPageTestApp(t)
	previous := configs.EffectiveKeybindings
	t.Cleanup(func() { configs.EffectiveKeybindings = previous })
	configs.EffectiveKeybindings = keybindings.BuildEffectiveBindings(map[keybindings.OperateType][]string{
		keybindings.OpPlayOrToggle: {"f6"}, keybindings.OpCassetteView: {"f7"},
	}, true)
	engine := &cassetteTestPlayer{state: types.Playing}
	n.player = &Player{Player: engine, playlistManager: playlist.NewPlaylistManager()}
	p := newCassettePage(n, NewEventHandler(n))
	found := false
	for _, h := range p.hints() {
		if h.Label == "PLAY" {
			found = h.Key == "F6"
		}
	}
	if !found {
		t.Fatal("hint did not use effective binding")
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyF6}, a)
	if engine.State() != types.Paused {
		t.Fatal("custom toggle key did not pause")
	}
	next, _ := p.Update(tea.KeyPressMsg{Code: tea.KeyF7}, a)
	if next != a.MustMain() {
		t.Fatal("custom view key did not return")
	}
}
