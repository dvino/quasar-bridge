package companion

import (
	"context"
	"strings"
	"testing"

	"quasarbridge/internal/llm"
	"quasarbridge/internal/tts"
)

func TestSpeechMarkupIsNotStoredInDialogue(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[enabled], func(t *testing.T) {
			s := New()
			const marked = `<speaker is_whisper="true">Прив+ет sil <[500]> друг.`
			const plain = "Привет друг."
			calls := 0
			client := completeFunc(func(_ context.Context, messages []llm.Message, _ bool) (string, error) {
				calls++
				hasMarkup := false
				for _, m := range messages {
					if m.Role == "developer" && strings.Contains(m.Content, `<speaker is_whisper`) {
						hasMarkup = true
					}
				}
				if hasMarkup != enabled {
					t.Fatal("unexpected markup instructions")
				}
				if calls == 2 {
					found := false
					for _, m := range messages {
						if m.Role == "assistant" && m.Content == plain {
							found = true
						}
						if m.Role == "assistant" && strings.Contains(m.Content, "<speaker") {
							t.Fatal("markup leaked to history")
						}
					}
					if !found {
						t.Fatal("plain history missing")
					}
				}
				return encoded(marked, "история", "", false), nil
			})
			instructions := ""
			if enabled {
				instructions = tts.Instructions
			}
			for i := 0; i < 2; i++ {
				answer, err := s.AnswerWithMarkup(context.Background(), client, "family", "продолжай", "family", nil, nil, instructions)
				if err != nil {
					t.Fatal(err)
				}
				want := plain
				if enabled {
					want = marked
				}
				if answer != want {
					t.Fatalf("got %q, want %q", answer, want)
				}
			}
			if got, _ := s.Repeat("family"); got != plain {
				t.Fatalf("repeat = %q", got)
			}
		})
	}
}
