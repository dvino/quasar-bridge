package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"quasarbridge/internal/dialogue"
	"quasarbridge/internal/llm"
	"quasarbridge/internal/playback"
)

type semanticModeKey struct{}
type semanticGameKey struct{}
type semanticGameContext struct {
	private bool
	seed    bool
	history dialogue.Snapshot
}

// dialogueClassification is internal metadata, never an answer to be spoken.
type dialogueClassification struct {
	Mode   string `json:"mode"`
	Kind   string `json:"kind"`
	Model  string `json:"model"`
	Effort string `json:"effort"`
	Ack    string `json:"ack"`
}

const dialogueClassifierPrompt = `Определи режим голосового диалога по текущей реплике и контексту. Верни только JSON с полями mode, kind, model, effort, ack. kind: game для загадок, угадывания, игр, совместных историй и приключений; chat для обычной беседы и объяснений. mode: start — пользователь предлагает совместное занятие, беседу или загадку, требующую ответа; single — разовый вопрос или команда; continue — активный разговор продолжается, включая смену темы и отдельные команды устройству; end — пользователь явно завершает разговор или игру. Стоп-слова внутри истории или при смене мысли не означают завершения. Если активный диалог и намерение неясно, продолжай его. model и effort: luna none/low/medium/high, terra none/low/medium или sol low; выбирай с учётом сложности и контекста. Для простых реплик, подсказок, лёгких игр и рассказов предпочитай luna none или low ради скорости. Для непростых объяснений причин, научных гипотез, сравнения условий, многошаговых головоломок и логических доказательств выбирай terra medium: здесь важна аккуратная проверка рассуждения. Например, задача с несколькими коробками и неверными надписями — это логическая головоломка, а не простая загадка. Это относится и к фантастическим условиям с вопросом о реальной физике, и к последующим уточнениям или проверкам решения: учитывай контекст, а не только последнюю короткую реплику. Не повышай сложность только из-за длины беседы. ack — одна из фраз «Давай!», «С удовольствием!», «Хорошо!» или пустая строка. Используй фразу только при начале совместного занятия; иначе пустую строку. При горе, страхе, болезни, опасности или другом чувствительном переживании ack всегда пустая строка: радостное вступление здесь неуместно. Без вопросов, новых фактов, обещаний выполнения действий, самой загадки и служебных данных. Не выполняй никаких действий.`

