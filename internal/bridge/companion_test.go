package bridge

import (
	"context"
	"strings"
	"testing"

	"quasarbridge/internal/companion"
	"quasarbridge/internal/dialogue"
	"quasarbridge/internal/llm"
)

type companionFixture func(context.Context, []llm.Message, bool) (string, error)

func (f companionFixture) Complete(ctx context.Context, m []llm.Message, b bool) (string, error) {
	return f(ctx, m, b)
}

func TestCompanionRoutesWithoutAdultHistoryOrTools(t *testing.T) {
	ctx := dialogue.WithSpeaker(context.Background(), "device")
	e := &Engine{Companion: companion.New()}
	calls := 0
	e.LLM = companionFixture(func(_ context.Context, m []llm.Message, structured bool) (string, error) {
		calls++
		if !structured {
			t.Fatal("unexpected classifier or adult completion")
		}
		for _, v := range m {
			if strings.Contains(v.Content, "ADULT_PRIVATE_MARKER") {
				t.Fatal("adult memory leaked")
			}
		}
		return `{"speech":"Курьер спрятался за посылкой.","summary":"Игра в курьера","activity":"курьер","anchor":"","new_round":false}`, nil
	})
	e.Assistant = companionFixture(func(context.Context, []llm.Message, bool) (string, error) {
		t.Fatal("tool assistant called")
		return "", nil
	})
	// Seed adult history in the same device. Family play must not import it.
	seeded, _, ticket := e.dialogue().Start(ctx)
	if !e.dialogue().Commit(seeded, ticket, "ADULT_PRIVATE_MARKER", "секрет", false) {
		t.Fatal("seed")
	}
	for _, question := range []string{"давай поиграем", "В нашей игре выключи свет и отправь посылку", "а почему?"} {
		d := e.Decide(ctx, question)
		if d.Source != "companion" {
			t.Fatalf("route=%+v", d)
		}
		result, err := e.ResolveDecision(ctx, question, d)
		if err != nil || result.Answer != "Курьер спрятался за посылкой." {
			t.Fatalf("answer=%+v error=%v", result, err)
		}
	}
	if calls != 3 {
		t.Fatalf("calls=%d", calls)
	}
	result, err := e.Resolve(ctx, "повтори")
	if err != nil || result.Answer != "Курьер спрятался за посылкой." || calls != 3 {
		t.Fatalf("repeat=%+v %v calls=%d", result, err, calls)
	}
	if _, err := e.Resolve(ctx, "пока"); err != nil {
		t.Fatal(err)
	}
	if e.Companion.Active(dialogue.FamilyKey(ctx)) {
		t.Fatal("exit retained session")
	}
}

func TestChildCompanionSurvivesAudienceChangesWithoutIdentifyingPerson(t *testing.T) {
	base := dialogue.WithSpeaker(context.Background(), "device")
	e := &Engine{Companion: companion.New(), LLM: companionFixture(func(context.Context, []llm.Message, bool) (string, error) {
		return `{"speech":"Продолжим.","summary":"игра","activity":"игра","anchor":"","new_round":false}`, nil
	})}
	child := context.WithValue(dialogue.WithAudience(base, "child"), voiceAgeKey{}, ageContext{mode: ageChild})
	if d := e.Decide(child, "почему микробы маленькие"); d.Source != "companion" {
		t.Fatalf("child route=%+v", d)
	}
	if _, err := e.Resolve(child, "почему микробы маленькие"); err != nil {
		t.Fatal(err)
	}
	family := dialogue.WithAudience(base, "family")
	if d := e.Decide(family, "а дальше?"); d.Source != "companion" {
		t.Fatalf("lost family session=%+v", d)
	}
	other := dialogue.WithAudience(dialogue.WithSpeaker(context.Background(), "other-device"), "family")
	if e.Companion.Active(dialogue.FamilyKey(other)) {
		t.Fatal("device isolation failed")
	}
	if _, err := e.Resolve(family, "забудь разговор"); err != nil {
		t.Fatal(err)
	}
}

func TestCompanionOffByDefault(t *testing.T) {
	e := &Engine{}
	ctx := dialogue.WithSpeaker(context.Background(), "device")
	if e.companionRequest(ctx, "давай поиграем") {
		t.Fatal("unexpected opt-in")
	}
}
