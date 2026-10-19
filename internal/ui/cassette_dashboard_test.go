package ui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/anhoder/foxful-cli/model"
	"github.com/charmbracelet/x/ansi"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	"github.com/go-musicfox/go-musicfox/internal/keybindings"
	"github.com/go-musicfox/go-musicfox/internal/lyric"
	"github.com/go-musicfox/go-musicfox/internal/playlist"
	"github.com/go-musicfox/go-musicfox/internal/structs"
	"github.com/go-musicfox/go-musicfox/internal/types"
)

type searchableCassetteMenu struct{ *MainMenu }

func (*searchableCassetteMenu) IsSearchable() bool { return true }

func newCassetteDashboardFixture(t *testing.T, status string) (*model.App, *Netease, *cassetteTestPlayer) {
	t.Helper()
	previous, bindings := configs.AppConfig, configs.EffectiveKeybindings
	t.Cleanup(func() { configs.AppConfig, configs.EffectiveKeybindings = previous, bindings })
	configs.AppConfig = &configs.Config{Theme: configs.ThemeConfig{Layout: "cassette", ActiveTheme: "Cassette", DoubleColumn: true, StatusBar: true, StatusBarPosition: status}}
	configs.EffectiveKeybindings = keybindings.BuildEffectiveBindings(nil, true)
	engine := &cassetteTestPlayer{state: types.Paused, position: 24 * time.Second}
	n := &Netease{player: &Player{Player: engine, playlistManager: playlist.NewPlaylistManager()}, lyricService: lyric.NewService(nil, true, 0, true)}
	_ = n.player.playlistManager.Initialize(0, []structs.Song{{Id: 1, Name: "测试磁带", Duration: 4 * time.Minute}})
	opts := model.DefaultOptions()
	configs.AppConfig.FillToModelOpts(opts)
	opts.EnableStartup = false
	menu := NewMainMenu(n)
	// Keep this fixture offline; Last.fm authorization is outside layout tests.
	menu.menuList[13] = nil
	opts.MainMenu = &searchableCassetteMenu{menu}
	opts.MainMenuTitle = &model.MenuItem{Title: "音乐库"}
	opts.LocalSearchMenu = NewLocalSearchMenu(n)
	opts.Components = n.Components()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opts.TeaOptions = []tea.ProgramOption{tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard)}
	a := model.NewApp(opts)
	n.App = a
	_ = a.Run()
	return a, n, engine
}

func TestCassetteDashboardMenuSearchAndSeek(t *testing.T) {
	for _, status := range []string{"top", "bottom"} {
		t.Run(status, func(t *testing.T) {
			a, n, _ := newCassetteDashboardFixture(t, status)
			for _, size := range [][2]int{{120, 40}, {100, 30}, {80, 24}, {60, 24}, {40, 20}} {
				a.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				main := a.MustMain()
				for _, searching := range []bool{false, true} {
					if searching {
						main.Update(tea.KeyPressMsg{Code: '/', Text: "/"}, a)
					}
					view := main.View(a)
					rows := strings.Split(ansi.Strip(view), "\n")
					if searching && strings.Contains(ansi.Strip(view), "MUSIC LIBRARY") {
						t.Fatal("local search did not replace the library hint")
					}
					if len(rows) != size[1] {
						t.Fatalf("%v search=%v: rows=%d", size, searching, len(rows))
					}
					for i, row := range rows {
						if ansi.StringWidth(row) > size[0] {
							t.Fatalf("%v row %d overflow: %q", size, i, row)
						}
					}
					bottom := main.EffectiveWindowHeight(a)
					if !strings.Contains(rows[bottom-1], "CASSETTE") || !strings.Contains(rows[bottom-2], "DECK") || !strings.Contains(rows[bottom-3], "●") {
						t.Fatalf("%v search=%v footer not anchored:\n%s", size, searching, strings.Join(rows, "\n"))
					}
					handler := NewEventHandler(n)
					margin := 2
					if size[0] < 50 {
						margin = 1
					}
					target, handled := cassetteSeekTarget(tea.Mouse{X: size[0] - margin - 1, Y: bottom - 3}, size[0], bottom, bottom-main.MenuBottomRow()-2, 4*time.Minute)
					if !handled || target != 4*time.Minute {
						t.Fatal("visible progress endpoint did not seek to track end")
					}
					if handled, _, _ := handler.MouseMsgHandle(tea.MouseClickMsg(tea.Mouse{X: main.MenuStartColumn(), Y: main.MenuStartRow(), Button: tea.MouseLeft}), a); handled {
						t.Fatal("deck swallowed a native menu click")
					}
					if searching {
						main.Update(tea.KeyPressMsg{Code: tea.KeyEscape}, a)
					}
				}
				main.SetSelectedIndex(0)
				main.MoveDown()
				want := 1
				if main.IsDualColumn() {
					want = 2
				}
				if main.SelectedIndex() != want {
					t.Fatal("menu navigation broken")
				}
				main.SetSelectedIndex(0)
			}
		})
	}
}

func TestCassetteLocalSearchResultsKeepDashboard(t *testing.T) {
	a, _, _ := newCassetteDashboardFixture(t, "top")
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	main := a.MustMain()
	for _, key := range []tea.KeyPressMsg{{Code: '/', Text: "/"}, {Code: '每', Text: "每"}, {Code: tea.KeyEnter}} {
		main.Update(key, a)
	}
	if _, ok := main.CurMenu().(*LocalSearchMenu); !ok {
		t.Fatal("search did not enter the native results menu")
	}
	rows := strings.Split(ansi.Strip(main.View(a)), "\n")
	if !strings.Contains(rows[39], "CASSETTE") || !strings.Contains(strings.Join(rows, "\n"), "每日") {
		t.Fatal("search results displaced the deck footer or lost their entries")
	}
}
