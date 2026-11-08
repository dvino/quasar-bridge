package companion

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"quasarbridge/internal/llm"
)

// RelevantFacts selects compact topic matches from the current and recent
// turns. It is a heuristic, not a semantic guarantee or access control.
func RelevantFacts(facts []LearnedFact, input string, history []llm.Message) []LearnedFact {
	recent := input
	for _, m := range history[max(0, len(history)-4):] {
		recent += " " + m.Content
	}
	recent = strings.ToLower(recent)
	stop := map[string]bool{"нравится": true, "любит": true, "люблю": true, "предпочитает": true, "предпочитаю": true, "очень": true, "хочет": true, "хочу": true, "иногда": true, "играть": true}
	var out []LearnedFact
	for _, fact := range facts {
		words := strings.FieldsFunc(strings.ToLower(fact.Topic+" "+fact.Key+" "+fact.Value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		for _, word := range words {
			if stop[word] || utf8.RuneCountInString(word) < 4 {
				continue
			}
			chars := []rune(word)
			if len(chars) > 5 {
				word = string(chars[:len(chars)-2])
			}
			if strings.Contains(recent, word) {
				out = append(out, fact)
				break
			}
		}
		if len(out) >= 12 {
			break
		}
	}
	return out
}
