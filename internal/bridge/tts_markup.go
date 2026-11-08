package bridge

import (
	"context"
	"strings"

	"quasarbridge/internal/tts"
)

func (e *Engine) ownerVoice(ctx context.Context) bool {
	age, ok := ctx.Value(voiceAgeKey{}).(ageContext)
	return e.TTSMarkup && ok && age.mode == ageAdult && age.adultMale
}

func (e *Engine) ttsInstructions(ctx context.Context) string {
	if !e.TTSMarkup {
		return " Говоря о себе, используй женский род."
	}
	instructions := tts.Instructions
	if e.ownerVoice(ctx) {
		return instructions + ` Для этого взрослого собеседника твой постоянный голос — kostya, мужской. Начинай speech или текст ответа с <speaker voice="kostya"> и говори о себе в мужском роде («понял», «готов»), кроме явно запрошенного другого голоса или реплики персонажа. После реплик персонажей возвращайся к kostya.`
	}
	return instructions + " По умолчанию говори о себе в женском роде; при смене голоса согласуй род с голосом."
}

func (e *Engine) setSpokenAnswer(ctx context.Context, r *Resolution, text string) {
	if e.ownerVoice(ctx) && text != "" && !strings.Contains(text, "voice=") {
		text = `<speaker voice="kostya">` + text
	}
	r.setModelAnswer(text, e.TTSMarkup)
}

func (r *Resolution) setModelAnswer(text string, useMarkup bool) {
	// Models often emit XML-style closers despite the prompt. Alice uses
	// state-changing speaker tags, not XML scopes.
	text = strings.NewReplacer("</speaker>", "", "<speak>", "", "</speak>", "").Replace(text)
	r.Answer = tts.Plain(text)
	r.speech = ""
	if r.Answer != text && useMarkup {
		r.speech = text
	}
}
