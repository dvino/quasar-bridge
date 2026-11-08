package companion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"quasarbridge/internal/llm"
)

type Completer interface {
	Complete(context.Context, []llm.Message, bool) (string, error)
}

type memory struct {
	Summary  string `json:"summary"`
	Activity string `json:"activity"`
	Anchor   string `json:"anchor"`
}

type response struct {
	Speech     string      `json:"speech"`
	Summary    string      `json:"summary"`
	Activity   string      `json:"activity"`
	Anchor     string      `json:"anchor"`
	NewRound   bool        `json:"new_round"`
	Preference *preference `json:"preference,omitempty"`
}

type session struct {
	memory     memory
	messages   []llm.Message
	generation uint64
	at         time.Time
	last       string
}

// Store retains no audio or transcript files. Only explicitly enabled, closed-
// vocabulary preferences may persist. Keys must scope a safe family
// session, never an inferred personal identity or a shared adult history.
type Store struct {
	mu             sync.Mutex
	profile        Profile
	preferences    preferences
	preferencePath string
	learned        *LearnedMemory
	sessions       map[string]*session
	generation     uint64
	now            func() time.Time
}

// SetLearnedMemory enables optional on-disk observations.
func (s *Store) SetLearnedMemory(m *LearnedMemory) { s.mu.Lock(); defer s.mu.Unlock(); s.learned = m }

func New() *Store {
	return &Store{sessions: make(map[string]*session), now: time.Now}
}

const lifetime = 30 * time.Minute

func (s *Store) expireLocked() {
	for key, entry := range s.sessions {
		if s.now().Sub(entry.at) >= lifetime {
			delete(s.sessions, key)
		}
	}
}

func (s *Store) Active(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked()
	return s.sessions[key] != nil
}

// ClassificationContext returns only the active family session's compact state
// and recent turns. It does not include personal profiles or learned memory.
func (s *Store) ClassificationContext(key string) (string, []llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked()
	entry := s.sessions[key]
	if entry == nil {
		return "", nil
	}
	state, _ := json.Marshal(entry.memory)
	return string(state), append([]llm.Message(nil), entry.messages...)
}

// Interrupt invalidates pending model work without discarding confirmed play.
func (s *Store) Interrupt(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry := s.sessions[key]; entry != nil {
		s.generation++
		entry.generation = s.generation
	}
}

func (s *Store) Reset(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, key)
}

// Repeat returns the last generated line without advancing the round. It does
// not assert that the device played that line or that a child heard it.
func (s *Store) Repeat(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked()
	entry := s.sessions[key]
	if entry == nil || entry.last == "" {
		return "", false
	}
	return entry.last, true
}

// Answer snapshots RAM under the lock, calls the model without it, and commits
// only the newest successful request. A failed response cannot advance play.
func (s *Store) Answer(ctx context.Context, client Completer, key, input string) (string, error) {
	return s.AnswerScoped(ctx, client, key, input, "")
}

// AnswerScoped enables personal learning only for a classified, opted-in source.
func (s *Store) AnswerScoped(ctx context.Context, client Completer, key, input, source string) (string, error) {
	return s.AnswerWithAdultProfile(ctx, client, key, input, source, nil)
}

// AnswerWithAdultProfile keeps adult and child context separate for shared games.
func (s *Store) AnswerWithAdultProfile(ctx context.Context, client Completer, key, input, source string, adult *Profile) (string, error) {
	return s.AnswerWithHistory(ctx, client, key, input, source, adult, nil)
}

