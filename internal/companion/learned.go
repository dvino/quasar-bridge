package companion

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unicode/utf8"
)

// LearnedFact is a compact, editable observation, not a conversation log.
type LearnedFact struct {
	Topic  string `json:"topic"`
	Key    string `json:"key"`
	Value  string `json:"value"`
	Kind   string `json:"kind"`   // fact or hypothesis
	Source string `json:"source"` // adult, child or family
}

type learnedFile struct {
	Facts    []LearnedFact `json:"facts"`
	Disabled bool          `json:"disabled,omitempty"`
}

// LearnedMemory serializes local updates to preserve independent observations.
// Each scope has a separate file: voice age is not proof of speaker identity.
type LearnedMemory struct {
	mu    sync.Mutex
	paths map[string]string
}

func NewLearnedMemory(paths map[string]string) (*LearnedMemory, error) {
	copyPaths := map[string]string{}
	seen := map[string]bool{}
	for scope, path := range paths {
		if (scope != "adult" && scope != "child" && scope != "family") || path == "" || seen[path] {
			return nil, errors.New("invalid memory scope")
		}
		seen[path] = true
		copyPaths[scope] = path
	}
	m := &LearnedMemory{paths: copyPaths}
	for scope := range copyPaths {
		if _, err := m.read(scope); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func validFact(f LearnedFact) bool {
	bounded := func(s string, n int) bool {
		return strings.TrimSpace(s) != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= n
	}
	return bounded(f.Topic, 60) && bounded(f.Key, 80) && bounded(f.Value, 300) && (f.Kind == "fact" || f.Kind == "hypothesis") && (f.Source == "adult" || f.Source == "child" || f.Source == "family")
}

func (m *LearnedMemory) read(scope string) (learnedFile, error) {
	var data learnedFile
	path, ok := m.paths[scope]
	if !ok {
		return data, errors.New("memory scope disabled")
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return learnedFile{Facts: []LearnedFact{}}, nil
	}
	if err != nil {
		return data, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 32769))
	if err != nil {
		return data, err
	}
	if len(raw) > 32768 {
		return data, errors.New("memory file too large")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&data) != nil || len(data.Facts) > 64 {
		return learnedFile{}, errors.New("invalid memory file")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return learnedFile{}, errors.New("invalid memory suffix")
	}
	seen := map[string]bool{}
	for _, fact := range data.Facts {
		if !validFact(fact) || seen[fact.Key] {
			return learnedFile{}, errors.New("invalid memory fact")
		}
		seen[fact.Key] = true
	}
	return data, nil
}

// Snapshot reloads the file on every call, so owner edits take effect without restart.
func (m *LearnedMemory) Snapshot(scope string) ([]LearnedFact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := m.read(scope)
	return data.Facts, err
}

// Clear removes automatically learned observations from one scope. Seed
// profiles are separate and are deliberately not changed by this operation.
func (m *LearnedMemory) Clear(scope string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := m.read(scope)
	if err != nil {
		return err
	}
	data.Facts = []LearnedFact{}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return atomicWrite(m.paths[scope], raw)
}

func (m *LearnedMemory) LearningEnabled(scope string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := m.read(scope)
	return !data.Disabled, err
}

// SetLearning disables or resumes automatic updates without deleting facts.
func (m *LearnedMemory) SetLearning(scope string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := m.read(scope)
	if err != nil {
		return err
	}
	data.Disabled = !enabled
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(m.paths[scope], raw)
}

// Apply replaces observations by stable key. It never writes model evidence,
// raw conversation text or instructions. Invalid candidates fail as a batch.
func (m *LearnedMemory) Apply(scope string, facts []LearnedFact) error {
	if len(facts) > 8 {
		return errors.New("too many memory updates")
	}
	for _, f := range facts {
		if !validFact(f) || (scope != "family" && f.Source != scope) {
			return errors.New("invalid memory update")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := m.read(scope)
	if err != nil {
		return err
	}
	if data.Disabled {
		return nil
	}
	for _, f := range facts {
		found := false
		for i := range data.Facts {
			if data.Facts[i].Key == f.Key {
				data.Facts[i] = f
				found = true
				break
			}
		}
		if !found {
			data.Facts = append(data.Facts, f)
		}
	}
	if len(data.Facts) > 64 {
		return errors.New("memory full")
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if len(raw) > 32768 {
		return errors.New("memory too large")
	}
	if err := atomicWrite(m.paths[scope], raw); err != nil {
		return fmt.Errorf("save memory: %w", err)
	}
	return nil
}
