package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/anhoder/foxful-cli/model"
	"github.com/go-musicfox/go-musicfox/internal/cassette"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	"github.com/go-musicfox/go-musicfox/internal/keybindings"
	"github.com/go-musicfox/go-musicfox/internal/lyric"
	"github.com/go-musicfox/go-musicfox/internal/types"
)

const cassettePageType model.PageType = "cassette"

type cassetteWakeMsg struct{ page *cassettePage }
type cassetteTickMsg struct {
	page       *cassettePage
	generation uint64
}

type cassettePage struct {
	netease                  *Netease
	handler                  *EventHandler
	selected, current, frame int
	pending                  bool
	generation               uint64
}

func newCassettePage(n *Netease, handler *EventHandler) *cassettePage {
	index := max(0, n.player.CurSongIndex())
	return &cassettePage{netease: n, handler: handler, selected: index, current: index}
}

func (p *cassettePage) Type() model.PageType             { return cassettePageType }
func (p *cassettePage) IgnoreQuitKeyMsg(tea.KeyMsg) bool { return false }
func (p *cassettePage) Msg() tea.Msg                     { return cassetteWakeMsg{p} }

func (p *cassettePage) schedule() tea.Cmd {
	if p.pending {
		return nil
	}
	p.pending = true
	p.generation++
	msg := cassetteTickMsg{p, p.generation}
	return tea.Tick(time.Second/8, func(time.Time) tea.Msg { return msg })
}

func (p *cassettePage) Update(msg tea.Msg, a *model.App) (model.Page, tea.Cmd) {
	switch msg := msg.(type) {
	case cassetteWakeMsg:
		if msg.page == p {
			return p, p.schedule()
		}
	case cassetteTickMsg:
		if msg.page != p || msg.generation != p.generation || !p.pending {
			return p, nil
		}
		p.pending = false
		if p.netease.player.State() == types.Playing {
			p.frame = (p.frame + 1) % 8
		}
		if index := p.netease.player.CurSongIndex(); index != p.current {
			p.current, p.selected = index, max(0, index)
		}
		return p, p.schedule()
	case tea.KeyPressMsg:
		op, bound := p.handler.keyToOperateMap[msg.String()]
		if !bound {
			return p, nil
		}
		player := p.netease.player
		switch op {
		case keybindings.OpDesktopLyricsSettings:
			p.pending = false
			return newDesktopSettingsPage(p.netease, p, p.handler), nil
		case keybindings.OpCassetteView, keybindings.OpGoBack:
			p.pending = false
			return a.MustMain(), a.RerenderCmd(true)
		case keybindings.OpMoveUp:
			p.selected = max(0, p.selected-1)
		case keybindings.OpMoveDown:
			p.selected = min(max(0, len(player.Playlist())-1), p.selected+1)
		case keybindings.OpEnter:
			queue := player.Playlist()
			if p.selected >= 0 && p.selected < len(queue) {
				player.InitSongManager(p.selected, queue)
				player.StartPlay()
			}
		case keybindings.OpPlayOrToggle, keybindings.OpToggle:
			if player.State() == types.Stopped || player.State() == types.Unknown {
				if len(player.Playlist()) > 0 {
					player.StartPlay()
				}
			} else {
				player.Toggle()
			}
		case keybindings.OpCurPlaylist:
			_, _, cmd := p.handler.handle(op)
			p.pending = false
			return a.MustMain(), cmd
		case keybindings.OpSwitchTheme:
			_, _, cmd := p.handler.handle(op)
			return p, cmd
		case keybindings.OpPrevious, keybindings.OpNext,
			keybindings.OpSeekBackward1s, keybindings.OpSeekBackward5s,
			keybindings.OpSeekForward5s, keybindings.OpSeekForward10s,
			keybindings.OpVolumeDown, keybindings.OpVolumeUp,
			keybindings.OpSwitchPlayMode, keybindings.OpHelp,
			keybindings.OpLikePlayingSong:
			_, next, cmd := p.handler.handle(op)
			if next != nil {
				p.pending = false
				return next, cmd
			}
			return p, cmd
		}
	}
	return p, nil
}

