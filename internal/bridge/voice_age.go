package bridge

import (
	"context"
	common "github.com/skrashevich/quasar-go-genproto/proto/alice/megamind/protos/common"
	"os"
)

type voiceAge string

const (
	ageUnknown voiceAge = "family"
	ageChild   voiceAge = "child"
	ageAdult   voiceAge = "adult"
)

type voiceAgeKey struct{}
type ageContext struct {
	mode      voiceAge
	adultMale bool
}

func voiceAudience(mode voiceAge, adultMale bool) string {
	if mode == ageAdult && adultMale {
		return "adult_male"
	}
	return string(mode)
}

// Only the explicit, confident age decision counts; scores list alternatives.
func classifiedVoiceAge(c *common.TBiometryClassification) voiceAge {
	if c == nil {
		return ageUnknown
	}
	found := false
	mode := ageUnknown
	for _, r := range c.GetSimple() {
		if r.GetTag() != "children" {
			continue
		}
		if found {
			return ageUnknown
		}
		found = true
		if r.GetConfidence() != common.TBiometryClassification_high {
			return ageUnknown
		}
		switch r.GetClassName() {
		case "adult":
			mode = ageAdult
		case "child":
			mode = ageChild
		default:
			return ageUnknown
		}
	}
	return mode
}

// Caller holds n.mu. Never reuse an unmatched or late result.
func (n *Native) updateVoiceAge(id string, c *common.TBiometryClassification) {
	if id == "" {
		return
	}
	p := n.requests[id]
	if p == nil || p.started || p.canceled || p.ctx.Err() != nil {
		return
	}
	p.age = classifiedVoiceAge(c)
	// Neither age nor gender establishes identity. Adult profile access requires
	// an explicit household-wide opt-in; never infer ownership from gender.
	p.adultMale = p.age == ageAdult && os.Getenv("QUASAR_ALL_ADULTS_OWNER") == "1"
}

func ageInstructions(ctx context.Context) string {
	a, ok := ctx.Value(voiceAgeKey{}).(ageContext)
	if !ok {
		return ""
	}
	switch a.mode {
	case ageAdult:
		return " Собеседник определён как взрослый. Отвечай обычным взрослым языком, не упрощай объяснение до детского уровня."
	case ageChild:
		return " Собеседник — ребёнок примерно 6–10 лет. Объясняй коротко, простыми словами и понятными примерами. Не предоставляй взрослый, сексуально откровенный или жестокий контент и опасные инструкции. На чувствительные вопросы отвечай бережно и уместно возрасту, при необходимости предложи обратиться к родителям."
	default:
		return " Возраст собеседника не определён уверенно. Используй безопасный семейный стиль: понятные короткие объяснения, без взрослого, сексуально откровенного или жестокого контента и опасных инструкций. Не предполагай, что говоришь со взрослым."
	}
}
