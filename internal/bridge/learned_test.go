package bridge

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	common "github.com/skrashevich/quasar-go-genproto/proto/alice/megamind/protos/common"
	"google.golang.org/protobuf/proto"
	"quasarbridge/internal/companion"
	"quasarbridge/internal/dialogue"
	"quasarbridge/internal/llm"
)

func TestForgetLearnedKeepsSeedAndOtherScope(t *testing.T) {
	dir := t.TempDir()
	m, err := companion.NewLearnedMemory(map[string]string{"adult": filepath.Join(dir, "adult.json"), "child": filepath.Join(dir, "child.json"), "family": filepath.Join(dir, "family.json")})
	if err != nil {
		t.Fatal(err)
	}
	fact := companion.LearnedFact{Topic: "игры", Key: "игра", Value: "загадки", Kind: "fact", Source: "child"}
	if err := m.Apply("child", []companion.LearnedFact{fact}); err != nil {
		t.Fatal(err)
	}
	e := &Engine{LearnedMemory: m}
	ctx := context.WithValue(context.Background(), voiceAgeKey{}, ageContext{mode: ageChild})
	if ok, err := e.forgetLearned(ctx, "забудь всё обо мне"); !ok || err != nil {
		t.Fatalf("forget: %v %v", ok, err)
	}
	facts, err := m.Snapshot("child")
	if err != nil || len(facts) != 0 {
		t.Fatal("not deleted")
	}
	if ok, _ := e.forgetLearned(ctx, "удали общую выученную память"); ok {
		t.Fatal("child cleared shared memory")
	}
	if ok, _ := e.forgetLearned(ctx, "в сказке забудь всё обо мне"); ok {
		t.Fatal("fiction deleted memory")
	}
}

func TestAdultLearningRequiresClassifiedAdultMale(t *testing.T) {
	t.Setenv("QUASAR_ALL_ADULTS_OWNER", "1")
	dir := t.TempDir()
	m, err := companion.NewLearnedMemory(map[string]string{"adult": filepath.Join(dir, "adult.json"), "child": filepath.Join(dir, "child.json"), "family": filepath.Join(dir, "family.json")})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	client := companionFixture(func(_ context.Context, messages []llm.Message, _ bool) (string, error) {
		calls.Add(1)
		if strings.Contains(messages[0].Content, "Извлеки только") {
			return `{"candidates":[{"scope":"adult","topic":"хобби","key":"походы","value":"любит походы","kind":"fact","evidence":"я люблю походы"}]}`, nil
		}
		return "Здорово.", nil
	})
	e := &Engine{LearnedMemory: m, LLM: client}
	p := &nativeRequest{ctx: t.Context()}
	n := &Native{requests: map[string]*nativeRequest{"current": p}}
	n.updateVoiceAge("current", &common.TBiometryClassification{Simple: []*common.TBiometryClassification_TClassificationSimple{{Tag: proto.String("children"), ClassName: proto.String("adult"), Confidence: common.TBiometryClassification_high.Enum()}}})
	ctx := dialogue.WithSpeaker(context.Background(), "a")
	ctx = context.WithValue(ctx, voiceAgeKey{}, ageContext{mode: p.age, adultMale: p.adultMale})
	if _, err := e.Fallback(ctx, "я люблю походы"); err != nil {
		t.Fatal(err)
	}
	facts, err := m.Snapshot("adult")
	if err != nil || len(facts) != 1 {
		t.Fatalf("facts=%v err=%v", facts, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
	unknown := dialogue.WithSpeaker(context.Background(), "b")
	if _, err := e.Fallback(unknown, "я люблю походы"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("unclassified voice invoked extraction")
	}
	child, err := m.Snapshot("child")
	if err != nil || len(child) != 0 {
		t.Fatal("adult memory leaked")
	}
}
