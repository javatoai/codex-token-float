package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSettingsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	defaults := desktopSettings{RefreshSeconds: 2, ShowUSD: true}
	if got := loadSettings(path); got != defaults {
		t.Fatalf("defaults = %+v, want %+v", got, defaults)
	}
	for _, test := range []struct {
		name, contents string
		want           desktopSettings
	}{
		{"malformed", `{"refreshSeconds":5,`, defaults},
		{"invalid interval", `{"refreshSeconds":3,"showUSD":false}`, desktopSettings{RefreshSeconds: 2, ShowUSD: false}},
		{"absent visibility", `{"refreshSeconds":1}`, desktopSettings{RefreshSeconds: 1, ShowUSD: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(test.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := loadSettings(path); got != test.want {
				t.Fatalf("loaded %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestSaveSettingsReplacesPreviousSelection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	for _, next := range []desktopSettings{
		{RefreshSeconds: 5, ShowUSD: false},
		{RefreshSeconds: 1, ShowUSD: true},
	} {
		if err := saveSettings(path, next); err != nil {
			t.Fatal(err)
		}
		if got := loadSettings(path); got != next {
			t.Fatalf("loaded %+v, want %+v", got, next)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "settings.json" {
		t.Fatalf("unexpected files after saving settings: %v, %v", entries, err)
	}
}
