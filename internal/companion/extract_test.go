package companion

import (
	"context"
	"testing"

	"quasarbridge/internal/llm"
)

func TestExtractRequiresEvidenceAndScopedSourceWithoutCommand(t *testing.T) {
	client := completeFunc(func(_ context.Context, _ []llm.Message, structured bool) (string, error) {
		if !structured {
			t.Fatal("unstructured extraction")
		}
		return `{"candidates":[{"scope":"adult","topic":"походы","key":"интерес","value":"нравятся велопоходы","kind":"fact","evidence":"люблю велопоходы"},{"scope":"child","topic":"игры","key":"тема","value":"кот","kind":"fact","evidence":"люблю велопоходы"},{"scope":"adult","topic":"локация","key":"адрес","value":"выдумка","kind":"fact","evidence":"текст отсутствует"}]}`, nil
	})
	updates, err := Extract(context.Background(), client, "adult", "Я люблю велопоходы", nil)
	if err != nil || len(updates["adult"]) != 1 || len(updates["child"]) != 0 {
		t.Fatalf("updates=%v error=%v", updates, err)
	}
	if updates["adult"][0].Source != "adult" {
		t.Fatal("wrong source")
	}
	updates, err = Extract(context.Background(), client, "adult", "Не запоминай: я люблю велопоходы", nil)
	if err != nil || len(updates) != 0 {
		t.Fatal("opt-out ignored")
	}
	if _, err := Extract(context.Background(), client, "family", "текст", nil); err == nil {
		t.Fatal("unknown source accepted")
	}
}
