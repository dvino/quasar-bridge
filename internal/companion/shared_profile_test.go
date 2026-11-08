package companion

import (
	"strings"
	"testing"
)

func TestSharedProfileReplacesPetFactsWithoutImportingOtherPersonalContext(t *testing.T) {
	s := New()
	if err := s.SetProfile(Profile{Base: "CHILD_BASE", Modules: []Module{
		{ID: "rats", Scope: "family", Triggers: []string{"крыс"}, Content: "OLD_RATS"},
		{ID: "play", Scope: "child", Triggers: []string{"игра"}, Content: "CHILD_PLAY"},
		{ID: "private", Scope: "adult", Triggers: []string{"крыс"}, Content: "ADULT_SECRET"},
	}}); err != nil {
		t.Fatal(err)
	}
	shared := Profile{Base: "OTHER_PRIVATE_BASE", Modules: []Module{
		{ID: "rats", Scope: "family", Triggers: []string{"крыс"}, Content: "NEW_RATS"},
		{ID: "private", Scope: "adult", Triggers: []string{"крыс"}, Content: "OTHER_ADULT_SECRET"},
	}}
	if err := s.SetSharedProfile(shared); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	got := s.profile.selectContext("крысы играют", nil)
	s.mu.Unlock()
	if !strings.Contains(got, "CHILD_BASE") || !strings.Contains(got, "NEW_RATS") || !strings.Contains(got, "CHILD_PLAY") {
		t.Fatal(got)
	}
	for _, unwanted := range []string{"OLD_RATS", "ADULT_SECRET", "OTHER_PRIVATE_BASE"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("leaked %s", unwanted)
		}
	}
	got = shared.SharedContext("крысы", nil)
	if strings.Contains(got, "PRIVATE") || strings.Contains(got, "SECRET") || !strings.Contains(got, "NEW_RATS") {
		t.Fatal("family context scope failure")
	}
}
