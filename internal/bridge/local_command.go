package bridge

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"quasarbridge/internal/reply"
)

// LocalCommand is an explicitly configured, trusted executable. Match must
// never perform actions; Run must only accept action tokens produced by Match.
// It is not a security sandbox and must not be installed from untrusted sources.
type LocalCommand interface {
	Match(context.Context, string) (string, error)
	Run(context.Context, string) (string, error)
}

type ExecutableCommand struct{ Path string }

type cappedOutput struct {
	buffer   bytes.Buffer
	overflow bool
}

func (w *cappedOutput) Write(p []byte) (int, error) {
	if len(p) > 4096-w.buffer.Len() {
		w.overflow = true
		return 0, errors.New("local command output too large")
	}
	return w.buffer.Write(p)
}

func (c ExecutableCommand) call(ctx context.Context, mode, input string) (string, error) {
	if c.Path == "" || len(input) > 4096 || !utf8.ValidString(input) {
		return "", errors.New("invalid local command input")
	}
	timeout := 2 * time.Second
	if mode == "run" {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Path, mode)
	cmd.Stdin = strings.NewReader(input)
	var out cappedOutput
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil || out.overflow || !utf8.Valid(out.buffer.Bytes()) {
		return "", errors.New("local command failed")
	}
	return strings.TrimSpace(out.buffer.String()), nil
}

func (c ExecutableCommand) Match(ctx context.Context, text string) (string, error) {
	return c.call(ctx, "match", text)
}

func (c ExecutableCommand) Run(ctx context.Context, action string) (string, error) {
	return c.call(ctx, "run", action)
}

func (e *Engine) matchLocalCommand(ctx context.Context, text string) (RouteDecision, bool) {
	if e.LocalCommand == nil {
		return RouteDecision{}, false
	}
	action, err := e.LocalCommand.Match(ctx, text)
	if err != nil || action == "" || len(action) > 256 {
		return RouteDecision{}, false
	}
	return RouteDecision{Route: "llm", Source: "local_command", localAction: action}, true
}

func (e *Engine) runLocalCommand(ctx context.Context, action string) (Resolution, error) {
	answer, err := e.LocalCommand.Run(ctx, action)
	if ctx.Err() != nil {
		return Resolution{Route: "llm"}, ctx.Err()
	}
	if err != nil || answer == "" {
		answer = "Не удалось выполнить команду."
	}
	reply.SuppressListening(ctx)
	return Resolution{Route: "llm", Answer: answer}, nil
}
