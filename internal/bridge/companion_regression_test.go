package bridge

import (
	"context"
	"strings"
	"testing"

	"quasarbridge/internal/companion"
	"quasarbridge/internal/dialogue"
	"quasarbridge/internal/llm"
)

func TestInitialChildUtilityCommandsDoNotStartGame(t *testing.T) {
	e := &Engine{Companion: companion.New()}
	ctx := context.WithValue(dialogue.WithSpeaker(context.Background(), "device"), voiceAgeKey{}, ageContext{mode: ageChild})
	for _, text := range []string{"включи музыку", "поставь таймер на пять минут", "какая погода завтра", "который час"} {
		if d := e.Decide(ctx, text); d.Source == "companion" {
			t.Fatalf("utility hijacked: %q", text)
		}
	}
	if d := e.Decide(ctx, "давай расскажем смешную сказку"); d.Source != "companion" {
		t.Fatal("story handed to native assistant")
	}
}

func TestMaleProfileDialogueDoesNotCarryIntoUnclassifiedAdult(t *testing.T) {
	e := &Engine{AdultProfile: &companion.Profile{Base: "PRIVATE_MALE_PROFILE"}}
	turn := 0
	e.LLM = companionFixture(func(_ context.Context, m []llm.Message, _ bool) (string, error) {
		if turn == 0 {
			turn++
			return "PRIVATE_MALE_REPLY", nil
		}
		for _, message := range m {
			if strings.Contains(message.Content, "PRIVATE_MALE") {
				t.Fatal("male profile or history leaked")
			}
		}
		return "Обычный ответ.", nil
	})
	base := dialogue.WithSpeaker(context.Background(), "device")
	male := context.WithValue(dialogue.WithAudience(base, voiceAudience(ageAdult, true)), voiceAgeKey{}, ageContext{mode: ageAdult, adultMale: true})
	other := context.WithValue(dialogue.WithAudience(base, voiceAudience(ageAdult, false)), voiceAgeKey{}, ageContext{mode: ageAdult})
	if _, err := e.Fallback(male, "вопрос"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Fallback(other, "привет"); err != nil {
		t.Fatal(err)
	}
	if dialogue.FamilyKey(male) != dialogue.FamilyKey(other) {
		t.Fatal("audience change lost device family scope")
	}
}
