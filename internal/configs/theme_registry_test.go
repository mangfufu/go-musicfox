package configs

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/anhoder/foxful-cli/style"
)

func TestThemeRegistryCurrentThemePairUsesRuntimeSelection(t *testing.T) {
	registry := ThemeRegistry{themes: map[string]*ThemeFile{
		"Default": {
			Name:  "Default",
			Dark:  ThemeVariant{Primary: "#101010"},
			Light: ThemeVariant{Primary: "#F0F0F0"},
		},
		"Ocean": {
			Name:  "Ocean",
			Dark:  ThemeVariant{Primary: "#112233"},
			Light: ThemeVariant{Primary: "#AABBCC"},
		},
	}}
	registry.rebuildIndex()
	registry.SelectTheme("Ocean", false)

	dark, light, ok := registry.CurrentThemePair()
	if !ok {
		t.Fatal("CurrentThemePair returned no theme")
	}
	assertThemeColor(t, dark.Primary, parseColor("#112233"), "dark primary")
	assertThemeColor(t, light.Primary, parseColor("#AABBCC"), "light primary")
}

func assertThemeColor(t *testing.T, got, want color.Color, label string) {
	t.Helper()
	gotR, gotG, gotB, gotA := got.RGBA()
	wantR, wantG, wantB, wantA := want.RGBA()
	if gotR != wantR || gotG != wantG || gotB != wantB || gotA != wantA {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
}

func TestBuiltinThemesStatusBarFontsUseConfiguredOrDefaultColors(t *testing.T) {
	themes := loadBuiltinThemes()
	defaultTheme, ok := themes["Default"]
	if !ok {
		t.Fatal("Default theme is missing")
	}

	variants := []struct {
		name     string
		defaultV ThemeVariant
	}{
		{name: "dark", defaultV: defaultTheme.Dark},
		{name: "light", defaultV: defaultTheme.Light},
	}
	for themeName, themeFile := range themes {
		for _, variant := range variants {
			actual := themeFile.Dark
			if variant.name == "light" {
				actual = themeFile.Light
			}
			want := style.NewStyleSet(variant.defaultV.toTheme())
			got := style.NewStyleSet(actual.toTheme())
			breadcrumb, clock := want.StatusBarBreadcrumb.GetForeground(), want.StatusBarTime.GetForeground()
			if actual.StatusBar.Breadcrumb != "" {
				breadcrumb = parseColor(actual.StatusBar.Breadcrumb)
			}
			if actual.StatusBar.Time != "" {
				clock = parseColor(actual.StatusBar.Time)
			}
			assertThemeColor(t, got.StatusBarBreadcrumb.GetForeground(), breadcrumb, themeName+" "+variant.name+" breadcrumb")
			assertThemeColor(t, got.StatusBarTime.GetForeground(), clock, themeName+" "+variant.name+" time")
		}
	}
}

func TestTransparentThemeConfiguresPrimaryBreadcrumbLabel(t *testing.T) {
	transparent, ok := loadBuiltinThemes()["Transparent"]
	if !ok {
		t.Fatal("Transparent theme is missing")
	}

	variants := []struct {
		name  string
		value ThemeVariant
	}{
		{name: "dark", value: transparent.Dark},
		{name: "light", value: transparent.Light},
	}
	for _, variant := range variants {
		styles := style.NewStyleSet(variant.value.toTheme())
		assertThemeColor(t, styles.StatusBarNuggetLabel.GetForeground(), parseColor("#EA403F"), variant.name+" breadcrumb marker foreground")
		assertThemeColor(t, styles.StatusBarBreadcrumbHover.GetForeground(), parseColor(variant.value.Primary), variant.name+" breadcrumb hover foreground")
		if _, ok := styles.StatusBarNuggetLabel.GetBackground().(lipgloss.NoColor); !ok {
			t.Fatalf("%s breadcrumb marker background = %v, want transparent", variant.name, styles.StatusBarNuggetLabel.GetBackground())
		}
	}
}

func TestCassetteThemesLoadFromEmbeddedFilesystem(t *testing.T) {
	themes := loadBuiltinThemes()
	for _, name := range []string{"Cassette", "Midnight Tape", "Paper Tape"} {
		theme, ok := themes[name]
		if !ok {
			t.Fatalf("embedded theme %q is missing", name)
		}
		for _, v := range []ThemeVariant{theme.Dark, theme.Light} {
			if v.App.Background == "" || v.App.Background != v.Background || v.Foreground == v.Background {
				t.Fatalf("theme %q must paint a readable, consistent application background", name)
			}
			assertThemeColor(t, style.NewStyleSet(v.toTheme()).AppBackground.GetBackground(), parseColor(v.Background), name)
			assertThemeColor(t, themeStyleSet(v.toTheme()).MenuItem.GetForeground(), parseColor(v.Foreground), name+" menu foreground")
		}
	}
}
