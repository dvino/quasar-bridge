package companion

import (
	"strings"
	"testing"

	"quasarbridge/internal/llm"
)

func TestRelevantFactsUsesTopicAndRecentTurns(t *testing.T) {
	facts := []LearnedFact{{Topic: "велосипед", Key: "походы", Value: "любит лесные прогулки"}, {Topic: "музыка", Key: "джаз", Value: "предпочитает джаз"}}
	got := RelevantFacts(facts, "расскажи о велосипеде", nil)
	if len(got) != 1 || got[0].Key != "походы" {
		t.Fatalf("wrong facts: %v", got)
	}
	got = RelevantFacts(facts, "а как насчёт этого?", []llm.Message{{Role: "user", Content: "джаз"}})
	if len(got) != 1 || !strings.Contains(got[0].Value, "джаз") {
		t.Fatalf("missing recent topic: %v", got)
	}
	if len(RelevantFacts(facts, "я люблю космос", nil)) != 0 {
		t.Fatal("irrelevant memory selected")
	}
}
