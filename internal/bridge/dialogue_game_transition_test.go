package bridge

import (
	"context"
	"strings"
	"testing"

	"quasarbridge/internal/companion"
	"quasarbridge/internal/dialogue"
	"quasarbridge/internal/llm"
)

func TestChatToGameCarriesScopedHistoryAndKeepsAnchor(t *testing.T) {
	base := dialogue.WithSpeaker(context.Background(), "station")
	adult := dialogue.WithAudience(base, "adult_male")
	adult = context.WithValue(adult, voiceAgeKey{}, ageContext{mode: ageAdult, adultMale: true})
	child := dialogue.WithAudience(base, "child")
	child = context.WithValue(child, voiceAgeKey{}, ageContext{mode: ageChild})
	classifier := classifierFunc(func(_ context.Context, m []llm.Message, _ bool) (string, error) {
		input := m[len(m)-2].Content
		if strings.Contains(input, "секретная тема") {
			return `{"mode":"continue","kind":"chat","model":"luna","effort":"none","ack":""}`, nil
		}
		if strings.Contains(input, "загадку") {
			return `{"mode":"continue","kind":"game","model":"luna","effort":"none","ack":""}`, nil
		}
		return `{"mode":"continue","kind":"game","model":"terra","effort":"low","ack":""}`, nil
	})
	calls := 0
	model := classifierFunc(func(_ context.Context, m []llm.Message, structured bool) (string, error) {
		if !structured {
			return "Расскажи, что тебе интересно.", nil
		}
		calls++
		var b strings.Builder
		for _, v := range m {
			b.WriteString(v.Content)
		}
		all := b.String()
		if calls == 1 && !strings.Contains(all, "секретная тема космос") {
			t.Error("prior scoped conversation missing")
		}
		if calls == 2 && !strings.Contains(all, "солнце") {
			t.Error("game anchor missing")
		}
		if calls == 1 {
			return `{"speech":"Загадала, спрашивай.","summary":"игра о космосе","activity":"загадка","anchor":"солнце","new_round":true}`, nil
		}
		return `{"speech":"Да, это звезда.","summary":"дана подсказка","activity":"загадка","anchor":"солнце","new_round":false}`, nil
	})
	e := &Engine{ExplicitConversations: true, DialogueClassifier: classifier, Companion: companion.New(), LLM: model}
	d := e.Decide(adult, "давай поболтаем")
	if d.Source != "dialogue_session" {
		t.Fatal(d.Source)
	}
	if _, err := e.ResolveDecision(adult, "давай поболтаем", d); err != nil {
		t.Fatal(err)
	}
	d = e.Decide(adult, "секретная тема космос")
	if d.Source != "dialogue_session" {
		t.Fatal(d.Source)
	}
	if _, err := e.ResolveDecision(adult, "секретная тема космос", d); err != nil {
		t.Fatal(err)
	}
	d = e.Decide(adult, "сыграем в загадку про космос")
	if d.Source != "companion_session" || !d.privateGame || !d.seedGame {
		t.Fatalf("transition: %+v", d)
	}
	if _, err := e.ResolveDecision(adult, "сыграем в загадку про космос", d); err != nil {
		t.Fatal(err)
	}
	d = e.Decide(adult, "это звезда?")
	if d.Source != "companion_session" || !d.privateGame || d.seedGame {
		t.Fatalf("continued: %+v", d)
	}
	if _, err := e.ResolveDecision(adult, "это звезда?", d); err != nil {
		t.Fatal(err)
	}
	state, history := e.Companion.ClassificationContext(dialogue.SpeakerKey(child))
	if state != "" || len(history) != 0 {
		t.Fatal("adult conversation leaked into child scope")
	}
	state, history = e.Companion.ClassificationContext(dialogue.FamilyKey(adult))
	if state != "" || len(history) != 0 {
		t.Fatal("adult conversation leaked into shared scope")
	}
	d = e.Decide(adult, "закончить игру")
	if d.Source != "companion_session_exit" || !d.privateGame {
		t.Fatalf("exit=%+v", d)
	}
	if _, err := e.ResolveDecision(adult, "закончить игру", d); err != nil {
		t.Fatal(err)
	}
	state, history = e.Companion.ClassificationContext(dialogue.SpeakerKey(adult))
	if state != "" || len(history) != 0 {
		t.Fatal("private game not cleared")
	}
}
