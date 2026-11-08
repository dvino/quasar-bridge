package bridge

import (
	"context"
	"strings"
)

// Persistent learning controls are intentionally exact and scoped; they do
// not delete previously saved information.
func (e *Engine) learningControl(ctx context.Context, text string) (bool, string, error) {
	if e.LearnedMemory == nil {
		return false, "", nil
	}
	phrase := strings.Trim(strings.ToLower(strings.TrimSpace(text)), ".!? ")
	enabled := false
	switch phrase {
	case "не запоминай обо мне", "ничего обо мне не запоминай", "перестань запоминать обо мне":
	case "можешь запоминать обо мне", "снова запоминай обо мне":
		enabled = true
	default:
		return false, "", nil
	}
	age, ok := ctx.Value(voiceAgeKey{}).(ageContext)
	if !ok {
		return false, "", nil
	}
	scope := ""
	if age.mode == ageChild {
		scope = "child"
	} else if age.mode == ageAdult && age.adultMale {
		scope = "adult"
	} else {
		return false, "", nil
	}
	if err := e.LearnedMemory.SetLearning(scope, enabled); err != nil {
		return true, "", err
	}
	if enabled {
		return true, "Хорошо, снова буду запоминать полезные предпочтения.", nil
	}
	return true, "Хорошо, больше не буду автоматически запоминать о тебе. Уже сохранённое осталось, его можно удалить.", nil
}

// Exact destructive commands only; an ordinary story about forgetting must
// not erase memory. This clears learned records, not the parent's seed profile.
func (e *Engine) forgetLearned(ctx context.Context, text string) (bool, error) {
	if e.LearnedMemory == nil {
		return false, nil
	}
	phrase := strings.Trim(strings.ToLower(strings.TrimSpace(text)), ".!? ")
	scope := ""
	switch phrase {
	case "забудь всё обо мне", "забудь все обо мне", "удали мою выученную память":
		age, ok := ctx.Value(voiceAgeKey{}).(ageContext)
		if !ok {
			return false, nil
		}
		if age.mode == ageChild {
			scope = "child"
		} else if age.mode == ageAdult && age.adultMale {
			scope = "adult"
		} else {
			return false, nil
		}
	case "удали общую выученную память":
		age, ok := ctx.Value(voiceAgeKey{}).(ageContext)
		if !ok || age.mode != ageAdult || !age.adultMale {
			return false, nil
		}
		scope = "family"
	default:
		return false, nil
	}
	return true, e.LearnedMemory.Clear(scope)
}
