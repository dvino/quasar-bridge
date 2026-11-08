package bridge

import (
	request "github.com/skrashevich/quasar-go-genproto/proto/alice/protos/api/alicekit/events/alice_request"
	"log/slog"
	"os"
	"strings"
)

// Never log arbitrary classifier strings: schemas can carry personal data.
func safeVoiceClass(s string) string {
	switch strings.ToLower(s) {
	case "child", "children", "kid", "adult", "unknown", "age", "male", "female":
		return strings.ToLower(s)
	default:
		return "unrecognized"
	}
}
func (n *Native) traceVoiceClass(kind string, req *request.TAliceRequestProto) {
	if os.Getenv("QUASAR_VOICE_CLASS_TRACE") != "1" || req == nil {
		return
	}
	c := req.GetRequest().GetEvent().GetBiometryClassification()
	n.mu.Lock()
	p := n.requests[req.GetVoiceInputMessageId()]
	linked := p != nil && req.GetVoiceInputMessageId() != ""
	started, final := false, false
	if linked {
		started, final = p.started, p.final
	}
	n.mu.Unlock()
	slog.Info("voice_class_trace", "kind", kind, "classification_present", c != nil, "scores", len(c.GetScores()), "simple", len(c.GetSimple()), "partial", c.GetPartialNumber(), "linked", linked, "started", started, "asr_final", final)
	for _, r := range c.GetScores() {
		slog.Info("voice_class_label", "source", "scores", "class", safeVoiceClass(r.GetClassName()), "tag", safeVoiceClass(r.GetTag()), "confidence", r.GetConfidence())
	}
	for _, r := range c.GetSimple() {
		slog.Info("voice_class_label", "source", "simple", "class", safeVoiceClass(r.GetClassName()), "tag", safeVoiceClass(r.GetTag()), "confidence", r.GetConfidence().String())
	}
}
