package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-musicfox/go-musicfox/internal/configs"
	desktop "github.com/go-musicfox/go-musicfox/internal/desktop_lyrics"
	"github.com/go-musicfox/go-musicfox/internal/lyric"
	"github.com/go-musicfox/go-musicfox/internal/structs"
	"github.com/go-musicfox/go-musicfox/internal/types"
)

type desktopLyricSink struct {
	desktop.Controller
	visible       bool
	current, next desktop.LyricLine
}

func (s *desktopLyricSink) Show()                     { s.visible = true }
func (s *desktopLyricSink) Hide()                     { s.visible = false }
func (s *desktopLyricSink) IsVisible() bool           { return s.visible }
func (s *desktopLyricSink) SetSpectrumAvailable(bool) {}
func (s *desktopLyricSink) Update(cur, next desktop.LyricLine, _ int, _ int64, _ bool) {
	s.current, s.next = cur, next
}

type desktopLyricFetcher struct{ fail bool }

func (f *desktopLyricFetcher) GetLyric(context.Context, structs.Song) (structs.LRCData, error) {
	if f.fail {
		return structs.LRCData{}, errors.New("offline")
	}
	return structs.LRCData{Original: "[00:00.00]第一句\n[00:02.00]第二句", Translated: "[00:00.00]first\n[00:02.00]second", Yrc: "[0,2000](0,2000,0)第一句\n[2000,2000](2000,2000,0)第二句"}, nil
}

func TestDesktopLyricsFollowPlaybackAndClearMissingSong(t *testing.T) {
	_, n, engine := newCassetteDashboardFixture(t, "top")
	sink := &desktopLyricSink{}
	n.desktopLyrics = sink
	n.player.netease = n
	fetcher := &desktopLyricFetcher{}
	n.lyricService = lyric.NewService(fetcher, true, 0, true)
	if err := n.lyricService.SetSong(context.Background(), structs.Song{Id: 1}); err != nil {
		t.Fatal(err)
	}
	engine.state = types.Playing
	n.lyricService.UpdatePosition(time.Second)
	n.player.updateDesktopLyrics()
	if !sink.visible || sink.current.Text != "第一句\nfirst" || sink.next.Text != "第二句\nsecond" {
		t.Fatalf("incorrect initial lyrics: %+v", sink)
	}
	n.lyricService.EnableYRC(true)
	n.lyricService.UpdatePosition(3 * time.Second)
	n.player.updateDesktopLyrics()
	if sink.current.Text != "第二句\nsecond" || len(sink.current.Words) != 1 || sink.next.Text != "" {
		t.Fatalf("YRC line did not advance: %+v", sink)
	}
	engine.state = types.Paused
	n.player.updateDesktopLyrics()
	if !sink.visible {
		t.Fatal("pause unexpectedly hid lyrics")
	}
	configs.AppConfig.Main.Lyric.DesktopLyrics.HideOnPause = true
	n.player.updateDesktopLyrics()
	if sink.visible {
		t.Fatal("hide-on-pause ignored")
	}
	engine.state = types.Playing
	n.player.updateDesktopLyrics()
	if !sink.visible {
		t.Fatal("resume did not restore lyrics")
	}
	engine.state = types.Stopped
	n.player.updateDesktopLyrics()
	if sink.visible {
		t.Fatal("stopped playback retained overlay")
	}
	engine.state = types.Playing
	fetcher.fail = true
	_ = n.lyricService.SetSong(context.Background(), structs.Song{Id: 2})
	n.player.updateDesktopLyrics()
	if sink.visible || sink.current.Text != "" || sink.next.Text != "" {
		t.Fatal("missing song retained stale lyrics")
	}
}
