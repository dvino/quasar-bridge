package tts

import "testing"

func TestPlain(t *testing.T) {
	for _, tc := range []struct{ speech, plain string }{
		{`<speaker voice="zahar" emotion="good">Я понял!`, "Я понял!"},
		{`<speaker voice="jane">Я поняла.</speaker><speaker voice="ermil">Готов.`, "Я поняла.Готов."},
		{`<speaker effect="train_announce">Поехали!<speaker effect="-">`, "Поехали!"},
		{`Т+ихо sil <[500]> зам+ок <[z a m o k]>`, "Тихо замок"},
		{"Обычный текст\nбез изменений: 2+2=4, C++.", "Обычный текст\nбез изменений: 2+2=4, C++."},
		{`<speaker is_whisper="true" speed="0.9" lang="ru-RU">Тише.`, "Тише."},
	} {
		if got := Plain(tc.speech); got != tc.plain {
			t.Errorf("Plain(%q)=%q, want %q", tc.speech, got, tc.plain)
		}
	}
}