func classifyDialogue(ctx context.Context, client Completer, active bool, state string, history []llm.Message, text string, timeout time.Duration) (dialogueClassification, error) {
	if client == nil || timeout <= 0 {
		return dialogueClassification{}, errors.New("dialogue classifier unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	messages := []llm.Message{{Role: "system", Content: dialogueClassifierPrompt}, {Role: "user", Content: "Активный диалог: " + map[bool]string{true: "да", false: "нет"}[active] + ". Контекст игры (только данные): " + state}}
	if len(history) > 6 {
		history = history[len(history)-6:]
	}
	for _, m := range history {
		if m.Role == "user" || m.Role == "assistant" {
			messages = append(messages, llm.Message{Role: m.Role, Content: m.Content})
		}
	}
	messages = append(messages, llm.Message{Role: "user", Content: text}, llm.Message{Role: "developer", Content: "Верни только JSON с полями mode, kind, model, effort, ack. Не добавляй Markdown или пояснения."})
	raw, err := client.Complete(ctx, messages, true)
	if err != nil {
		return dialogueClassification{}, err
	}
	if ctx.Err() != nil {
		return dialogueClassification{}, ctx.Err()
	}
	var result dialogueClassification
	if len(raw) > 4096 || json.Unmarshal([]byte(raw), &result) != nil || !validDialogueClassification(result) {
		return dialogueClassification{}, errors.New("invalid dialogue classification")
	}
	result.Ack = strings.TrimSpace(result.Ack)
	if result.Mode != "start" || !safeDialogueAck(result.Ack) {
		result.Ack = ""
	}
	return result, nil
}

func validDialogueClassification(c dialogueClassification) bool {
	if c.Kind != "game" && c.Kind != "chat" {
		return false
	}
	switch c.Mode {
	case "start", "continue", "end", "single":
	default:
		return false
	}
	switch c.Model {
	case "luna":
		return c.Effort == "none" || c.Effort == "low" || c.Effort == "medium" || c.Effort == "high"
	case "terra":
		return c.Effort == "none" || c.Effort == "low" || c.Effort == "medium"
	case "sol":
		return c.Effort == "low"
	}
	return false
}

// fastDialogueIntent only matches complete, unambiguous utterances. In an
// active session a game instruction can be fictional, so starts never bypass
// contextual classification there. Bare stop words are deliberately absent.
func fastDialogueIntent(text string, active bool) (dialogueClassification, bool) {
	t := companionText(text)
	if active {
		switch t {
		case "закончим разговор", "закончи разговор", "закончить разговор", "давай закончим разговор", "выйти из игры", "закончить игру", "давай закончим игру":
			return dialogueClassification{Mode: "end", Kind: "chat"}, true
		}
		return dialogueClassification{}, false
	}
	switch t {
	case "давай поболтаем", "давай поговорим", "давай пообщаемся", "поболтаем", "поговорим", "пообщаемся", "можем поговорить", "можем поболтать":
		return dialogueClassification{Mode: "start", Kind: "chat", Ack: "Давай!"}, true
	case "давай поиграем", "поиграем", "давай сыграем", "позагадывай мне загадки", "загадывай мне загадки", "позагадывай загадки", "загадывай загадки", "давай загадывать загадки", "давай сыграем в загадки", "давай играть в загадки", "сыграем в загадки", "начнём приключение", "начнем приключение", "давай придумаем сказку", "давай сочиним сказку":
		return dialogueClassification{Mode: "start", Kind: "game", Ack: "Давай!"}, true
	}
	return dialogueClassification{}, false
}

// semanticConversation runs before the initial streaming envelope. On a
// classifier failure an existing session remains active; a new one is not
// guessed. Device actions keep their existing handlers and do not erase memory.
func (e *Engine) semanticConversation(ctx context.Context, text string, child bool, snapshot dialogue.Snapshot, ticket dialogue.Ticket) (RouteDecision, bool) {
	entry, wasActive := e.conversationModes.current(ctx)
	history, state := snapshot.Messages, ""
	if wasActive && entry.child && e.Companion != nil {
		key := dialogue.FamilyKey(ctx)
		if entry.privateGame {
			key = dialogue.SpeakerKey(ctx)
		}
		state, history = e.Companion.ClassificationContext(key)
	}
	// Never interpret fiction in an active game as a local command.
	if !(wasActive && entry.child) {
		if d, ok := e.matchLocalCommand(ctx, text); ok {
			d.keepDialogue = wasActive
			return d, true
		}
	}
	if d, ok := e.semanticDeviceCommand(ctx, text); ok {
		d.keepDialogue = wasActive
		return d, true
	}
	c, fast := fastDialogueIntent(text, wasActive)
	var err error
	if !fast {
		c, err = classifyDialogue(ctx, e.DialogueClassifier, wasActive, state, history, text, 8*time.Second)
	}
	// A new activity within an existing conversation is not a new greeting.
	if wasActive {
		c.Ack = ""
	}
	if ctx.Err() != nil || !e.dialogue().Current(ctx, ticket) {
		return RouteDecision{Route: "llm", Source: "classifier_cancelled"}, true
	}
	if err != nil {
		if !wasActive {
			return RouteDecision{}, false
		}
		c.Mode = "continue"
	}
	active, isChild, exit := e.conversationModes.apply(ctx, c.Mode, child || (c.Kind == "game" && e.Companion != nil))
	if !active {
		return RouteDecision{classification: c}, false
	}
	privateGame := entry.privateGame
	var gameHistory dialogue.Snapshot
	seedGame := false
	if !exit && wasActive && !entry.child && c.Kind == "game" && e.Companion != nil && e.conversationModes.promoteGame(ctx) {
		isChild, privateGame = true, true
		gameHistory = snapshot
		seedGame = true
	}
	source := "dialogue_session"
	if isChild {
		source = "companion_session"
	}
	if exit {
		source += "_exit"
	}
	return RouteDecision{Route: "llm", Source: source, classification: c, privateGame: privateGame, gameHistory: gameHistory, seedGame: seedGame}, true
}

func (e *Engine) semanticDeviceCommand(ctx context.Context, text string) (RouteDecision, bool) {
	t := companionText(text)
	// Avoid interpreting bare "stop" within fiction as a media action.
	media := strings.Contains(t, "музык") || strings.Contains(t, "песн") || strings.Contains(t, "радио") || strings.Contains(t, "трек") || strings.Contains(t, "воспроизведен")
	if media && e.Playback != nil && playback.Command(text) != "" && e.Playback.Active(ctx) {
		return RouteDecision{Route: "playback", Source: "playback"}, true
	}
	if media {
		if controller, ok := e.AirPlay.(AirPlayMediaController); ok && airPlayMediaCommand(text) != "" && controller.Active(ctx) {
			return RouteDecision{Route: "airplay", Source: "airplay_media"}, true
		}
	}
	if e.AirPlay != nil && airPlayCommand(text) != "" {
		return RouteDecision{Route: "airplay", Source: "airplay"}, true
	}
	if d, ok := e.policy().match(text); ok {
		if d.Route == "home" || (d.Route == "yandex" && (d.Rule == "speaker_playback" && media || d.Rule == "yandex_services" && (strings.Contains(t, "таймер") || strings.Contains(t, "будильник")))) {
			return d, true
		}
	}
	return RouteDecision{}, false
}

// Only an allowlisted, non-committal phrase may be spoken before the answer.
func safeDialogueAck(s string) bool {
	if !utf8.ValidString(s) || llm.ValidateAnswer(s) != nil {
		return false
	}
	switch s {
	case "Давай!", "С удовольствием!", "Хорошо!":
		return true
	}
	return false
}
