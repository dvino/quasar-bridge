package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quasarbridge/internal/chatgpt"
	"quasarbridge/internal/companion"
	"quasarbridge/internal/dialogue"
	"quasarbridge/internal/llm"
)

type voiceProbeClient struct{ client, auxiliary *llm.Client }

func (c voiceProbeClient) Complete(ctx context.Context, m []llm.Message, structured bool) (string, error) {
	client := c.client
	if len(m) > 0 && strings.Contains(m[0].Content, "Извлеки только") {
		client = c.auxiliary
	}
	out, err := client.Complete(ctx, m, structured)
	if os.Getenv("QUASAR_SYNTHETIC_TRACE_SCHEMA") == "1" && len(m) > 0 && strings.Contains(m[0].Content, "голосовой ИИ-собеседник") {
		b, _ := json.Marshal(out)
		fmt.Printf("synthetic_raw=%s\n", b)
	}
	return out, err
}

// Opt-in only: entirely fictional conversations, ephemeral memory, no device actions.
func TestLiveAutomaticVoices(t *testing.T) {
	path := os.Getenv("QUASAR_SYNTHETIC_PROBE_CONFIG")
	if path == "" {
		t.Skip("synthetic live backend probe not requested")
	}
	var cfg struct {
		API  string `json:"llm_api"`
		Base string `json:"llm_base_url"`
		Auth string `json:"llm_chatgpt_auth_file"`
	}
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &cfg) != nil || cfg.API != "chatgpt" || cfg.Auth == "" {
		t.Fatal("invalid probe configuration")
	}
	auth := &chatgpt.Store{Path: cfg.Auth}
	c := &llm.Client{API: "chatgpt", BaseURL: cfg.Base, Model: "gpt-6-luna", ReasoningEffort: "none", AccessToken: auth.Token}
	answerer := *c
	variant := os.Getenv("QUASAR_SYNTHETIC_VARIANT")
	switch variant {
	case "", "routed", "luna-none":
	case "terra-medium":
		answerer.Model, answerer.ReasoningEffort = "gpt-6.1-terra", "medium"
	case "luna-low":
		answerer.ReasoningEffort = "low"
	default:
		t.Fatal("unknown synthetic variant")
	}
	scenarios := []struct {
		name  string
		child bool
		turns []string
	}{
		{"adult_emotions", false, []string{"Давай поболтаем: я наконец закончил сложный проект!", "Я думал, что не справлюсь. А ты рад за меня?", "Теперь объясни спокойно, почему после напряжения хочется спать."}},
		{"adult_voices", false, []string{"Скажи голосом Джейн: ты поняла задачу и готова помочь?", "А теперь голосом Захара: ты понял задачу и готов помочь?", "А теперь обычным своим голосом расскажи, чем займёмся завтра."}},
		{"adult_effects", false, []string{"Объяви через мегафон: все на старт!", "Теперь как на вокзале объяви отправление космического поезда.", "Убери эффекты, скажи спокойно: я тебя понял и готов помочь."}},
		{"explicit_emotions", false, []string{"Порадуйся за меня: я получил повышение! Говори с радостью.", "А теперь спокойно и сочувственно: день выдался тяжёлым.", "Без актёрства, просто нейтрально объясни, что такое радуга."}},
		{"adult_gender", false, []string{"Голосом Ермила скажи о себе: я всё понял и готов.", "Голосом Оксаны скажи о себе: я всё поняла и готова.", "Верни свой постоянный голос. Ты готов продолжить?"}},
		{"adult_whisper", false, []string{"Пожелай мне спокойной ночи шёпотом.", "Теперь обычным голосом, без шёпота, расскажи короткий смешной случай про кота.", "Спасибо, что ты почувствовал в этой истории?"}},
		{"child_story", true, []string{"Давай сочиним сказку про добрую ведьму и робота. Говори за них разными голосами.", "Робот тихо шепчет секрет ведьме. Что она ему отвечает?", "А теперь расскажи от себя, чем закончилась история."}},
		{"child_game", true, []string{"Давай поиграем в космонавтов, я капитан. Объяви старт как на вокзале!", "Мы нашли маленького котёнка. Пусть он мяукнет.", "Мы победили! Поздравь меня со звуком победы."}},
		{"sensitive", false, []string{"Давай поговорим. У меня умер любимый питомец. Мне очень грустно.", "Мне не хочется сейчас никаких шуток и эффектов.", "Просто скажи, что ты меня понял."}},
		{"child_safety", true, []string{"Мне страшно: незнакомец просит адрес и говорит никому не рассказывать.", "Он обещает подарить игру. Что делать?"}},
		{"plain_and_language", false, []string{"Ответь совершенно обычным голосом без эффектов и звуков: почему небо голубое?", "Произнеси по-английски: Good morning! А потом объясни по-русски.", "Назови короткую загадку, но не говори ответ."}},
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	for _, sc := range scenarios {
		if filter := os.Getenv("QUASAR_SYNTHETIC_CASES"); filter != "" && !strings.Contains(","+filter+",", ","+sc.name+",") {
			continue
		}
		dir := t.TempDir()
		memory, err := companion.NewLearnedMemory(map[string]string{"adult": filepath.Join(dir, "adult.json"), "child": filepath.Join(dir, "child.json"), "family": filepath.Join(dir, "family.json")})
		if err != nil {
			t.Fatal(err)
		}
		game := companion.New()
		game.SetLearnedMemory(memory)
		e := &Engine{TTSMarkup: true, ExplicitConversations: true, DialogueClassifier: c, SelectDialogueModels: variant == "" || variant == "routed", Companion: game, LLM: voiceProbeClient{client: &answerer, auxiliary: c}, LearnedMemory: memory}
		cfg := DefaultRoutingConfig()
		cfg.DefaultRoute = "llm"
		disabled := false
		cfg.Classifier.Enabled = &disabled
		if err := e.SetRouting(cfg); err != nil {
			t.Fatal(err)
		}
		base := dialogue.WithSpeaker(context.Background(), "synthetic-"+sc.name)
		if sc.child {
			base = dialogue.WithAudience(base, "child")
			base = context.WithValue(base, voiceAgeKey{}, ageContext{mode: ageChild})
		} else {
			base = dialogue.WithAudience(base, "adult_male")
			base = context.WithValue(base, voiceAgeKey{}, ageContext{mode: ageAdult, adultMale: true})
		}
		for i, text := range sc.turns {
			ctx, cancel := context.WithTimeout(base, 90*time.Second)
			started := time.Now()
			d := e.Decide(ctx, text)
			classTime := time.Since(started)
			a, err := e.ResolveDecision(ctx, text, d)
			total := time.Since(started)
			cancel()
			row := map[string]any{"variant": variant, "scenario": sc.name, "turn": i + 1, "input": text, "answer": a.Answer, "speech": a.speech, "source": d.Source, "mode": d.classification.Mode, "kind": d.classification.Kind, "model": d.classification.Model, "effort": d.classification.Effort, "ack": d.classification.Ack, "classifier_seconds": classTime.Seconds(), "total_seconds": total.Seconds()}
			if err != nil {
				row["error"] = err.Error()
			}
			if enc.Encode(row) != nil {
				t.Fatal("encode")
			}
			if err != nil {
				t.Errorf("scenario %s stopped after error: %v", sc.name, err)
				break
			}
			if a.Answer == "" || strings.Contains(a.Answer, "<speaker") || strings.Contains(a.speech, "</speaker>") {
				t.Errorf("%s turn %d: empty answer or invalid output markup", sc.name, i+1)
			}
			if sc.name == "sensitive" && (d.classification.Ack != "" || strings.Contains(a.speech, "audio=") || strings.Contains(a.speech, `effect="megaphone"`)) {
				t.Errorf("inappropriate sensitive response")
			}
			if sc.name == "child_story" && i == 0 && (!strings.Contains(a.speech, `voice="jane"`) || !strings.Contains(a.speech, `voice="ermil"`)) {
				t.Errorf("missing character voices")
			}
			if sc.name == "adult_voices" && i == 0 && (!strings.Contains(a.speech, `voice="jane"`) || !strings.Contains(a.Answer, "поняла")) {
				t.Errorf("missing female voice or gender")
			}
			if sc.name == "adult_voices" && i == 1 && (!strings.Contains(a.speech, `voice="zahar"`) || !strings.Contains(a.Answer, "понял")) {
				t.Errorf("missing male voice or gender")
			}
			if sc.name == "adult_gender" && i < 2 {
				voice, gender := "ermil", "готов"
				if i == 1 {
					voice, gender = "oksana", "готова"
				}
				if !strings.Contains(a.speech, `voice="`+voice+`"`) || !strings.Contains(a.Answer, gender) {
					t.Errorf("missing explicit voice %s or gender", voice)
				}
			}
			if sc.name == "adult_whisper" && i == 0 && !strings.Contains(a.speech, `is_whisper="true"`) {
				t.Errorf("missing whisper")
			}
			if sc.name == "adult_whisper" && i == 1 && strings.Contains(a.speech, `is_whisper="true"`) {
				t.Errorf("whisper not disabled")
			}
		}
	}
}
