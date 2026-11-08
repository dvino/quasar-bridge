package companion

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"quasarbridge/internal/llm"
)

func TestCancelledExtractionDoesNotPersist(t *testing.T) {
	dir := t.TempDir()
	m, err := NewLearnedMemory(map[string]string{"child": filepath.Join(dir, "child.json"), "family": filepath.Join(dir, "family.json")})
	if err != nil {
		t.Fatal(err)
	}
	s := New()
	s.SetLearnedMemory(m)
	ctx, cancel := context.WithCancel(context.Background())
	client := completeFunc(func(_ context.Context, messages []llm.Message, _ bool) (string, error) {
		if strings.Contains(messages[0].Content, "Извлеки только") {
			cancel()
			return `{"candidates":[{"scope":"child","topic":"игры","key":"загадки","value":"нравятся загадки","kind":"fact","evidence":"мне нравятся загадки"}]}`, nil
		}
		return encoded("Давай сыграем.", "разговор", "", false), nil
	})
	if _, err := s.AnswerScoped(ctx, client, "station", "мне нравятся загадки", "child"); err == nil {
		t.Fatal("cancelled turn accepted")
	}
	facts, err := m.Snapshot("child")
	if err != nil || len(facts) != 0 {
		t.Fatalf("cancelled data persisted: %v %v", facts, err)
	}
}

func TestExtractionRunsAlongsideAnswerAndCommitsOnlyValidTurn(t *testing.T) {
	dir := t.TempDir()
	m, err := NewLearnedMemory(map[string]string{"child": filepath.Join(dir, "child.json"), "family": filepath.Join(dir, "family.json")})
	if err != nil {
		t.Fatal(err)
	}
	s := New()
	s.SetLearnedMemory(m)
	started := make(chan string, 2)
	release := make(chan struct{})
	client := completeFunc(func(ctx context.Context, messages []llm.Message, _ bool) (string, error) {
		extract := strings.Contains(messages[0].Content, "Извлеки только")
		if extract {
			started <- "extract"
		} else {
			started <- "answer"
		}
		select {
		case <-release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		if extract {
			return `{"candidates":[{"scope":"child","topic":"игры","key":"загадки","value":"нравятся загадки","kind":"fact","evidence":"мне нравятся загадки"}]}`, nil
		}
		return encoded("Играем.", "разговор", "", false), nil
	})
	done := make(chan error, 1)
	go func() {
		_, err := s.AnswerScoped(context.Background(), client, "station", "мне нравятся загадки", "child")
		done <- err
	}()
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case name := <-started:
			seen[name] = true
		case <-time.After(time.Second):
			close(release)
			t.Fatal("answer and extraction did not overlap")
		}
	}
	if !seen["answer"] || !seen["extract"] {
		close(release)
		t.Fatal("missing call")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	facts, err := m.Snapshot("child")
	if err != nil || len(facts) != 1 {
		t.Fatalf("facts=%v err=%v", facts, err)
	}
}

func TestScopedChildLearningRequiresChildAndNoRememberCommand(t *testing.T) {
	dir := t.TempDir()
	m, err := NewLearnedMemory(map[string]string{"child": filepath.Join(dir, "child.json"), "family": filepath.Join(dir, "family.json")})
	if err != nil {
		t.Fatal(err)
	}
	s := New()
	s.SetLearnedMemory(m)
	var calls atomic.Int32
	client := completeFunc(func(_ context.Context, messages []llm.Message, _ bool) (string, error) {
		calls.Add(1)
		if strings.Contains(messages[0].Content, "Извлеки только") {
			return `{"candidates":[{"scope":"child","topic":"игры","key":"загадки","value":"нравятся загадки","kind":"fact","evidence":"мне нравятся загадки"}]}`, nil
		}
		return encoded("Давай сыграем.", "разговор", "", false), nil
	})
	if _, err := s.AnswerScoped(context.Background(), client, "station", "мне нравятся загадки", "child"); err != nil {
		t.Fatal(err)
	}
	facts, err := m.Snapshot("child")
	if err != nil || len(facts) != 1 || facts[0].Value != "нравятся загадки" {
		t.Fatalf("facts=%v err=%v", facts, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
	if _, err := s.AnswerScoped(context.Background(), client, "other", "мне нравятся загадки", ""); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("unclassified speaker invoked extraction")
	}
}
