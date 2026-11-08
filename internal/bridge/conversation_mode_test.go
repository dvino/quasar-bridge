package bridge

import (
	"context"
	"testing"
	"time"

	"quasarbridge/internal/dialogue"
	"quasarbridge/internal/llm"
	"quasarbridge/internal/reply"
)

func TestExplicitConversationIntentAndExit(t *testing.T) {
	e := &Engine{ExplicitConversations: true, LLM: companionFixture(func(context.Context, []llm.Message, bool) (string, error) { return "Хорошо.", nil })}
	cfg := DefaultRoutingConfig()
	cfg.DefaultRoute = "llm"
	disabled := false
	cfg.Classifier.Enabled = &disabled
	if err := e.SetRouting(cfg); err != nil {
		t.Fatal(err)
	}
	base := dialogue.WithSpeaker(context.Background(), "test")
	call := func(text string, wantSource string, wantListen bool) {
		ctx := reply.WithPolicy(base)
		decision := e.Decide(ctx, text)
		if decision.Source != wantSource {
			t.Fatalf("%q source=%s", text, decision.Source)
		}
		if _, err := e.ResolveDecision(ctx, text, decision); err != nil {
			t.Fatal(err)
		}
		if reply.ListeningAllowed(ctx) != wantListen {
			t.Fatalf("%q listening=%v", text, reply.ListeningAllowed(ctx))
		}
	}
	call("почему небо голубое", "default", false)
	call("давай поболтаем", "dialogue_session", true)
	call("а ещё?", "dialogue_session", true)
	call("закончим", "dialogue_session_exit", false)
	call("почему небо голубое", "dialogue", false)
}

func TestConversationSessionIsPerSpeakerAndExpires(t *testing.T) {
	m := conversationModes{}
	a := dialogue.WithSpeaker(context.Background(), "a")
	b := dialogue.WithSpeaker(context.Background(), "b")
	if active, _, _ := m.route(a, "давай поговорим", false); !active {
		t.Fatal("not started")
	}
	if active, _, _ := m.route(b, "продолжим", false); active {
		t.Fatal("other speaker shared session")
	}
	m.mu.Lock()
	entry := m.entries[dialogue.FamilyKey(a)]
	entry.at = time.Now().Add(-2 * time.Minute)
	m.entries[dialogue.FamilyKey(a)] = entry
	m.mu.Unlock()
	m.delivered(a)
	if active, _, _ := m.route(a, "продолжим", false); !active {
		t.Fatal("forwarded answer did not refresh inactivity window")
	}
	m.mu.Lock()
	entry = m.entries[dialogue.FamilyKey(a)]
	entry.at = time.Now().Add(-2 * time.Minute)
	m.entries[dialogue.FamilyKey(a)] = entry
	m.mu.Unlock()
	if active, _, _ := m.route(a, "продолжим", false); active {
		t.Fatal("expired session retained")
	}
}
