package bridge

import (
	request "github.com/skrashevich/quasar-go-genproto/proto/alice/protos/api/alicekit/events/alice_request"
	"testing"
)

func TestSafeVoiceClass(t *testing.T) {
	for input, want := range map[string]string{"adult": "adult", "CHILD": "child", "children": "children", "person-secret": "unrecognized", "token=value": "unrecognized", "": "unrecognized"} {
		if got := safeVoiceClass(input); got != want {
			t.Fatalf("unexpected allowed label: %q", got)
		}
	}
}
func TestVoiceTraceMissingClassification(t *testing.T) {
	t.Setenv("QUASAR_VOICE_CLASS_TRACE", "1")
	n := &Native{}
	n.traceVoiceClass("VoiceInput", nil)
	n.traceVoiceClass("VoiceInput", &request.TAliceRequestProto{})
}
