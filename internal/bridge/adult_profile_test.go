package bridge

import (
	"context"
	"strings"
	"testing"

	common "github.com/skrashevich/quasar-go-genproto/proto/alice/megamind/protos/common"
	"google.golang.org/protobuf/proto"
	"quasarbridge/internal/companion"
	"quasarbridge/internal/dialogue"
	"quasarbridge/internal/llm"
)

func TestGenderNeverGrantsAdultProfileAccess(t *testing.T) {
	t.Setenv("QUASAR_ALL_ADULTS_OWNER", "")
	for _, gender := range []string{"male", "female"} {
		p := &nativeRequest{ctx: t.Context()}
		n := &Native{requests: map[string]*nativeRequest{"current": p}}
		n.updateVoiceAge("current", &common.TBiometryClassification{Simple: []*common.TBiometryClassification_TClassificationSimple{
			{Tag: proto.String("children"), ClassName: proto.String("adult"), Confidence: common.TBiometryClassification_high.Enum()},
			{Tag: proto.String("gender"), ClassName: proto.String(gender), Confidence: common.TBiometryClassification_high.Enum()},
		}})
		if p.adultMale {
			t.Fatalf("gender %q granted adult profile access without opt-in", gender)
		}
	}
}

func TestOwnerProfileForRecognizedAdultOptIn(t *testing.T) {
	t.Setenv("QUASAR_ALL_ADULTS_OWNER", "1")
	for _, tc := range []struct {
		class string
		want  bool
	}{{"adult", true}, {"child", false}, {"unknown", false}} {
		t.Run(tc.class, func(t *testing.T) {
			p := &nativeRequest{ctx: t.Context()}
			n := &Native{requests: map[string]*nativeRequest{"current": p}}
			n.updateVoiceAge("current", &common.TBiometryClassification{Simple: []*common.TBiometryClassification_TClassificationSimple{{Tag: proto.String("children"), ClassName: proto.String(tc.class), Confidence: common.TBiometryClassification_high.Enum()}}})
			ctx := context.WithValue(dialogue.WithSpeaker(t.Context(), "device"), voiceAgeKey{}, ageContext{mode: p.age, adultMale: p.adultMale})
			e := &Engine{AdultProfile: &companion.Profile{Base: "PRIVATE_OWNER_CONTEXT"}}
			e.LLM = companionFixture(func(_ context.Context, m []llm.Message, _ bool) (string, error) {
				found := false
				for _, v := range m {
					found = found || strings.Contains(v.Content, "PRIVATE_OWNER_CONTEXT")
				}
				if found != tc.want {
					t.Fatalf("owner profile included=%v want %v", found, tc.want)
				}
				return "Ответ.", nil
			})
			if _, err := e.Fallback(ctx, "привет"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAdultProfileExcludedWithoutMaleDecision(t *testing.T) {
	e := &Engine{AdultProfile: &companion.Profile{Base: "PRIVATE_OWNER_CONTEXT"}}
	e.LLM = companionFixture(func(_ context.Context, m []llm.Message, _ bool) (string, error) {
		for _, message := range m {
			if strings.Contains(message.Content, "PRIVATE_OWNER_CONTEXT") {
				t.Fatal("owner profile disclosed to unidentified adult")
			}
		}
		return "Ответ.", nil
	})
	ctx := context.WithValue(dialogue.WithSpeaker(context.Background(), "device"), voiceAgeKey{}, ageContext{mode: ageAdult})
	if _, err := e.Fallback(ctx, "привет"); err != nil {
		t.Fatal(err)
	}
}
