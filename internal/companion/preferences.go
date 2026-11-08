package companion

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
)

// Learned preferences are deliberately a small closed vocabulary: model output
// cannot persist names, secrets, arbitrary instructions or psychological labels.
type preference struct {
	Category string `json:"category"`
	Value    string `json:"value"`
	Evidence string `json:"evidence"`
}

type preferences struct {
	Values map[string]string `json:"values"`
}

var preferenceValues = map[string][]string{
	"humor":        {"playful", "quiet"},
	"explanations": {"examples", "stories", "brief"},
	"puzzles":      {"more_hints", "harder"},
}

func validPreference(category, value string) bool {
	for _, allowed := range preferenceValues[category] {
		if allowed == value {
			return true
		}
	}
	return false
}

func (s *Store) EnablePreferences(path string) error {
	if path == "" {
		return errors.New("empty companion preferences path")
	}
	p := preferences{Values: map[string]string{}}
	f, err := os.Open(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		defer f.Close()
		raw, err := io.ReadAll(io.LimitReader(f, 4097))
		if err != nil {
			return err
		}
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		if len(raw) > 4096 || decoder.Decode(&p) != nil || p.Values == nil || len(p.Values) > len(preferenceValues) {
			return errors.New("invalid companion preferences")
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			return errors.New("invalid companion preferences suffix")
		}
		for k, v := range p.Values {
			if !validPreference(k, v) {
				return errors.New("invalid companion preference value")
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preferencePath = path
	s.preferences = p
	return nil
}

// preferenceContextLocked is model context, not a change to safety policy.
func (s *Store) preferenceContextLocked() string {
	if len(s.preferences.Values) == 0 {
		return ""
	}
	raw, _ := json.Marshal(struct {
		Preferences preferences `json:"learned_preferences"`
	}{s.preferences})
	return string(raw)
}

func memoryOptOut(input string) bool {
	input = strings.ToLower(input)
	return strings.Contains(input, "не запоминай") || strings.Contains(input, "не сохраняй")
}

func forgetPreferences(input string) bool {
	input = strings.ToLower(input)
	return strings.Contains(input, "забудь мои предпочтения") || strings.Contains(input, "удали мои предпочтения") || strings.Contains(input, "забудь что мне нравится") || strings.Contains(input, "забудь, что мне нравится")
}

// Only explicit requests to remember an on-topic preference are persisted.
// Inferred preferences remain in the session summary, not in permanent memory.
func (s *Store) updatePreferenceLocked(input string, p *preference) error {
	if s.preferencePath == "" || p == nil || memoryOptOut(input) {
		return nil
	}
	lower := strings.ToLower(input)
	if !strings.Contains(lower, "запомни") || !validPreference(p.Category, p.Value) || strings.TrimSpace(p.Evidence) == "" || !strings.Contains(input, p.Evidence) {
		return nil
	}
	// The evidence itself must contain an explicit first-person preference.
	evidence := strings.ToLower(p.Evidence)
	if !strings.Contains(evidence, "мне нравится") && !strings.Contains(evidence, "я люблю") && !strings.Contains(evidence, "я предпочитаю") && !strings.Contains(evidence, "мне больше нравятся") {
		return nil
	}
	next := preferences{Values: map[string]string{}}
	for k, v := range s.preferences.Values {
		next.Values[k] = v
	}
	next.Values[p.Category] = p.Value
	if err := savePreferences(s.preferencePath, next); err != nil {
		return err
	}
	s.preferences = next
	return nil
}

// ForgetRequested recognizes only explicit learned-preference deletion, not
// fictional forgetting inside a game.
func (s *Store) ForgetRequested(input string) (bool, error) {
	if !forgetPreferences(input) {
		return false, nil
	}
	return true, s.ResetPreferences()
}

// ResetPreferences is intentionally separate from resetting a game. It clears
// learned options only; the parent-approved thematic profile is unchanged.
func (s *Store) ResetPreferences() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := preferences{Values: map[string]string{}}
	if s.preferencePath != "" {
		if err := savePreferences(s.preferencePath, next); err != nil {
			return err
		}
	}
	s.preferences = next
	return nil
}

func savePreferences(path string, p preferences) error {
	// Reuse the private atomic writer with a closed schema, not a profile file.
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return atomicWrite(path, raw)
}
