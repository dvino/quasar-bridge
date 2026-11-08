package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"quasarbridge/internal/dialogue"
	"quasarbridge/internal/llm"
)

func TestDialogueModelRecommendationIsAppliedOnlyWhenEnabled(t *testing.T) {
	type request struct {
		Model     string `json:"model"`
		Reasoning struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
	}
	seen := make(chan request, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v request
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			t.Error(err)
		}
		seen <- v
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Ответ"}]}]}`))
	}))
	defer srv.Close()
	client := &llm.Client{API: "responses", BaseURL: srv.URL, Model: "gpt-6-luna", ReasoningEffort: "none"}
	for _, tc := range []struct {
		enabled bool
		model   string
	}{{true, "test-reasoning-model"}, {false, "test-reasoning-model"}, {true, ""}} {
		e := &Engine{ExplicitConversations: true, SelectDialogueModels: tc.enabled, DialogueModels: map[string]string{"terra": tc.model}, LLM: client, DialogueClassifier: classifierFunc(func(context.Context, []llm.Message, bool) (string, error) {
			return `{"mode":"start","kind":"chat","model":"terra","effort":"medium","ack":""}`, nil
		})}
		ctx := dialogue.WithSpeaker(context.Background(), "test")
		d := e.Decide(ctx, "Обсудим сложную физическую задачу вместе?")
		if _, err := e.ResolveDecision(ctx, "Обсудим сложную физическую задачу вместе?", d); err != nil {
			t.Fatal(err)
		}
		got := <-seen
		if tc.enabled && tc.model != "" && (got.Model != tc.model || got.Reasoning.Effort != "medium") {
			t.Fatalf("selected request: %+v", got)
		}
		if (!tc.enabled || tc.model == "") && (got.Model != "gpt-6-luna" || got.Reasoning.Effort != "none") {
			t.Fatalf("default request: %+v", got)
		}
	}
}
