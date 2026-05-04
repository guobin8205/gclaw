package tui

import "testing"

func TestLoadThemeTokyoNight(t *testing.T) {
	th := LoadTheme("tokyo-night")
	if th.Name != "tokyo-night" {
		t.Errorf("expected tokyo-night, got %s", th.Name)
	}
	if th.Text != "#c0caf5" {
		t.Errorf("expected #c0caf5, got %s", th.Text)
	}
}

func TestLoadThemeCatppuccinMocha(t *testing.T) {
	th := LoadTheme("catppuccin-mocha")
	if th.Name != "catppuccin-mocha" {
		t.Errorf("expected catppuccin-mocha, got %s", th.Name)
	}
	if th.Text != "#cdd6f4" {
		t.Errorf("expected #cdd6f4, got %s", th.Text)
	}
}

func TestLoadThemeUnknownFallsBack(t *testing.T) {
	th := LoadTheme("unknown")
	if th.Name != "tokyo-night" {
		t.Errorf("expected fallback tokyo-night, got %s", th.Name)
	}
}

func TestThemeStyles(t *testing.T) {
	th := LoadTheme("tokyo-night")
	s := th.Styles()
	_ = s.UserText
	_ = s.Prompt
}
