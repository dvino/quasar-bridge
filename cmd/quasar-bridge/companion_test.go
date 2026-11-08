package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quasarbridge/internal/bridge"
	"quasarbridge/internal/llm"
)

func clearCompanionEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"QUASAR_EXPLICIT_DIALOGUE", "QUASAR_SEMANTIC_DIALOGUE", "QUASAR_CHILD_COMPANION",
		"QUASAR_FAMILY_PROFILE", "QUASAR_CHILD_PROFILE", "QUASAR_ADULT_PROFILE",
		"QUASAR_CHILD_PREFERENCES", "QUASAR_LEARNED_MEMORY_DIR", "QUASAR_DIALOGUE_MODEL_ROUTING", "QUASAR_LOCAL_COMMAND",
		"QUASAR_DIALOGUE_CLASSIFIER_MODEL", "QUASAR_DIALOGUE_CLASSIFIER_EFFORT",
		"QUASAR_DIALOGUE_FAST_MODEL", "QUASAR_DIALOGUE_REASONING_MODEL", "QUASAR_DIALOGUE_ADVANCED_MODEL",
	} {
		t.Setenv(name, "")
	}
}

func TestCompanionConfigurationDefaultsOff(t *testing.T) {
	clearCompanionEnvironment(t)
	e := &bridge.Engine{}
	if err := configureCompanion(e, &llm.Client{}); err != nil {
		t.Fatal(err)
	}
	if e.ExplicitConversations || e.Companion != nil || e.DialogueClassifier != nil || e.LearnedMemory != nil || e.SharedProfile != nil || e.AdultProfile != nil || e.LocalCommand != nil {
		t.Fatal("optional features enabled by default")
	}
}

func TestCompanionClassifierConfigurationDoesNotMutateAnswerClient(t *testing.T) {
	clearCompanionEnvironment(t)
	t.Setenv("QUASAR_EXPLICIT_DIALOGUE", "1")
	t.Setenv("QUASAR_SEMANTIC_DIALOGUE", "1")
	t.Setenv("QUASAR_DIALOGUE_CLASSIFIER_MODEL", "test-classifier")
	t.Setenv("QUASAR_DIALOGUE_CLASSIFIER_EFFORT", "low")
	t.Setenv("QUASAR_DIALOGUE_MODEL_ROUTING", "1")
	t.Setenv("QUASAR_DIALOGUE_REASONING_MODEL", "test-reasoning")
	client := &llm.Client{Model: "test-answer", ReasoningEffort: "medium"}
	e := &bridge.Engine{}
	if err := configureCompanion(e, client); err != nil {
		t.Fatal(err)
	}
	classifier, ok := e.DialogueClassifier.(*llm.Client)
	if !ok || classifier == client || classifier.Model != "test-classifier" || classifier.ReasoningEffort != "low" || len(classifier.HostedTools) != 0 {
		t.Fatal("classifier configuration not isolated")
	}
	if client.Model != "test-answer" || client.ReasoningEffort != "medium" || !e.SelectDialogueModels || e.DialogueModels["terra"] != "test-reasoning" || e.DialogueModels["luna"] != "" {
		t.Fatal("model routing configuration changed defaults")
	}
}

func TestCompanionConfigurationRejectsInvalidProfileWithoutLeakingPath(t *testing.T) {
	clearCompanionEnvironment(t)
	path := filepath.Join(t.TempDir(), "private-profile.json")
	if err := os.WriteFile(path, []byte(`{"unexpected":"private data"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUASAR_FAMILY_PROFILE", path)
	err := configureCompanion(&bridge.Engine{}, &llm.Client{})
	if err == nil || !strings.Contains(err.Error(), "QUASAR_FAMILY_PROFILE") || strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "private data") {
		t.Fatalf("unexpected configuration error: %v", err)
	}
}

func TestLocalCommandRequiresAbsolutePath(t *testing.T) {
	clearCompanionEnvironment(t)
	t.Setenv("QUASAR_LOCAL_COMMAND", "relative-handler")
	if err := configureCompanion(&bridge.Engine{}, &llm.Client{}); err == nil {
		t.Fatal("relative local command accepted")
	}
	t.Setenv("QUASAR_LOCAL_COMMAND", "/example/handler")
	e := &bridge.Engine{}
	if err := configureCompanion(e, &llm.Client{}); err != nil || e.LocalCommand == nil {
		t.Fatalf("absolute local command rejected: %v", err)
	}
}

func TestCompanionMemoryDirectoryMustExist(t *testing.T) {
	clearCompanionEnvironment(t)
	t.Setenv("QUASAR_LEARNED_MEMORY_DIR", filepath.Join(t.TempDir(), "absent"))
	if err := configureCompanion(&bridge.Engine{}, &llm.Client{}); err == nil {
		t.Fatal("missing memory directory accepted")
	}
}

func TestCompanionMemoryAndGamesRequireIndependentOptIn(t *testing.T) {
	clearCompanionEnvironment(t)
	dir := t.TempDir()
	t.Setenv("QUASAR_LEARNED_MEMORY_DIR", dir)
	t.Setenv("QUASAR_CHILD_COMPANION", "1")
	e := &bridge.Engine{}
	if err := configureCompanion(e, &llm.Client{}); err != nil {
		t.Fatal(err)
	}
	if e.Companion == nil || e.LearnedMemory == nil {
		t.Fatal("explicitly enabled module not initialized")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("configuration wrote memory files")
	}
}
