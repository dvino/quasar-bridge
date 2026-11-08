package bridge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"quasarbridge/internal/companion"
	"quasarbridge/internal/dialogue"
	"quasarbridge/internal/llm"
)

type testLocalCommand struct{ matched, ran int }

func (c *testLocalCommand) Match(_ context.Context, text string) (string, error) {
	c.matched++
	if text == "команда" {
		return "test-action", nil
	}
	return "", nil
}
func (c *testLocalCommand) Run(_ context.Context, action string) (string, error) {
	c.ran++
	if action != "test-action" {
		return "", errors.New("bad action")
	}
	return "готово", nil
}

func TestLocalCommandMatchesAndRuns(t *testing.T) {
	c := &testLocalCommand{}
	e := &Engine{LocalCommand: c}
	ctx := dialogue.WithSpeaker(t.Context(), "test")
	d := e.Decide(ctx, "команда")
	if d.Source != "local_command" || c.ran != 0 {
		t.Fatal("matching must not execute the action")
	}
	r, err := e.ResolveDecision(ctx, "команда", d)
	if err != nil || r.Answer != "готово" || c.ran != 1 {
		t.Fatalf("result=%+v err=%v runs=%d", r, err, c.ran)
	}
}

func TestLocalCommandDoesNotRunDuringGame(t *testing.T) {
	c := &testLocalCommand{}
	e := &Engine{LocalCommand: c, Companion: companion.New(), ExplicitConversations: true,
		DialogueClassifier: classifierFunc(func(context.Context, []llm.Message, bool) (string, error) {
			return `{"mode":"continue","kind":"game","model":"luna","effort":"none","ack":""}`, nil
		})}
	ctx := dialogue.WithSpeaker(t.Context(), "test")
	start := e.Decide(ctx, "давай поиграем")
	if start.Source != "companion_session" {
		t.Fatalf("game start: %+v", start)
	}
	c.matched = 0
	d := e.Decide(ctx, "команда")
	if d.Source != "companion_session" || c.matched != 0 || c.ran != 0 {
		t.Fatalf("game command escaped: source=%s matches=%d runs=%d", d.Source, c.matched, c.ran)
	}
}

func TestExecutableCommandRejectsOversizedOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "handler")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nhead -c 5000 /dev/zero\n"), 0700); err != nil {
		t.Fatal(err)
	}
	_, err := (ExecutableCommand{Path: path}).Match(t.Context(), "test")
	if err == nil {
		t.Fatal("oversized handler output accepted")
	}
}

func TestExecutableCommandProtocol(t *testing.T) {
	path := filepath.Join(t.TempDir(), "handler")
	data := "#!/bin/sh\ncase \"$1\" in match) read -r line; [ \"$line\" = test ] && echo action;; run) read -r line; [ \"$line\" = action ] && echo done;; esac\nexit 0\n"
	if err := os.WriteFile(path, []byte(data), 0700); err != nil {
		t.Fatal(err)
	}
	c := ExecutableCommand{Path: path}
	match, err := c.Match(t.Context(), "test\n")
	if err != nil || match != "action" {
		t.Fatalf("match=%q err=%v", match, err)
	}
	answer, err := c.Run(t.Context(), match+"\n")
	if err != nil || answer != "done" {
		t.Fatalf("answer=%q err=%v", answer, err)
	}
}
