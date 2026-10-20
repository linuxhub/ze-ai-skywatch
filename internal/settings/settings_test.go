package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/linuxhub/ze-ai-skywatch/internal/ai"
)

func TestProviderRoundTripAndLegacySettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"autoRefresh":false,"intervalSeconds":5,"menuBar":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil || s.Interval != 5 || s.AutoRefresh || len(s.Providers) != 0 {
		t.Fatalf("legacy: %+v %v", s, err)
	}
	s.Providers = []ai.Provider{{ID: "one", Name: "local", Protocol: ai.OpenAI, BaseURL: "http://localhost:1234/v1", APIKey: "test-key", Model: "custom"}}
	s.ActiveProvider = "one"
	s.NetworkAnimation = true
	s.NetworkGeo = true
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || len(got.Providers) != 1 || got.Providers[0] != s.Providers[0] || got.ActiveProvider != "one" {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if !got.NetworkAnimation || !got.NetworkGeo {
		t.Fatal("network preferences did not persist")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private file permissions: %v %v", info, err)
	}
}
