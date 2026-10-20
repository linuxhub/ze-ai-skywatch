package settings

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/linuxhub/ze-ai-skywatch/internal/ai"
	"github.com/linuxhub/ze-ai-skywatch/internal/ipnotes"
)

type Settings struct {
	IPNotes          []ipnotes.Note `json:"ipNotes,omitempty"`
	AutoRefresh      bool           `json:"autoRefresh"`
	Interval         int            `json:"intervalSeconds"`
	Tray             bool           `json:"menuBar"`
	StartHidden      bool           `json:"startHidden"`
	Providers        []ai.Provider  `json:"aiProviders,omitempty"`
	ActiveProvider   string         `json:"activeAIProvider,omitempty"`
	NetworkAnimation bool           `json:"networkAnimation"`
	NetworkGeo       bool           `json:"networkGeoLookup"`
}

func Default() Settings { return Settings{AutoRefresh: true, Interval: 2, Tray: true} }

func Load(path string) (Settings, error) {
	s := Default()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(data, &s); err != nil {
		return Default(), err
	}
	if s.Interval != 1 && s.Interval != 2 && s.Interval != 5 && s.Interval != 10 {
		s.Interval = 2
	}
	return s, nil
}

func Save(path string, s Settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "settings-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
