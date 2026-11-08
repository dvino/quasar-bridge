package bridge

import (
	"context"
	"strings"
	"sync"
	"time"

	"quasarbridge/internal/dialogue"
)

// conversationModes tracks explicit conversational intent per device, not identity.
type conversationModes struct {
	mu      sync.Mutex
	entries map[string]conversationMode
}
type conversationMode struct {
	child       bool
	privateGame bool
	at          time.Time
}

// A game entered from a scoped chat must not expose that chat to other audiences.
func (m *conversationModes) promoteGame(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	key := dialogue.FamilyKey(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.entries[key]
	if !ok || entry.child || ctx.Err() != nil {
		return false
	}
	entry.child = true
	entry.privateGame = true
	m.entries[key] = entry
	return true
}

// delivered starts the logical inactivity window at the forwarded final answer,
// not at the user's question, which may precede a long model response.
func (m *conversationModes) delivered(ctx context.Context) {
	key := dialogue.FamilyKey(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.entries[key]; ok {
		entry.at = time.Now()
		m.entries[key] = entry
	}
}

// current returns a snapshot without refreshing the inactivity deadline.
func (m *conversationModes) current(ctx context.Context) (conversationMode, bool) {
	key := dialogue.FamilyKey(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.entries[key]
	if ok && time.Since(entry.at) >= 2*time.Minute {
		delete(m.entries, key)
		return conversationMode{}, false
	}
	return entry, ok
}

func (m *conversationModes) apply(ctx context.Context, mode string, child bool) (active, isChild, exit bool) {
	key := dialogue.FamilyKey(ctx)
	if key == "" || ctx.Err() != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	if m.entries == nil {
		m.entries = make(map[string]conversationMode)
	}
	now := time.Now()
	for k, v := range m.entries {
		if now.Sub(v.at) >= 2*time.Minute {
			delete(m.entries, k)
		}
	}
	entry, ok := m.entries[key]
	if ok && mode == "end" {
		delete(m.entries, key)
		return true, entry.child, true
	}
	if ok {
		entry.at = now
		m.entries[key] = entry
		return true, entry.child, false
	}
	if mode != "start" || len(m.entries) >= 32 {
		return
	}
	m.entries[key] = conversationMode{child: child, at: now}
	return true, child, false
}

func conversationStart(text string) bool {
	t := companionText(text)
	switch t {
	case "давай поболтаем", "давай поговорим", "давай пообщаемся", "поболтаем", "давай поиграем", "начнём приключение", "начнем приключение", "включи детского собеседника":
		return true
	}
	return strings.HasPrefix(t, "давай поиграем в ") || strings.HasPrefix(t, "давай поговорим о ")
}
func conversationStop(text string) bool {
	switch companionText(text) {
	case "хватит", "пока", "закончим", "закончить разговор", "закончить игру", "выйти из игры", "выключи детского собеседника":
		return true
	}
	return false
}

// route returns whether explicit mode handles this turn and whether it exits.
func (m *conversationModes) route(ctx context.Context, text string, child bool) (active, isChild, exit bool) {
	key := dialogue.FamilyKey(ctx)
	if key == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if m.entries == nil {
		m.entries = make(map[string]conversationMode)
	}
	for k, v := range m.entries {
		if now.Sub(v.at) >= 2*time.Minute {
			delete(m.entries, k)
		}
	}
	entry, ok := m.entries[key]
	if ok && conversationStop(text) {
		delete(m.entries, key)
		return true, entry.child, true
	}
	if ok {
		entry.at = now
		m.entries[key] = entry
		return true, entry.child, false
	}
	if !conversationStart(text) {
		return
	}
	// Bound device state even if a caller supplies arbitrary scopes.
	if len(m.entries) >= 32 {
		return
	}
	m.entries[key] = conversationMode{child: child, at: now}
	return true, child, false
}
