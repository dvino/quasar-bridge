package companion

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quasarbridge/internal/llm"
)

func TestPreferencesPersistOnlyExplicitClosedVocabularyAndCanBeForgotten(t *testing.T) {
	s := New()
	path := filepath.Join(t.TempDir(), "preferences.json")
	if err := s.EnablePreferences(path); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		input string
		p     preference
		want  string
	}{
		{"я люблю рассказы", preference{"explanations", "stories", "я люблю рассказы"}, ""},
		{"запомни: я люблю рассказы", preference{"explanations", "stories", "я люблю рассказы"}, "stories"},
		{"не запоминай: я люблю примеры", preference{"explanations", "examples", "я люблю примеры"}, "stories"},
		{"запомни: я люблю примеры", preference{"explanations", "examples", "поддельная цитата"}, "stories"},
		{"запомни: я люблю примеры", preference{"explanations", "адрес школы", "я люблю примеры"}, "stories"},
		{"запомни: я люблю примеры", preference{"explanations", "examples", "я люблю примеры"}, "examples"},
	}
	for _, tc := range cases {
		s.mu.Lock()
		err := s.updatePreferenceLocked(tc.input, &tc.p)
		got := s.preferences.Values["explanations"]
		s.mu.Unlock()
		if err != nil || got != tc.want {
			t.Fatalf("%q got %q want %q err=%v", tc.input, got, tc.want, err)
		}
	}
	reloaded := New()
	if err := reloaded.EnablePreferences(path); err != nil {
		t.Fatal(err)
	}
	if reloaded.preferences.Values["explanations"] != "examples" {
		t.Fatal("preference lost")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "я люблю") || strings.Contains(string(raw), "школ") {
		t.Fatal("raw evidence persisted")
	}
	if ok, err := reloaded.ForgetRequested("забудь мои предпочтения"); !ok || err != nil {
		t.Fatalf("forget: %v %v", ok, err)
	}
	again := New()
	if err := again.EnablePreferences(path); err != nil {
		t.Fatal(err)
	}
	if len(again.preferences.Values) != 0 {
		t.Fatal("deleted preferences restored")
	}
}

func TestPreferenceDoesNotCommitFromCancelledOrMalformedAnswer(t *testing.T) {
	s := New()
	path := filepath.Join(t.TempDir(), "preferences.json")
	if err := s.EnablePreferences(path); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	client := completeFunc(func(context.Context, []llm.Message, bool) (string, error) {
		cancel()
		raw, _ := json.Marshal(response{Speech: "Учту.", Summary: "", Preference: &preference{"explanations", "stories", "я люблю рассказы"}})
		return string(raw), nil
	})
	if _, err := s.Answer(ctx, client, "a", "запомни: я люблю рассказы"); err == nil {
		t.Fatal("cancelled turn succeeded")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("cancelled preference persisted")
	}
}
