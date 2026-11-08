package companion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quasarbridge/internal/llm"
)

func TestProfileRoundTripAndModuleSelection(t *testing.T) {
	p := Profile{Base: "общий стиль", Modules: []Module{{ID: "rats", Triggers: []string{"крыс", "питомец"}, Content: "Крыса игривая"}, {ID: "explain", Triggers: []string{"почему"}, Content: "объясняй понятно"}}}
	path := filepath.Join(t.TempDir(), "memory.json")
	if err := SaveProfile(path, p); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions=%v", info.Mode())
	}
	got := loaded.selectContext("почему питомец так делает", nil)
	if !strings.Contains(got, "игривая") || !strings.Contains(got, "объясняй понятно") {
		t.Fatal(got)
	}
	if got := loaded.selectContext("о космосе", nil); strings.Contains(got, "игривая") {
		t.Fatal("unrelated module selected")
	}
	history := []llm.Message{{Role: "user", Content: "питомец"}}
	if got := loaded.selectContext("а она?", history); !strings.Contains(got, "игривая") {
		t.Fatal("lost immediate follow-up")
	}
	for i := 0; i < 4; i++ {
		history = append(history, llm.Message{Role: "user", Content: "космос"})
	}
	if got := loaded.selectContext("а звёзды?", history); strings.Contains(got, "игривая") {
		t.Fatal("old module stuck")
	}
	// Editing/deleting context replaces the file, not append-only history.
	if err := SaveProfile(path, Profile{Base: "новый стиль"}); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Modules) != 0 || loaded.Base != "новый стиль" {
		t.Fatal("old profile retained")
	}
}

func TestMalformedProfileRejectedWithoutOverwritingGoodFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	good := Profile{Base: "сохранить"}
	if err := SaveProfile(path, good); err != nil {
		t.Fatal(err)
	}
	if err := SaveProfile(path, Profile{Base: strings.Repeat("x", 3001)}); err == nil {
		t.Fatal("accepted oversized base")
	}
	loaded, err := LoadProfile(path)
	if err != nil || loaded.Base != "сохранить" {
		t.Fatal("good profile lost")
	}
	for _, raw := range []string{`{"base":"ok","private_notes":"secret"}`, `{"base":"ok"} {}`, strings.Repeat("x", 64001)} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadProfile(path); err == nil {
			t.Fatal("accepted malformed file")
		}
	}
}

func TestSetProfileCopiesCallerMemory(t *testing.T) {
	s := New()
	p := Profile{Modules: []Module{{ID: "rats", Triggers: []string{"крыс"}, Content: "безопасный контекст"}}}
	if err := s.SetProfile(p); err != nil {
		t.Fatal(err)
	}
	p.Modules[0].Triggers[0] = "иной"
	p.Modules[0].Content = "подмена"
	s.mu.Lock()
	got := s.profile.selectContext("крысы", nil)
	s.mu.Unlock()
	if !strings.Contains(got, "безопасный контекст") || strings.Contains(got, "подмена") {
		t.Fatal("profile aliases caller")
	}
}
