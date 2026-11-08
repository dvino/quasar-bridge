package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"quasarbridge/internal/companion"
	"quasarbridge/internal/dialogue"
	"quasarbridge/internal/llm"
	"quasarbridge/internal/reply"
)

func TestFastIntentSkipsClassifierButAmbiguousUsesIt(t *testing.T) {
	ctx := dialogue.WithSpeaker(context.Background(), "fast")
	calls := 0
	e := &Engine{ExplicitConversations: true, Companion: companion.New(), DialogueClassifier: classifierFunc(func(context.Context, []llm.Message, bool) (string, error) {
		calls++
		return `{"mode":"continue","kind":"game","model":"luna","effort":"none","ack":""}`, nil
	})}
	if d := e.Decide(ctx, "Позагадывай мне загадки"); d.Source != "companion_session" || calls != 0 {
		t.Fatalf("start=%s calls=%d", d.Source, calls)
	}
	if d := e.Decide(ctx, "стоп, я передумал"); d.Source != "companion_session" || calls != 1 {
		t.Fatalf("ambiguous=%s calls=%d", d.Source, calls)
	}
	if d := e.Decide(ctx, "закончить игру"); d.Source != "companion_session_exit" || calls != 1 {
		t.Fatalf("exit=%s calls=%d", d.Source, calls)
	}
}

func TestFastIntentNoSubstringMatches(t *testing.T) {
	for _, text := range []string{"хватит", "стоп", "давай поговорим о том как закончить игру", "закончи разговор с драконом", "позагадывай мне загадки про стоп"} {
		if _, ok := fastDialogueIntent(text, true); ok {
			t.Fatalf("unexpected active fast path: %q", text)
		}
		if _, ok := fastDialogueIntent(text, false); ok {
			t.Fatalf("unexpected inactive fast path: %q", text)
		}
	}
}

func TestSemanticAdultGameUsesGameStore(t *testing.T) {
	ctx := dialogue.WithSpeaker(context.Background(), "adult-game")
	e := &Engine{ExplicitConversations: true, Companion: companion.New(), DialogueClassifier: classifierFunc(func(context.Context, []llm.Message, bool) (string, error) {
		return `{"mode":"start","kind":"game","model":"luna","effort":"none","ack":"Давай!"}`, nil
	})}
	if d := e.Decide(ctx, "Позагадывай мне загадки"); d.Source != "companion_session" {
		t.Fatalf("source=%s", d.Source)
	}
}

func TestSemanticNewActivityDoesNotGreetDuringActiveSession(t *testing.T) {
	ctx := dialogue.WithSpeaker(context.Background(), "ack-active")
	e := &Engine{ExplicitConversations: true, Companion: companion.New(), DialogueClassifier: classifierFunc(func(context.Context, []llm.Message, bool) (string, error) {
		return `{"mode":"start","kind":"game","model":"luna","effort":"none","ack":"Давай!"}`, nil
	})}
	e.conversationModes.apply(ctx, "start", false)
	d := e.Decide(ctx, "Теперь я хочу игру про космос")
	if d.Source != "companion_session" || d.classification.Ack != "" {
		t.Fatalf("unexpected transition: %+v", d)
	}
}

func TestSemanticClassifierTimeoutKeepsActiveSession(t *testing.T) {
	ctx, cancel := context.WithCancel(dialogue.WithSpeaker(context.Background(), "timeout"))
	defer cancel()
	started := make(chan struct{})
	e := &Engine{ExplicitConversations: true, DialogueClassifier: classifierFunc(func(ctx context.Context, _ []llm.Message, _ bool) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})}
	e.conversationModes.apply(ctx, "start", false)
	done := make(chan RouteDecision, 1)
	go func() { done <- e.Decide(ctx, "подсказка") }()
	<-started
	cancel()
	select {
	case d := <-done:
		if d.Source != "classifier_cancelled" {
			t.Fatalf("source=%s", d.Source)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation ignored")
	}
	if _, ok := e.conversationModes.current(dialogue.WithSpeaker(context.Background(), "timeout")); !ok {
		t.Fatal("cancel removed active game")
	}
}

func TestSemanticClassifierFailureDoesNotEndGame(t *testing.T) {
	ctx := dialogue.WithSpeaker(context.Background(), "failure")
	e := &Engine{ExplicitConversations: true, DialogueClassifier: classifierFunc(func(context.Context, []llm.Message, bool) (string, error) { return "", errors.New("failed") })}
	e.conversationModes.apply(ctx, "start", false)
	if d := e.Decide(ctx, "хватит сахара?"); d.Source != "dialogue_session" {
		t.Fatalf("source=%s", d.Source)
	}
}

func TestSemanticConversationStartContextStopAndDevice(t *testing.T) {
	modes := []string{"start", "continue", "end"}
	i := 0
	classifier := classifierFunc(func(ctx context.Context, m []llm.Message, structured bool) (string, error) {
		if i > 0 && !strings.Contains(m[1].Content, "да") {
			t.Error("active context not provided")
		}
		mode := modes[i]
		i++
		data, _ := json.Marshal(dialogueClassification{Mode: mode, Kind: "chat", Model: "luna", Effort: "none", Ack: "Давай!"})
		return string(data), nil
	})
	e := &Engine{ExplicitConversations: true, DialogueClassifier: classifier, LLM: classifierFunc(func(context.Context, []llm.Message, bool) (string, error) { return "Обычный ответ.", nil })}
	cfg := DefaultRoutingConfig()
	cfg.DefaultRoute = "llm"
	disabled := false
	cfg.Classifier.Enabled = &disabled
	if err := e.SetRouting(cfg); err != nil {
		t.Fatal(err)
	}
	base := dialogue.WithSpeaker(context.Background(), "semantic-test")
	for _, tc := range []struct {
		text, source string
		listen       bool
	}{
		{"Мне бы немного поговорить", "dialogue_session", true},
		{"стоп, я придумал другой ответ", "dialogue_session", true},
		{"останови музыку", "rule", true},
		{"нам пора завершать игру", "dialogue_session_exit", false},
	} {
		ctx := reply.WithPolicy(base)
		d := e.Decide(ctx, tc.text)
		if d.Source != tc.source {
			t.Fatalf("%q source=%q route=%q", tc.text, d.Source, d.Route)
		}
		r, err := e.ResolveDecision(ctx, tc.text, d)
		if err != nil {
			t.Fatalf("%q: %v", tc.text, err)
		}
		if strings.Contains(r.Answer, "mode") || strings.Contains(r.Answer, "luna") {
			t.Fatal("metadata leaked")
		}
		if reply.ListeningAllowed(ctx) != tc.listen {
			t.Fatalf("%q listen=%v", tc.text, reply.ListeningAllowed(ctx))
		}
	}
}
