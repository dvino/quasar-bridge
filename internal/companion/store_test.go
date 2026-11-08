package companion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"quasarbridge/internal/llm"
)

type completeFunc func(context.Context, []llm.Message, bool) (string, error)

func (f completeFunc) Complete(ctx context.Context, m []llm.Message, structured bool) (string, error) {
	return f(ctx, m, structured)
}
func encoded(speech, summary, anchor string, newRound bool) string {
	b, _ := json.Marshal(response{Speech: speech, Summary: summary, Activity: "детектив: найдена улика", Anchor: anchor, NewRound: newRound})
	return string(b)
}

func TestLongConversationRetainsAnchorAndSummaryAcrossClients(t *testing.T) {
	s := New()
	for turn := 0; turn < 20; turn++ {
		client := completeFunc(func(ctx context.Context, messages []llm.Message, structured bool) (string, error) {
			if !structured {
				t.Fatal("not structured")
			}
			if len(messages) > 20 {
				t.Fatalf("unbounded history: %d", len(messages))
			}
			if turn > 0 && (!strings.Contains(messages[1].Content, "кот спрятал ключ") || !strings.Contains(messages[1].Content, fmt.Sprintf("важная идея %d", turn-1))) {
				t.Fatalf("lost memory at %d: %s", turn, messages[1].Content)
			}
			return encoded("Давай проверим улику.", fmt.Sprintf("важная идея %d", turn), "кот спрятал ключ", turn == 0), nil
		})
		if _, err := s.Answer(context.Background(), client, "family", "ещё улика"); err != nil {
			t.Fatal(err)
		}
	}
	if got, ok := s.Repeat("family"); !ok || got != "Давай проверим улику." {
		t.Fatalf("repeat=%q %v", got, ok)
	}
	if _, ok := s.Repeat("adult"); ok {
		t.Fatal("cross-session leak")
	}
}

func TestInvalidResponseDoesNotAdvanceMemory(t *testing.T) {
	s := New()
	good := completeFunc(func(context.Context, []llm.Message, bool) (string, error) {
		return encoded("Начнём.", "первая идея", "луна", true), nil
	})
	if _, err := s.Answer(context.Background(), good, "a", "игра"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		encoded("Новая улика.", "подмена", "солнце", false),
		`{"speech":"привет"}`,
		encoded("<analysis>секрет</analysis>", "", "луна", false),
		encoded("```json", "", "луна", false),
		encoded("Ответ.", "", "луна", false) + ` {}`,
		strings.Repeat("x", 64001),
	} {
		bad := completeFunc(func(context.Context, []llm.Message, bool) (string, error) { return raw, nil })
		if _, err := s.Answer(context.Background(), bad, "a", "продолжай"); err == nil {
			t.Fatalf("accepted %s", raw[:min(80, len(raw))])
		}
		if got, _ := s.Repeat("a"); got != "Начнём." {
			t.Fatal("failed reply committed")
		}
		if s.sessions["a"].memory.Summary != "первая идея" {
			t.Fatal("failed memory committed")
		}
	}
}

func TestInvalidStructuredReplyRetriesOnceWithoutWeakeningAnchor(t *testing.T) {
	for _, first := range []string{`not json`, `{"speech":"missing fields"}`, encoded("bad", "bad", "солнце", false), encoded("new riddle", "bad", "солнце", true)} {
		t.Run(first[:min(12, len(first))], func(t *testing.T) {
			s := New()
			start := completeFunc(func(context.Context, []llm.Message, bool) (string, error) {
				return encoded("Начинаем", "исходное", "луна", true), nil
			})
			if _, err := s.Answer(context.Background(), start, "a", "игра"); err != nil {
				t.Fatal(err)
			}
			calls := 0
			retry := completeFunc(func(_ context.Context, m []llm.Message, _ bool) (string, error) {
				calls++
				if calls == 1 {
					return first, nil
				}
				if calls > 2 {
					t.Fatal("extra retry")
				}
				if !strings.Contains(m[len(m)-1].Content, "скопируй предыдущий anchor") {
					t.Fatal("missing repair instruction")
				}
				return encoded("Подсказка", "продолжение", "луна", false), nil
			})
			got, err := s.Answer(context.Background(), retry, "a", "подсказка")
			if err != nil || got != "Подсказка" || calls != 2 {
				t.Fatalf("got %q calls %d err %v", got, calls, err)
			}
			if s.sessions["a"].memory.Anchor != "луна" {
				t.Fatal("anchor changed")
			}
		})
	}
}

func TestCancelledAndSupersededCallsCannotCommit(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(fmt.Sprint(reset), func(t *testing.T) {
			s := New()
			started, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			slow := completeFunc(func(context.Context, []llm.Message, bool) (string, error) {
				close(started)
				<-release
				return encoded("Старый ответ.", "старое", "", false), nil
			})
			go func() { _, err := s.Answer(context.Background(), slow, "a", "старый вопрос"); done <- err }()
			<-started
			if reset {
				s.Reset("a")
			}
			fast := completeFunc(func(context.Context, []llm.Message, bool) (string, error) {
				return encoded("Новый ответ.", "новое", "", false), nil
			})
			if _, err := s.Answer(context.Background(), fast, "a", "новый вопрос"); err != nil {
				t.Fatal(err)
			}
			close(release)
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("old call: %v", err)
			}
			if got, _ := s.Repeat("a"); got != "Новый ответ." {
				t.Fatalf("old result won: %s", got)
			}
		})
	}
	s := New()
	ctx, cancel := context.WithCancel(context.Background())
	client := completeFunc(func(context.Context, []llm.Message, bool) (string, error) {
		cancel()
		return encoded("Отменено.", "", "", false), nil
	})
	if _, err := s.Answer(ctx, client, "a", "вопрос"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, ok := s.Repeat("a"); ok {
		t.Fatal("cancelled answer committed")
	}
}

func TestMemoryExpiresAndSessionCountIsBounded(t *testing.T) {
	s := New()
	now := time.Now()
	s.now = func() time.Time { return now }
	client := completeFunc(func(context.Context, []llm.Message, bool) (string, error) {
		return encoded("Привет.", "", "", false), nil
	})
	for i := 0; i < 40; i++ {
		if _, err := s.Answer(context.Background(), client, fmt.Sprint(i), "привет"); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Second)
	}
	if len(s.sessions) != 32 {
		t.Fatalf("sessions=%d", len(s.sessions))
	}
	now = now.Add(lifetime)
	if s.Active("39") {
		t.Fatal("expired session active")
	}
	if len(s.sessions) != 0 {
		t.Fatal("expired memory retained")
	}
}

func TestBackendFailurePreservesLastAnswer(t *testing.T) {
	s := New()
	client := completeFunc(func(context.Context, []llm.Message, bool) (string, error) {
		return encoded("Сохраним ход.", "память", "", false), nil
	})
	if _, err := s.Answer(context.Background(), client, "a", "начать"); err != nil {
		t.Fatal(err)
	}
	failed := completeFunc(func(context.Context, []llm.Message, bool) (string, error) { return "", context.DeadlineExceeded })
	if _, err := s.Answer(context.Background(), failed, "a", "дальше"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if got, _ := s.Repeat("a"); got != "Сохраним ход." {
		t.Fatal("failure changed state")
	}
}