// AnswerWithHistory imports scoped recent turns only when starting a new session.
func (s *Store) AnswerWithHistory(ctx context.Context, client Completer, key, input, source string, adult *Profile, initial []llm.Message) (string, error) {
	if client == nil || key == "" || strings.TrimSpace(input) == "" || !utf8.ValidString(input) || utf8.RuneCountInString(input) > 8000 {
		return "", errors.New("invalid companion request")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.expireLocked()
	entry := s.sessions[key]
	if entry == nil {
		if len(s.sessions) >= 32 {
			oldest := ""
			var at time.Time
			for k, v := range s.sessions {
				if at.IsZero() || v.at.Before(at) {
					oldest, at = k, v.at
				}
			}
			delete(s.sessions, oldest)
		}
		entry = &session{}
		if len(initial) > 16 {
			initial = initial[len(initial)-16:]
		}
		for _, m := range initial {
			if m.Role == "user" || m.Role == "assistant" {
				entry.messages = append(entry.messages, m)
			}
		}
		s.sessions[key] = entry
	}
	s.generation++
	generation := s.generation
	entry.generation, entry.at = generation, s.now()
	previous := entry.memory
	history := append([]llm.Message(nil), entry.messages...)
	profileContext := s.profile.selectContext(input, history)
	preferenceContext := s.preferenceContextLocked()
	if source == "adult" || source == "family" {
		profileContext = s.profile.SharedContext(input, history)
		preferenceContext = ""
		if source == "adult" && adult != nil {
			profileContext += "\n" + adult.ModelContext(input, history)
		}
	}
	learned := s.learned
	s.mu.Unlock()
	var facts []LearnedFact
	memoryReadable := true
	if learned != nil {
		scopes := []string{"family"}
		if source == "child" || source == "adult" {
			scopes = append(scopes, source)
		}
		for _, scope := range scopes {
			items, err := learned.Snapshot(scope)
			if err != nil {
				memoryReadable = false
				slog.Warn("companion memory read failed", "scope", scope)
				continue
			}
			facts = append(facts, items...)
		}
	}

	data, err := json.Marshal(struct {
		Memory memory `json:"memory"`
	}{previous})
	if err != nil {
		return "", err
	}
	messages := []llm.Message{{Role: "system", Content: instructions}, {Role: "user", Content: string(data)}}
	if source == "adult" {
		messages = append(messages, llm.Message{Role: "developer", Content: "Собеседник — взрослый. Говори естественно и без снисходительности, сохраняя правила игры."})
	}
	if profileContext != "" {
		messages = append(messages, llm.Message{Role: "user", Content: profileContext})
	}
	relevant := RelevantFacts(facts, input, history)
	if len(relevant) > 0 {
		data, _ := json.Marshal(struct {
			Facts []LearnedFact `json:"learned_context"`
		}{relevant})
		messages = append(messages, llm.Message{Role: "user", Content: string(data)})
	}
	if preferenceContext != "" {
		messages = append(messages, llm.Message{Role: "user", Content: preferenceContext})
	}
	messages = append(messages, history...)
	messages = append(messages, llm.Message{Role: "user", Content: input})
	// Subscription backends do not enforce JSON mode. Repeat the wire contract
	// at the end without mixing the user input into privileged instructions.
	messages = append(messages, llm.Message{Role: "developer", Content: "Для этого запроса верни только один объект JSON без Markdown: все обязательные поля speech, summary, activity, anchor (строки), new_round (boolean). Поле preference необязательно. Не пиши обычный текст вне JSON. Соблюдай остальные инструкции и неизменность anchor."})
	var extraction <-chan ExtractionResult
	if learned != nil && (source == "child" || source == "adult") && memoryReadable {
		if enabled, _ := learned.LearningEnabled(source); enabled {
			var cancel context.CancelFunc
			extraction, cancel = ExtractAsync(ctx, client, source, input, facts, history)
			defer cancel()
		}
	}
	raw, err := client.Complete(llm.WithoutHostedTools(ctx), messages, true)
	if err != nil {
		return "", err
	}
	result, err := decodeTurn(raw, previous.Anchor, input)
	if err != nil && (errors.Is(err, errResponseSchema) || errors.Is(err, errResponseIncomplete) || errors.Is(err, errAnchorChanged) || errors.Is(err, errUnexpectedRound)) && ctx.Err() == nil {
		// A subscription backend may ignore JSON mode or rewrite an anchor.
		// Retry once, without exposing the rejected output or weakening checks.
		retryCtx, cancel := context.WithTimeout(llm.WithoutHostedTools(ctx), 8*time.Second)
		retryMessages := append(append([]llm.Message(nil), messages...), llm.Message{Role: "developer", Content: "Исправь формат: только допустимые обязательные поля JSON, без дополнительных полей. Если продолжается прежний раунд или собеседник отгадал ответ, скопируй предыдущий anchor дословно, даже если он пустой, и установи new_round=false; новую загадку или раунд начинай только по отдельной просьбе собеседника."})
		retryRaw, retryErr := client.Complete(retryCtx, retryMessages, true)
		retryValid := retryErr == nil && retryCtx.Err() == nil
		cancel()
		if retryValid {
			result, err = decodeTurn(retryRaw, previous.Anchor, input)
		}
	}
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var updates map[string][]LearnedFact
	if extraction != nil {
		select {
		case result := <-extraction:
			updates = result.Updates
			if result.Err != nil && ctx.Err() == nil {
				slog.Warn("companion memory extraction failed")
			}
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry = s.sessions[key]
	if ctx.Err() != nil || entry == nil || entry.generation != generation {
		return "", context.Canceled
	}
	if learned != nil && ctx.Err() == nil {
		for _, scope := range []string{"child", "adult", "family"} {
			if len(updates[scope]) > 0 {
				if err := learned.Apply(scope, updates[scope]); err != nil {
					slog.Warn("companion memory update failed", "scope", scope)
				}
			}
		}
	}
	if source != "adult" && source != "family" {
		if err := s.updatePreferenceLocked(input, result.Preference); err != nil {
			return "", errors.New("could not save companion preference")
		}
	}
	entry.memory = memory{result.Summary, result.Activity, result.Anchor}
	entry.messages = append(history, llm.Message{Role: "user", Content: input}, llm.Message{Role: "assistant", Content: result.Speech})
	// Summary is refreshed every successful turn, before old verbatim turns leave.
	for len(entry.messages) > 16 || messageChars(entry.messages) > 18000 {
		entry.messages = entry.messages[2:]
	}
	entry.messages = append([]llm.Message(nil), entry.messages...)
	entry.last, entry.at = result.Speech, s.now()
	return result.Speech, nil
}

func messageChars(messages []llm.Message) int {
	n := 0
	for _, message := range messages {
		n += utf8.RuneCountInString(message.Content)
	}
	return n
}

var (
	errResponseSchema     = errors.New("invalid companion response schema")
	errResponseIncomplete = errors.New("incomplete companion response")
	errAnchorChanged      = errors.New("companion changed round solution")
	errUnexpectedRound    = errors.New("companion restarted round without request")
)

func decodeTurn(raw, anchor, input string) (response, error) {
	result, err := decode(raw, anchor)
	if err == nil && anchor != "" && result.NewRound && !newRoundRequested(input) {
		return response{}, errUnexpectedRound
	}
	return result, err
}

func decode(raw, anchor string) (response, error) {
	var result response
	if len(raw) > 64000 || !utf8.ValidString(raw) {
		return result, errors.New("invalid companion response size")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	// Decode pointers first so omitted/null fields are not confused with empty values.
	var wire struct {
		Speech     *string     `json:"speech"`
		Summary    *string     `json:"summary"`
		Activity   *string     `json:"activity"`
		Anchor     *string     `json:"anchor"`
		NewRound   *bool       `json:"new_round"`
		Preference *preference `json:"preference,omitempty"`
	}
	if err := decoder.Decode(&wire); err != nil {
		return result, errResponseSchema
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || wire.Speech == nil || wire.Summary == nil || wire.Activity == nil || wire.Anchor == nil || wire.NewRound == nil {
		return result, errResponseIncomplete
	}
	result = response{Speech: strings.TrimSpace(*wire.Speech), Summary: *wire.Summary, Activity: *wire.Activity, Anchor: *wire.Anchor, NewRound: *wire.NewRound, Preference: wire.Preference}
	if result.Speech == "" || utf8.RuneCountInString(result.Speech) > 5000 || utf8.RuneCountInString(result.Summary) > 6000 || utf8.RuneCountInString(result.Activity) > 3000 || utf8.RuneCountInString(result.Anchor) > 2000 {
		return response{}, errors.New("invalid companion response limits")
	}
	if !result.NewRound && result.Anchor != anchor {
		return response{}, errAnchorChanged
	}
	if err := llm.ValidateAnswer(result.Speech); err != nil {
		return response{}, err
	}
	if strings.HasPrefix(result.Speech, "{") || strings.Contains(result.Speech, "```") {
		return response{}, errors.New("companion returned protocol as speech")
	}
	return result, nil
}
