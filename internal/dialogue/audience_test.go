package dialogue

import "testing"

func TestAudienceIsolation(t *testing.T) {
	s := New(DefaultConfig())
	base := WithSpeaker(t.Context(), "station")
	base, _, old := s.Start(base)
	s.Commit(base, old, "legacy adult question", "legacy answer", true)
	adult := WithAudience(base, "adult")
	adult, view, ticket := s.Start(adult)
	if len(view.Messages) != 0 {
		t.Fatal("inherited unclassified history")
	}
	if !s.Commit(adult, ticket, "adult question", "adult answer", true) {
		t.Fatal("adult ticket failed")
	}
	for _, mode := range []string{"child", "family"} {
		ctx := WithAudience(base, mode)
		ctx, snapshot, ticket := s.Start(ctx)
		if len(snapshot.Messages) != 0 || snapshot.PrivateHistory {
			t.Fatal("adult history leaked to", mode)
		}
		if !s.Commit(ctx, ticket, mode+" question", mode+" answer", false) {
			t.Fatal("audience ticket failed")
		}
	}
	next := WithAudience(base, "adult")
	_, snapshot, _ := s.Start(next)
	if len(snapshot.Messages) != 2 || snapshot.Messages[0].Content != "adult question" {
		t.Fatal("adult history mixed")
	}
}
