package bridge

import (
	"context"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/encoding/protowire"
	"quasarbridge/internal/wire"
)

func TestSpeechDeliveryKeepsTextAndRemovesURLFromVoice(t *testing.T) {
	var sent []wire.Frame
	n := NewNative(t.Context(), func(_ context.Context, _ websocket.MessageType, b []byte) error {
		sent = append(sent, wire.Decode(b, wire.ClientToServer))
		return nil
	}, func(_ context.Context, _ websocket.MessageType, b []byte) error {
		sent = append(sent, wire.Decode(b, wire.ServerToClient))
		return nil
	}, nil)
	defer n.Close()
	p := &nativeRequest{id: "fixture", ctx: t.Context(), ttsID: "tts"}
	text := "Источник AP ([apnews.com](https://apnews.com/story))."
	if err := n.deferredPart(p, text, true); err != nil {
		t.Fatal(err)
	}
	part := sent[0].Server.GetEvent().GetDeferredAliceResponse().GetBaseResponse()
	if part.GetText() != text {
		t.Fatal("display text changed")
	}
	_, _, size := protowire.ConsumeTag(part.ProtoReflect().GetUnknown())
	voice, _ := protowire.ConsumeString(part.ProtoReflect().GetUnknown()[size:])
	if strings.Contains(voice, "apnews.com") {
		t.Fatalf("address in voice: %q", voice)
	}
	if err := n.synthesize(p, text); err != nil {
		t.Fatal(err)
	}
	if got := sent[1].Client.GetEvent().GetTtsGenerate().GetText(); got != voice {
		t.Fatalf("TTS %q != voice %q", got, voice)
	}
}
