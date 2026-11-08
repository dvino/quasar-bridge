package bridge

import (
	"context"
	"strings"
	"testing"
)

func TestAutomaticVoiceRespectsAudienceAndDisabledMarkup(t *testing.T) {
	for _, tc := range []struct {
		name          string
		age           ageContext
		enabled, male bool
	}{
		{"owner", ageContext{mode: ageAdult, adultMale: true}, true, true},
		{"child", ageContext{mode: ageChild}, true, false},
		{"unknown", ageContext{}, true, false},
		{"disabled", ageContext{mode: ageAdult, adultMale: true}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &Engine{TTSMarkup: tc.enabled}
			ctx := context.WithValue(t.Context(), voiceAgeKey{}, tc.age)
			instructions := e.ttsInstructions(ctx)
			if strings.Contains(instructions, "постоянный голос — zahar") != tc.male {
				t.Fatal("incorrect default voice")
			}
			if strings.Contains(instructions, "Автоматически передавай уместные эмоции") != tc.enabled {
				t.Fatal("incorrect emotional speech policy")
			}
			var r Resolution
			e.setSpokenAnswer(ctx, &r, "Готов.")
			if r.Answer != "Готов." {
				t.Fatal("markup in displayed answer")
			}
			if strings.Contains(r.speech, `voice="zahar"`) != tc.male {
				t.Fatalf("speech=%q", r.speech)
			}
			const explicit = `<speaker voice="jane">Готова.`
			e.setSpokenAnswer(ctx, &r, explicit)
			if tc.enabled && r.speech != explicit {
				t.Fatal("explicit voice overridden")
			}
			e.setSpokenAnswer(ctx, &r, explicit+"</speaker>")
			if strings.Contains(r.speech, "</speaker>") || r.Answer != "Готова." {
				t.Fatal("XML closer leaked")
			}
		})
	}
}
