package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type selectionRequest struct {
	Model     string `json:"model"`
	Reasoning struct {
		Effort string `json:"effort"`
	} `json:"reasoning"`
}

func TestModelSelectionIsPerRequest(t *testing.T) {
	seen := make(chan selectionRequest, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v selectionRequest
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			t.Error(err)
		}
		seen <- v
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseAnswer))
	}))
	defer srv.Close()
	c := &Client{API: "responses", BaseURL: srv.URL, Model: "base", ReasoningEffort: "low"}
	for _, ctx := range []context.Context{WithModelSelection(context.Background(), "alternate", "high"), context.Background(), WithDefaultModelSelection(WithModelSelection(context.Background(), "alternate", "high"))} {
		if _, err := c.Complete(ctx, []Message{{Role: "user", Content: "hi"}}, false); err != nil {
			t.Fatal(err)
		}
	}
	a, b, d := <-seen, <-seen, <-seen
	if a.Model != "alternate" || a.Reasoning.Effort != "high" || b.Model != "base" || b.Reasoning.Effort != "low" || d.Model != "base" || d.Reasoning.Effort != "low" || c.Model != "base" {
		t.Fatalf("selection leaked: %+v %+v %+v", a, b, d)
	}
}