func cassetteLyrics(state lyric.State) ([]cassette.Line, int) {
	var lines []cassette.Line
	if state.YRCEnabled && len(state.YRCLines) > 0 {
		for _, line := range state.YRCLines {
			var text strings.Builder
			for _, word := range line.Words {
				text.WriteString(word.Word)
			}
			translation := ""
			if state.ShowTranslation {
				translation = line.TranslatedLyric
			}
			lines = append(lines, cassette.Line{Text: text.String(), Translation: translation})
		}
		return lines, state.YRCLineIndex
	}
	for _, line := range state.Fragments {
		translation := ""
		if state.ShowTranslation {
			translation = state.TranslatedFragments[line.StartTimeMs]
		}
		lines = append(lines, cassette.Line{Text: line.Content, Translation: translation})
	}
	return lines, state.CurrentIndex
}

func (p *cassettePage) hints() []cassette.Hint {
	return cassetteHints(p.handler, false)
}

func cassetteHints(handler *EventHandler, dashboard bool) []cassette.Hint {
	var hints []cassette.Hint
	items := []struct {
		op    keybindings.OperateType
		label string
	}{
		{keybindings.OpGoBack, "BACK"},
		{keybindings.OpPlayOrToggle, "PLAY"},
		{keybindings.OpSwitchTheme, "THEME"},
		{keybindings.OpDesktopLyricsSettings, "LYRIC SET"},
		{keybindings.OpPrevious, "PREV"},
		{keybindings.OpNext, "NEXT"},
		{keybindings.OpVolumeDown, "VOL-"},
		{keybindings.OpVolumeUp, "VOL+"},
		{keybindings.OpSwitchPlayMode, "MODE"},
		{keybindings.OpMoveUp, "SELECT"},
		{keybindings.OpEnter, "LOAD"},
		{keybindings.OpHelp, "HELP"},
	}
	if dashboard {
		items[0].op, items[0].label = keybindings.OpCassetteView, "DECK"
	}
	for _, item := range items {
		if dashboard && item.op == keybindings.OpEnter {
			item.label = "OPEN"
		}
		keys := configs.EffectiveKeybindings[item.op]
		preferred := map[keybindings.OperateType]string{
			keybindings.OpGoBack: "esc", keybindings.OpMoveUp: "up", keybindings.OpEnter: "enter",
		}[item.op]
		if op, ok := handler.keyToOperateMap[preferred]; ok && op == item.op {
			keys = []string{preferred}
		}
		for _, key := range keys {
			if handler.keyToOperateMap[key] == item.op {
				hints = append(hints, cassette.Hint{Key: keybindings.FormatKeyForDisplay(key), Label: item.label})
				break
			}
		}
	}
	return hints
}

func (p *cassettePage) View(a *model.App) string {
	s := p.netease.cassetteSnapshot(p.frame, p.selected)
	s.Hints = p.hints()
	return cassette.Render(s, a.WindowWidth(), a.WindowHeight())
}

func (n *Netease) cassetteSnapshot(frame, selected int) cassette.Snapshot {
	player := n.player
	song := player.CurSong()
	state := n.lyricService.State()
	lines, index := cassetteLyrics(state)
	status := "STOPPED"
	switch player.State() {
	case types.Playing:
		status = "PLAYING"
	case types.Paused:
		status = "PAUSED"
	case types.Interrupted:
		status = "BUFFERING"
	}
	s := cassette.Snapshot{
		Track: cassette.Track{Title: song.Name, Artist: song.ArtistName()},
		Album: song.Album.Name, Quality: "目标 " + qualityDisplayName(configs.AppConfig.Player.SongLevel),
		Mode: player.Mode().String(), Status: status,
		Position: player.PassedTime(), Duration: song.Duration, Volume: player.Volume(),
		Current: player.CurSongIndex(), Selected: selected, Frame: frame,
		Lyrics: lines, LyricIndex: index,
	}
	if !state.IsRunning {
		s.LyricMessage = "歌词暂不可用 / 等待加载"
	}
	if song.Id == 0 {
		s.LyricMessage = "按 Esc 返回，登录或选择歌单后开始播放"
	}
	for _, song := range player.Playlist() {
		s.Queue = append(s.Queue, cassette.Track{Title: song.Name, Artist: song.ArtistName()})
	}
	s.Palette = cassettePalette()
	return s
}
