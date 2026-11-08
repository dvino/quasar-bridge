package main

import (
	"fmt"
	"os"
	"path/filepath"

	"quasarbridge/internal/bridge"
	"quasarbridge/internal/companion"
	"quasarbridge/internal/llm"
)

// configureCompanion owns the optional module's process configuration.
// It does not create directories, migrate data or contact model providers.
func configureCompanion(e *bridge.Engine, client *llm.Client) error {
	if path := os.Getenv("QUASAR_LOCAL_COMMAND"); path != "" {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("QUASAR_LOCAL_COMMAND must be an absolute path")
		}
		e.LocalCommand = bridge.ExecutableCommand{Path: path}
	}
	e.ExplicitConversations = os.Getenv("QUASAR_EXPLICIT_DIALOGUE") == "1"
	if e.ExplicitConversations && os.Getenv("QUASAR_SEMANTIC_DIALOGUE") == "1" {
		classifier := *client
		if model := os.Getenv("QUASAR_DIALOGUE_CLASSIFIER_MODEL"); model != "" {
			classifier.Model = model
		}
		if effort := os.Getenv("QUASAR_DIALOGUE_CLASSIFIER_EFFORT"); effort != "" {
			classifier.ReasoningEffort = effort
		}
		classifier.HostedTools = nil
		e.DialogueClassifier = &classifier
		e.SelectDialogueModels = os.Getenv("QUASAR_DIALOGUE_MODEL_ROUTING") == "1"
		e.DialogueModels = map[string]string{
			"luna":  os.Getenv("QUASAR_DIALOGUE_FAST_MODEL"),
			"terra": os.Getenv("QUASAR_DIALOGUE_REASONING_MODEL"),
			"sol":   os.Getenv("QUASAR_DIALOGUE_ADVANCED_MODEL"),
		}
	}
	for _, setting := range []struct {
		name   string
		target **companion.Profile
	}{{"QUASAR_FAMILY_PROFILE", &e.SharedProfile}, {"QUASAR_ADULT_PROFILE", &e.AdultProfile}} {
		if path := os.Getenv(setting.name); path != "" {
			profile, err := companion.LoadProfile(path)
			if err != nil {
				return fmt.Errorf("invalid %s", setting.name)
			}
			*setting.target = &profile
		}
	}
	if dir := os.Getenv("QUASAR_LEARNED_MEMORY_DIR"); dir != "" {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("QUASAR_LEARNED_MEMORY_DIR must be an existing directory")
		}
		memory, err := companion.NewLearnedMemory(map[string]string{
			"adult":  filepath.Join(dir, "adult.json"),
			"child":  filepath.Join(dir, "child.json"),
			"family": filepath.Join(dir, "family.json"),
		})
		if err != nil {
			return fmt.Errorf("invalid QUASAR_LEARNED_MEMORY_DIR")
		}
		e.LearnedMemory = memory
	}
	if os.Getenv("QUASAR_CHILD_COMPANION") != "1" {
		return nil
	}
	e.Companion = companion.New()
	e.Companion.SetLearnedMemory(e.LearnedMemory)
	if path := os.Getenv("QUASAR_CHILD_PREFERENCES"); path != "" {
		if err := e.Companion.EnablePreferences(path); err != nil {
			return fmt.Errorf("invalid QUASAR_CHILD_PREFERENCES")
		}
	}
	if path := os.Getenv("QUASAR_CHILD_PROFILE"); path != "" {
		profile, err := companion.LoadProfile(path)
		if err != nil {
			return fmt.Errorf("invalid QUASAR_CHILD_PROFILE")
		}
		if e.SharedProfile == nil {
			e.SharedProfile = &profile
		}
		if err := e.Companion.SetProfile(profile); err != nil {
			return fmt.Errorf("invalid child profile")
		}
	}
	if e.SharedProfile != nil {
		if err := e.Companion.SetSharedProfile(*e.SharedProfile); err != nil {
			return fmt.Errorf("invalid shared family context")
		}
	}
	return nil
}
