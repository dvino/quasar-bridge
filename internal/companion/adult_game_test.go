package companion

import (
	"context"
	"errors"
	"quasarbridge/internal/llm"
	"strings"
	"testing"
)

func TestTransferredHistorySurvivesFailedFirstGameAnswer(t *testing.T) {
	s := New()
	history := []llm.Message{{Role: "user", Content: "интересуюсь космосом"}, {Role: "assistant", Content: "Поговорим о космосе"}}
	fail := completeFunc(func(context.Context, []llm.Message, bool) (string, error) { return "", errors.New("network") })
	if _, err := s.AnswerWithHistory(context.Background(), fail, "adult", "игра", "adult", nil, history); err == nil {
		t.Fatal("expected failure")
	}
	ok := completeFunc(func(_ context.Context, m []llm.Message, _ bool) (string, error) {
		var b strings.Builder
		for _, v := range m {
			b.WriteString(v.Content)
		}
		if !strings.Contains(b.String(), "интересуюсь космосом") {
			t.Fatal("lost transferred context")
		}
		return encoded("Играем.", "начали", "звезда", true), nil
	})
	if _, err := s.AnswerWithHistory(context.Background(), ok, "adult", "игра", "adult", nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestGameProfileScopes(t *testing.T) {
	for _, tc := range []struct {
		source       string
		adult, child bool
	}{{"adult", true, false}, {"child", false, true}, {"family", false, false}} {
		t.Run(tc.source, func(t *testing.T) {
			s := New()
			if err := s.SetProfile(Profile{Base: "CHILD_PRIVATE", Modules: []Module{{ID: "shared", Scope: "family", Triggers: []string{"крысы"}, Content: "FAMILY_SHARED"}}}); err != nil {
				t.Fatal(err)
			}
			adult := &Profile{Base: "ADULT_PRIVATE"}
			client := completeFunc(func(_ context.Context, msgs []llm.Message, _ bool) (string, error) {
				var b strings.Builder
				for _, m := range msgs {
					b.WriteString(m.Content)
				}
				all := b.String()
				if strings.Contains(all, "ADULT_PRIVATE") != tc.adult || strings.Contains(all, "CHILD_PRIVATE") != tc.child || !strings.Contains(all, "FAMILY_SHARED") {
					t.Fatalf("wrong profile scope: %s", all)
				}
				return encoded("Играем.", "начали", "кот", true), nil
			})
			if _, err := s.AnswerWithAdultProfile(context.Background(), client, "scope", "крысы", tc.source, adult); err != nil {
				t.Fatal(err)
			}
		})
	}
}
