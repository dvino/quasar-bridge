package companion

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLearningOptOutPersistsWithoutDeleting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "child.json")
	m, err := NewLearnedMemory(map[string]string{"child": path})
	if err != nil {
		t.Fatal(err)
	}
	fact := LearnedFact{Topic: "игры", Key: "загадки", Value: "любит загадки", Kind: "fact", Source: "child"}
	if err := m.Apply("child", []LearnedFact{fact}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetLearning("child", false); err != nil {
		t.Fatal(err)
	}
	m, err = NewLearnedMemory(map[string]string{"child": path})
	if err != nil {
		t.Fatal(err)
	}
	if enabled, err := m.LearningEnabled("child"); err != nil || enabled {
		t.Fatal("opt-out lost")
	}
	fact.Value = "любит ребусы"
	if err := m.Apply("child", []LearnedFact{fact}); err != nil {
		t.Fatal(err)
	}
	facts, _ := m.Snapshot("child")
	if len(facts) != 1 || facts[0].Value != "любит загадки" {
		t.Fatal("disabled learning changed memory")
	}
	if err := m.Clear("child"); err != nil {
		t.Fatal(err)
	}
	if enabled, _ := m.LearningEnabled("child"); enabled {
		t.Fatal("clear re-enabled learning")
	}
}

func TestLearnedMemorySeparatesScopesAndRespectsExternalEdits(t *testing.T) {
	dir := t.TempDir()
	paths := map[string]string{"adult": filepath.Join(dir, "adult.json"), "child": filepath.Join(dir, "child.json"), "family": filepath.Join(dir, "family.json")}
	m, err := NewLearnedMemory(paths)
	if err != nil {
		t.Fatal(err)
	}
	fact := LearnedFact{Topic: "походы", Key: "интерес", Value: "нравятся велопоездки", Kind: "fact", Source: "adult"}
	if err := m.Apply("adult", []LearnedFact{fact}); err != nil {
		t.Fatal(err)
	}
	child, err := m.Snapshot("child")
	if err != nil || len(child) != 0 {
		t.Fatal("adult memory leaked")
	}
	if err := m.Apply("child", []LearnedFact{fact}); err == nil {
		t.Fatal("wrong source accepted")
	}
	fact.Value = "предпочитает пешие походы"
	fact.Kind = "hypothesis"
	if err := m.Apply("adult", []LearnedFact{fact}); err != nil {
		t.Fatal(err)
	}
	got, err := m.Snapshot("adult")
	if err != nil || len(got) != 1 || got[0].Value != fact.Value {
		t.Fatal("update failed")
	}
	if err := os.WriteFile(paths["adult"], []byte(`{"facts":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = m.Snapshot("adult")
	if err != nil || len(got) != 0 {
		t.Fatal("manual edit ignored")
	}
	if err := os.WriteFile(paths["adult"], []byte(`{"facts":[],"unknown":"private"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Apply("adult", []LearnedFact{fact}); err == nil {
		t.Fatal("invalid file overwritten")
	}
}
