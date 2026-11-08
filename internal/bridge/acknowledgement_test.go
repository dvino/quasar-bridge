package bridge

import "testing"

func TestRemoveRepeatedAcknowledgement(t *testing.T) {
	for _, tc := range []struct{ answer, ack, want string }{
		{"Давай! Поговорим о крысах.", "Давай!", "Поговорим о крысах."},
		{"давай! Поговорим.", "Давай!", "Поговорим."},
		{"Давай!", "Давай!", "Давай!"},
		{"Давай, поговорим о крысах.", "Давай!", "Давай, поговорим о крысах."},
		{"Хорошо! Что тебя интересует?", "Давай!", "Что тебя интересует?"},
		{"С удовольствием! О крысах.", "Давай!", "О крысах."},
		{"Давай! Поговорим.", "", "Давай! Поговорим."},
	} {
		if got := removeRepeatedAcknowledgement(tc.answer, tc.ack); got != tc.want {
			t.Errorf("%q / %q: got %q want %q", tc.answer, tc.ack, got, tc.want)
		}
	}
}
