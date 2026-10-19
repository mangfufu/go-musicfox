package configs

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestSetTOMLValuesAtomicAndPreservesOtherSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := "# Keep my settings\n[main.lyric.desktopLyrics]\nfontSize = 36 # pixels\ncustomSetting = 'keep'\n[theme]\nactiveTheme = 'Cassette'\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SetTOMLValues(path, []TOMLUpdate{{[]string{"main", "lyric", "desktopLyrics", "fontSize"}, 48}, {nil, true}}); err == nil {
		t.Fatal("invalid edit accepted")
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatal("failed transaction changed the file")
	}
	var group sync.WaitGroup
	for _, update := range []TOMLUpdate{{[]string{"main", "lyric", "desktopLyrics", "fontSize"}, 48}, {[]string{"theme", "activeTheme"}, "Midnight Tape"}} {
		group.Add(1)
		go func(update TOMLUpdate) {
			defer group.Done()
			if err := SetTOMLValues(path, []TOMLUpdate{update}); err != nil {
				t.Error(err)
			}
		}(update)
	}
	group.Wait()
	data, _ = os.ReadFile(path)
	var parsed Config
	if err := toml.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{"# Keep my settings", "fontSize = 48 # pixels", "customSetting = 'keep'", "activeTheme = \"Midnight Tape\""} {
		if !strings.Contains(content, want) {
			t.Fatalf("lost setting or comment %q: %s", want, content)
		}
	}
}
