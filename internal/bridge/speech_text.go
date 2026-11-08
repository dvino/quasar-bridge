package bridge

import (
	"net/url"
	"regexp"
	"strings"
)

var speechMarkdownLink = regexp.MustCompile(`\[([^\]]+)\]\(https?://[^\s)]+\)`)
var speechURL = regexp.MustCompile(`(?i)\b(?:https?://|www\.)[^\s<>()\[\]]+`)

// speechText removes addresses from speech while leaving the displayed answer intact.
func speechText(text string) string {
	text = speechMarkdownLink.ReplaceAllStringFunc(text, func(link string) string {
		parts := speechMarkdownLink.FindStringSubmatch(link)
		label := parts[1]
		address := link[strings.Index(link, "](")+2 : len(link)-1]
		if u, err := url.Parse(address); err == nil && strings.EqualFold(strings.TrimPrefix(label, "www."), strings.TrimPrefix(u.Hostname(), "www.")) {
			return ""
		}
		return label
	})
	text = speechURL.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "()", "")
	text = strings.TrimSpace(text)
	if text == "" {
		return "Ссылка доступна в текстовом ответе."
	}
	return text
}
