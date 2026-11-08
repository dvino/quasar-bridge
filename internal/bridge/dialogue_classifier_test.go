package bridge

import (
	"context"
	"errors"
	"testing"
	"time"

	"quasarbridge/internal/llm"
)

type classifierFunc func(context.Context, []llm.Message, bool) (string, error)

func (f classifierFunc) Complete(ctx context.Context, m []llm.Message, structured bool) (string, error) {
	return f(ctx, m, structured)
}

func TestDialogueClassificationValidation(t *testing.T) {
	for _, tc := range []struct {
		name, raw, mode, ack string
		wantErr              bool
	}{
		{"valid", `{"mode":"start","kind":"game","model":"luna","effort":"none","ack":"Давай!"}`, "start", "Давай!", false},
		{"question suppressed", `{"mode":"start","kind":"game","model":"luna","effort":"low","ack":"Какой вариант?"}`, "start", "", false},
		{"markup suppressed", `{"mode":"start","kind":"game","model":"terra","effort":"medium","ack":"<think>"}`, "start", "", false},
		{"continuation suppressed", `{"mode":"continue","kind":"game","model":"sol","effort":"low","ack":"Давай!"}`, "continue", "", false},
		{"bad effort", `{"mode":"start","kind":"game","model":"sol","effort":"none","ack":""}`, "", "", true},
		{"bad mode", `{"mode":"start now","kind":"game","model":"luna","effort":"low","ack":""}`, "", "", true},
		{"markdown", "```json\n{}\n```", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := classifierFunc(func(context.Context, []llm.Message, bool) (string, error) { return tc.raw, nil })
			got, err := classifyDialogue(context.Background(), c, false, "", nil, "hello", time.Second)
			if (err != nil) != tc.wantErr || got.Mode != tc.mode || got.Ack != tc.ack {
				t.Fatalf("got %+v err %v", got, err)
			}
		})
	}
}
func TestDialogueClassifierContextAndCancellation(t *testing.T) {
	h := make([]llm.Message, 10)
	for i := range h {
		h[i] = llm.Message{Role: "user", Content: "old"}
	}
	c := classifierFunc(func(ctx context.Context, m []llm.Message, structured bool) (string, error) {
		if !structured || len(m) != 10 {
			t.Errorf("unexpected messages %d structured=%v", len(m), structured)
		}
		<-ctx.Done()
		return "", ctx.Err()
	})
	_, err := classifyDialogue(context.Background(), c, true, "", h, "текст", time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}
