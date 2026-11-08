package companion

import "strings"

// A model cannot grant itself permission to replace an established solution.
// Unrecognized transitions must preserve it or ask for an explicit new round.
func newRoundRequested(input string) bool {
	text := strings.ToLower(strings.TrimSpace(input))
	text = strings.NewReplacer("ё", "е", ",", " ", "!", " ", "?", " ").Replace(text)
	if strings.Contains(text, "не начина") || strings.Contains(text, "не хочу") || strings.Contains(text, "не надо") || strings.Contains(text, "не меняй") {
		return false
	}
	for _, phrase := range []string{"новую загадку", "другую загадку", "еще загадку", "новая загадка", "новый раунд", "новую игру", "другую игру", "начнем заново", "начни заново", "давай заново", "начнем сначала", "давай детектив", "давай расследование", "давай угадайку", "давай приключение", "давай другую", "давай новое"} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
