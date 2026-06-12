package modelinfo

import (
	"strconv"
	"strings"

	"github.com/docker/docker-agent/pkg/effort"
)

// SupportedThinkingLevels returns the ordered thinking-effort levels a
// reasoning-capable model accepts for user-selectable cycling. It combines
// the provider API's effort vocabulary with per-model gating of the optional
// top tiers: not every model accepts xhigh or max, and offering them blindly
// makes the API reject the request. Callers are expected to have already
// established that the model reasons at all.
func SupportedThinkingLevels(provider, modelID string) []effort.Level {
	return effort.SupportedLevels(true, thinkingLevelMap(provider, modelID))
}

// thinkingLevelMap builds the per-model capability map consumed by
// [effort.SupportedLevels].
func thinkingLevelMap(provider, modelID string) effort.LevelMap {
	switch providerFamily(provider) {
	case "anthropic":
		// The Anthropic effort scale starts at low ([effort.ForAnthropic]
		// maps minimal onto low), so offering minimal would duplicate low.
		m := effort.LevelMap{effort.Minimal: false}
		if top := anthropicTopEffort(modelID); top != "" {
			m[top] = true
		}
		return m
	case "openai":
		if openAISupportsXHighEffort(modelID) {
			return effort.LevelMap{effort.XHigh: true}
		}
		return nil
	case "google":
		return nil
	default:
		// Unknown providers (e.g. dmr) get the conservative low/medium/high
		// scale; minimal is far from universally accepted.
		return effort.LevelMap{effort.Minimal: false}
	}
}

// providerFamily normalises a provider type onto the model family whose API
// defines the thinking-level vocabulary, tolerating aliases such as
// "amazon-bedrock" (hosting Anthropic models) or "vertexai" (Gemini).
func providerFamily(providerType string) string {
	p := normalize(providerType)
	switch {
	case strings.Contains(p, "anthropic"), strings.Contains(p, "claude"), strings.Contains(p, "bedrock"):
		return "anthropic"
	case strings.Contains(p, "google"), strings.Contains(p, "gemini"), strings.Contains(p, "vertex"):
		return "google"
	case strings.Contains(p, "openai"), strings.Contains(p, "azure"):
		return "openai"
	default:
		return p
	}
}

// anthropicTopEffort returns the highest selectable effort above high for a
// Claude model: Max for Opus 4.6 (the only model accepting effort "max"),
// XHigh for Opus 4.7+ and the Fable family, and "" for every other Claude
// model, which tops out at high.
//
// Works on bare Anthropic IDs ("claude-opus-4-7", "claude-opus-4.7") as well
// as Bedrock-style IDs with regional prefixes ("us.anthropic.claude-opus-4-7").
func anthropicTopEffort(modelID string) effort.Level {
	m := normalize(modelID)
	if strings.Contains(m, "fable") {
		return effort.XHigh
	}
	_, rest, found := strings.Cut(m, "opus-4")
	if !found {
		return ""
	}
	if rest == "" || (rest[0] != '-' && rest[0] != '.') {
		return ""
	}
	minor, width := leadingInt(rest[1:])
	// Long digit runs are date stamps (claude-opus-4-20250514 is Opus 4.0),
	// not minor versions.
	if width == 0 || width > 2 {
		return ""
	}
	switch {
	case minor >= 7:
		return effort.XHigh
	case minor == 6:
		return effort.Max
	default:
		return ""
	}
}

// openAISupportsXHighEffort reports whether an OpenAI model accepts
// reasoning effort "xhigh". Only gpt-5.2 and later minor versions do; the
// o-series and earlier gpt-5 releases top out at high.
func openAISupportsXHighEffort(modelID string) bool {
	m := normalize(modelID)
	const prefix = "gpt-5."
	if !strings.HasPrefix(m, prefix) {
		return false
	}
	minor, width := leadingInt(m[len(prefix):])
	return width > 0 && minor >= 2
}

// leadingInt parses the run of decimal digits at the start of s, returning
// its value and width. A zero width means s does not start with a digit.
func leadingInt(s string) (value, width int) {
	for width < len(s) && s[width] >= '0' && s[width] <= '9' {
		width++
	}
	if width == 0 {
		return 0, 0
	}
	n, err := strconv.Atoi(s[:width])
	if err != nil {
		return 0, 0
	}
	return n, width
}
