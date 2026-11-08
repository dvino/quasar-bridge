package bridge

import (
	"context"
	common "github.com/skrashevich/quasar-go-genproto/proto/alice/megamind/protos/common"
	"google.golang.org/protobuf/proto"
	"quasarbridge/internal/dialogue"
	"quasarbridge/internal/llm"
	"strings"
	"testing"
)

func TestClassifiedVoiceAge(t *testing.T) {
	mk := func(name string, confidence common.TBiometryClassification_EConfidence) *common.TBiometryClassification {
		return &common.TBiometryClassification{Simple: []*common.TBiometryClassification_TClassificationSimple{{ClassName: proto.String(name), Tag: proto.String("children"), Confidence: confidence.Enum()}}}
	}
	for _, tc := range []struct {
		name string
		c    *common.TBiometryClassification
		want voiceAge
	}{
		{"missing", nil, ageUnknown},
		{"adult", mk("adult", common.TBiometryClassification_high), ageAdult},
		{"child", mk("child", common.TBiometryClassification_high), ageChild},
		{"uncertain", mk("adult", common.TBiometryClassification_low), ageUnknown},
		{"invalid", mk("other", common.TBiometryClassification_high), ageUnknown},
		{"scores-only", &common.TBiometryClassification{Scores: []*common.TBiometryClassification_TClassificationResult{{ClassName: proto.String("adult"), Tag: proto.String("children"), Confidence: proto.Float64(1)}}}, ageUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifiedVoiceAge(tc.c); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
	c := mk("adult", common.TBiometryClassification_high)
	c.Simple = append(c.Simple, c.Simple[0])
	if classifiedVoiceAge(c) != ageUnknown {
		t.Fatal("ambiguous decision accepted")
	}
}
func TestAgePromptAndConversationIsolation(t *testing.T) {
	base := dialogue.WithSpeaker(t.Context(), "station")
	e := &Engine{}
	for _, mode := range []voiceAge{ageAdult, ageChild, ageUnknown, ageAdult} {
		ctx := dialogue.WithAudience(e.Prepare(base), string(mode))
		ctx = context.WithValue(ctx, voiceAgeKey{}, ageContext{mode: mode})
		e.LLM = completionFunc(func(_ context.Context, messages []llm.Message, _ bool) (string, error) {
			if !strings.Contains(messages[0].Content, ageInstructions(ctx)) {
				t.Fatal("age instruction absent")
			}
			for _, m := range messages[1:] {
				if mode != ageAdult && strings.Contains(m.Content, "adult-secret") {
					t.Fatal("adult history exposed")
				}
			}
			if mode == ageAdult {
				return "adult-secret", nil
			}
			return "family answer", nil
		})
		if _, err := e.Fallback(ctx, "нейтральный вопрос"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVoiceAgeLifecycle(t *testing.T) {
	c := &common.TBiometryClassification{Simple: []*common.TBiometryClassification_TClassificationSimple{{ClassName: proto.String("adult"), Tag: proto.String("children"), Confidence: common.TBiometryClassification_high.Enum()}}}
	p := &nativeRequest{ctx: t.Context()}
	n := &Native{requests: map[string]*nativeRequest{"current": p}}
	n.updateVoiceAge("foreign", c)
	n.updateVoiceAge("", c)
	if p.age != "" {
		t.Fatal("foreign result applied")
	}
	n.updateVoiceAge("current", c)
	if p.age != ageAdult {
		t.Fatal("current result ignored")
	}
	n.updateVoiceAge("current", nil)
	if p.age != ageUnknown {
		t.Fatal("uncertain update retained old decision")
	}
	p.started = true
	n.updateVoiceAge("current", c)
	if p.age != ageUnknown {
		t.Fatal("late result applied")
	}
	p.started = false
	p.canceled = true
	n.updateVoiceAge("current", c)
	if p.age != ageUnknown {
		t.Fatal("canceled result applied")
	}
	p.canceled = false
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	p.ctx = ctx
	n.updateVoiceAge("current", c)
	if p.age != ageUnknown {
		t.Fatal("expired result applied")
	}
}

func TestAllAdultsOwnerOptIn(t *testing.T) {
	t.Setenv("QUASAR_ALL_ADULTS_OWNER", "1")
	p := &nativeRequest{ctx: t.Context()}
	n := &Native{requests: map[string]*nativeRequest{"current": p}}
	for _, tc := range []struct {
		class      string
		confidence common.TBiometryClassification_EConfidence
		want       bool
	}{
		{"adult", common.TBiometryClassification_high, true},
		{"adult", common.TBiometryClassification_low, false},
		{"child", common.TBiometryClassification_high, false},
	} {
		c := &common.TBiometryClassification{Simple: []*common.TBiometryClassification_TClassificationSimple{{ClassName: proto.String(tc.class), Tag: proto.String("children"), Confidence: tc.confidence.Enum()}}}
		n.updateVoiceAge("current", c)
		if p.adultMale != tc.want {
			t.Fatalf("%s %v: owner=%v", tc.class, tc.confidence, p.adultMale)
		}
	}
}

func TestAgeInstructions(t *testing.T) {
	if ageInstructions(context.Background()) != "" {
		t.Fatal("changed unconfigured requests")
	}
	for _, m := range []voiceAge{ageChild, ageAdult, ageUnknown} {
		s := ageInstructions(context.WithValue(context.Background(), voiceAgeKey{}, ageContext{mode: m}))
		if s == "" {
			t.Fatal("missing instruction")
		}
		if m != ageAdult && !strings.Contains(s, "опасны") {
			t.Fatal("missing family safety instruction")
		}
	}
}
