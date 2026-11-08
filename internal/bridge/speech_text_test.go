package bridge

import "testing"

func TestSpeechTextRemovesAddressesButKeepsSourceNames(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"Новости AP: [статья](https://apnews.com/story).", "Новости AP: статья."},
		{"Источник AP ([apnews.com](https://apnews.com/story)).", "Источник AP ."},
		{"Источник Axios: https://example.com/a?x=1", "Источник Axios:"},
		{"Источник BBC — www.bbc.com/news", "Источник BBC —"},
		{"https://example.com/", "Ссылка доступна в текстовом ответе."},
		{"Привет, источник AP.", "Привет, источник AP."},
	} {
		if got := speechText(tc.input); got != tc.want {
			t.Errorf("speechText(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
