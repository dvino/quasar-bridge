package companion

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"quasarbridge/internal/llm"
)

// Profile contains only approved model context, not parental notes or raw logs.
// Trigger matching is an optimization, never an authorization decision.
type Profile struct {
	Base    string   `json:"base"`
	Modules []Module `json:"modules"`
}

type Module struct {
	Scope    string   `json:"scope,omitempty"`
	ID       string   `json:"id"`
	Triggers []string `json:"triggers"`
	Content  string   `json:"content"`
}

func (p Profile) Validate() error {
	if !utf8.ValidString(p.Base) || utf8.RuneCountInString(p.Base) > 3000 || len(p.Modules) > 16 {
		return errors.New("invalid companion profile limits")
	}
	seen := map[string]bool{}
	for _, m := range p.Modules {
		if m.Scope != "" && m.Scope != "family" && m.Scope != "child" && m.Scope != "adult" {
			return errors.New("invalid companion module scope")
		}
		if m.ID == "" || len(m.ID) > 64 || seen[m.ID] || len(m.Triggers) == 0 || len(m.Triggers) > 32 || strings.TrimSpace(m.Content) == "" || !utf8.ValidString(m.Content) || utf8.RuneCountInString(m.Content) > 5000 {
			return errors.New("invalid companion module")
		}
		seen[m.ID] = true
		for _, trigger := range m.Triggers {
			if utf8.RuneCountInString(strings.TrimSpace(trigger)) < 3 || len(trigger) > 100 || !utf8.ValidString(trigger) {
				return errors.New("invalid companion trigger")
			}
		}
	}
	return nil
}

// LoadProfile reads a bounded, explicitly configured context file. Unknown
// fields fail closed to avoid accidentally sending extra records to a model.
func LoadProfile(path string) (Profile, error) {
	var p Profile
	f, err := os.Open(path)
	if err != nil {
		return p, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 64001))
	if err != nil {
		return p, err
	}
	if len(raw) > 64000 || !utf8.Valid(raw) {
		return p, errors.New("invalid companion profile size")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return Profile{}, errors.New("invalid companion profile schema")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return Profile{}, errors.New("invalid companion profile suffix")
	}
	return p, p.Validate()
}

// SaveProfile atomically replaces the approved context, never a transcript.
// A failed replacement leaves the previous file intact. No backups are made.
func SaveProfile(path string, p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if len(raw) > 64000 {
		return errors.New("invalid companion profile size")
	}
	return atomicWrite(path, raw)
}

func atomicWrite(path string, raw []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".companion-profile-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}

func (s *Store) SetProfile(p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	// Copy all caller-owned slices so later edits cannot race active calls.
	clone := Profile{Base: p.Base}
	for _, m := range p.Modules {
		if m.Scope == "adult" {
			continue
		}
		m.Triggers = append([]string(nil), m.Triggers...)
		clone.Modules = append(clone.Modules, m)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profile = clone
	return nil
}

// SetSharedProfile replaces family modules while preserving the child's base
// and private modules. Both modes then select the same approved family facts.
func (s *Store) SetSharedProfile(p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := Profile{Base: s.profile.Base}
	for _, m := range s.profile.Modules {
		if m.Scope != "family" {
			next.Modules = append(next.Modules, m)
		}
	}
	for _, m := range p.Modules {
		if m.Scope == "family" {
			m.Triggers = append([]string(nil), m.Triggers...)
			next.Modules = append(next.Modules, m)
		}
	}
	if err := next.Validate(); err != nil {
		return err
	}
	s.profile = next
	return nil
}

// ModelContext returns selected data for this explicitly chosen profile.
func (p Profile) ModelContext(input string, history []llm.Message) string {
	return p.selectContext(input, history)
}

// SharedContext excludes the personal base and all non-family modules. Sharing
// pet facts never shares conversation history or a child's preferences.
func (p Profile) SharedContext(input string, history []llm.Message) string {
	shared := Profile{}
	for _, m := range p.Modules {
		if m.Scope == "family" {
			shared.Modules = append(shared.Modules, m)
		}
	}
	return shared.selectContext(input, history)
}

func (p Profile) selectContext(input string, history []llm.Message) string {
	// Keep immediate pronoun follow-ups in scope, but do not use the cumulative
	// summary: one old rat story must not select the rat module forever.
	recent := input
	for _, m := range history[max(0, len(history)-4):] {
		recent += "\n" + m.Content
	}
	recent = strings.ToLower(recent)
	selected := Profile{Base: p.Base}
	for _, m := range p.Modules {
		for _, trigger := range m.Triggers {
			if strings.Contains(recent, strings.ToLower(trigger)) {
				selected.Modules = append(selected.Modules, Module{ID: m.ID, Content: m.Content})
				break
			}
		}
	}
	if selected.Base == "" && len(selected.Modules) == 0 {
		return ""
	}
	raw, _ := json.Marshal(struct {
		Profile Profile `json:"profile_data"`
	}{selected})
	return string(raw)
}
