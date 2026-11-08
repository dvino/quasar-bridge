package bridge

import (
	"context"
	"strings"
	"testing"

	"quasarbridge/internal/companion"
	"quasarbridge/internal/dialogue"
	"quasarbridge/internal/llm"
)

func TestSharedPetContextDoesNotImportChildProfileForAdult(t *testing.T) {
	family := &companion.Profile{Base: "CHILD_BASE_PRIVATE", Modules: []companion.Module{
		{ID: "rats", Scope: "family", Triggers: []string{"крыс"}, Content: "Шоколадка — вожак стаи"},
		{ID: "puzzles", Scope: "child", Triggers: []string{"крыс"}, Content: "CHILD_PREFERENCE_PRIVATE"},
	}}
	adult := &companion.Profile{Base: "ADULT_PREFERENCE_PRIVATE"}
	for _, mode := range []voiceAge{ageAdult, ageChild, ageUnknown} {
		t.Run(string(mode), func(t *testing.T) {
			e := &Engine{SharedProfile: family, AdultProfile: adult}
			e.LLM = companionFixture(func(_ context.Context, m []llm.Message, structured bool) (string, error) {
				all := ""
				for _, v := range m {
					all += v.Content
				}
				if !strings.Contains(all, "Шоколадка") {
					t.Fatal("shared pet context missing")
				}
				if strings.Contains(all, "CHILD_BASE_PRIVATE") || strings.Contains(all, "CHILD_PREFERENCE_PRIVATE") {
					t.Fatal("private child profile leaked")
				}
				if strings.Contains(all, "ADULT_PREFERENCE_PRIVATE") != (mode == ageAdult) {
					t.Fatal("adult profile applied without adult classification")
				}
				if !strings.Contains(m[0].Content, "до трёх предложений") {
					t.Fatal("adult brevity lost")
				}
				return "Шоколадка — вожак.", nil
			})
			ctx := context.WithValue(dialogue.WithSpeaker(context.Background(), "device"), voiceAgeKey{}, ageContext{mode: mode, adultMale: mode == ageAdult})
			if _, err := e.Fallback(ctx, "расскажи про наших крыс"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
