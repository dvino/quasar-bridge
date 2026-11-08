package bridge

import (
	"context"
	"strings"

	"quasarbridge/internal/dialogue"
	"quasarbridge/internal/reply"
)

// Companion routing deliberately bypasses Assistant, classifier and device
// tools. In an active game a fictional instruction must not become an action.
func (e *Engine) companionRequest(ctx context.Context, text string) bool {
	if e.Companion == nil {
		return false
	}
	key := dialogue.FamilyKey(ctx)
	if key == "" {
		return false
	}
	if e.Companion.Active(key) {
		return true
	}
	if age, ok := ctx.Value(voiceAgeKey{}).(ageContext); ok && age.mode == ageChild {
		if nativeUtilityRequest(text) {
			return false
		}
		return true
	}
	if age, ok := ctx.Value(voiceAgeKey{}).(ageContext); ok && age.mode == ageAdult && companionText(text) == "давай поболтаем" {
		return false
	}
	switch companionText(text) {
	case "давай поиграем", "давай поболтаем", "включи детского собеседника", "начнём приключение", "начнем приключение":
		return true
	}
	return false
}

// Outside play, explicit media/time/weather commands retain normal routing.
// Inside play the Active check wins: fictional commands never reach devices.
func nativeUtilityRequest(text string) bool {
	t := companionText(text)
	if t == "который час" || t == "сколько времени" || strings.HasPrefix(t, "какая погода") || strings.HasPrefix(t, "погода ") {
		return true
	}
	for _, verb := range []string{"включи ", "выключи ", "поставь ", "останови ", "убавь ", "прибавь ", "сделай "} {
		if !strings.HasPrefix(t, verb) {
			continue
		}
		for _, noun := range []string{"музык", "песн", "радио", "громк", "таймер", "будильник", "свет", "люстр", "ламп", "бра"} {
			if strings.Contains(t, noun) {
				return true
			}
		}
	}
	return false
}

func companionText(text string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(text)), ".!?, ")
}

func (e *Engine) companionAnswer(ctx context.Context, text string) (Resolution, error) {
	result := Resolution{Route: "llm"}
	key := dialogue.FamilyKey(ctx)
	game, _ := ctx.Value(semanticGameKey{}).(semanticGameContext)
	if game.private {
		key = dialogue.SpeakerKey(ctx)
	}
	if requested, answer, err := e.learningControl(ctx, text); requested {
		result.Answer = answer
		return result, err
	}
	if requested, err := e.forgetLearned(ctx, text); requested {
		if err != nil {
			return result, err
		}
		result.Answer = "Удалены выученные сведения. Начальный профиль остаётся отдельно."
		return result, nil
	}
	if requested, err := e.Companion.ForgetRequested(text); requested {
		if err != nil {
			return result, err
		}
		result.Answer = "Сохранённые предпочтения удалены."
		return result, nil
	}
	// Semantic classification decides whether ambiguous stop-words end a game.
	if ctx.Value(semanticModeKey{}) == nil {
		switch companionText(text) {
		case "хватит", "пока", "закончим", "закончить игру", "выйти из игры", "выключи детского собеседника":
			e.Companion.Reset(key)
			reply.SuppressListening(ctx)
			result.Answer = "Хорошо, до встречи!"
			return result, nil
		}
	}
	if dialogue.IsReset(text) {
		e.Companion.Reset(key)
		result.Answer = "Хорошо, начнём с чистого листа."
		return result, nil
	}
	if isRetryCommand(text) || companionText(text) == "повтори" || companionText(text) == "повтори ответ" {
		if answer, ok := e.Companion.Repeat(key); ok {
			e.setSpokenAnswer(ctx, &result, answer)
		} else {
			result.Answer = "Пока нечего повторить. О чём поговорим?"
		}
		return result, nil
	}
	source := ""
	age, classified := ctx.Value(voiceAgeKey{}).(ageContext)
	if classified && age.mode == ageChild {
		source = "child"
	} else if ctx.Value(semanticModeKey{}) != nil {
		source = "family"
		if classified && age.mode == ageAdult && age.adultMale {
			source = "adult"
		}
	}
	if game.seed {
		e.Companion.Reset(key)
	}
	answer, err := e.Companion.AnswerWithMarkup(ctx, e.LLM, key, e.stripModelPrefix(text), source, e.AdultProfile, game.history.Messages, e.ttsInstructions(ctx))
	e.setSpokenAnswer(ctx, &result, answer)
	return result, err
}
