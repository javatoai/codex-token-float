package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const defaultRefreshSeconds = 2

var refreshChoices = [...]int{1, defaultRefreshSeconds, 5}

type desktopSettings struct {
	RefreshSeconds int  `json:"refreshSeconds"`
	ShowUSD        bool `json:"showUSD"`
}

func defaultSettings() desktopSettings {
	return desktopSettings{RefreshSeconds: defaultRefreshSeconds, ShowUSD: true}
}

func (s desktopSettings) interval() time.Duration {
	return time.Duration(s.RefreshSeconds) * time.Second
}

func loadSettings(path string) desktopSettings {
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultSettings()
	}
	settings := defaultSettings()
	if err := json.Unmarshal(data, &settings); err != nil {
		return defaultSettings()
	}
	if !slices.Contains(refreshChoices[:], settings.RefreshSeconds) {
		settings.RefreshSeconds = defaultRefreshSeconds
	}
	return settings
}

func saveSettings(path string, settings desktopSettings) error {
	file, err := os.CreateTemp(filepath.Dir(path), "settings-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	writeErr := json.NewEncoder(file).Encode(settings)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}
