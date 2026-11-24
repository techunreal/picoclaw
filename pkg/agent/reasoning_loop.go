package agent

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/sipeed/picoclaw/pkg/providers"
)

// errReasoningLoop is returned when streaming reasoning repeats itself badly
// enough that the turn should be retried instead of waiting for the token limit.
var errReasoningLoop = errors.New("reasoning loop detected")

const (
	// recoveryRetryNote is appended (as a user message) only to the single
	// recovery attempt of a turn. It never persists to session history.
	recoveryRetryNote = "The previous attempt got stuck repeating itself. " +
		"Answer directly and briefly; do not draft or plan."

	reasoningLoopResponse = "The model got stuck repeating itself and returned " +
		"no answer. Try a shorter request or switch models."

	emptyLengthResponse = "The model hit its output limit before producing an " +
		"answer. Try a shorter request."
)

// reasoningLoopDetector looks for a long block that is repeated many times in
// the accumulated reasoning. It deliberately uses whole-text counts (not a
// sliding window) because some loops spread the same draft over thousands of
// characters with short self-talk in between.
type reasoningLoopDetector struct {
	lastChecked   int
	minChars      int
	minBlockChars int
	minRepeats    int

	cutChars   int
	blockChars int
	repeats    int
}

func newReasoningLoopDetector() *reasoningLoopDetector {
	return &reasoningLoopDetector{
		minChars:      2500,
		minBlockChars: 150,
		minRepeats:    5,
	}
}

// Observe returns true when the accumulated reasoning contains a loop. It is
// cheap enough to call on every stream chunk; the full scan runs at most once
// per 500 new characters.
func (d *reasoningLoopDetector) Observe(accumulated string) bool {
	if d == nil || len(accumulated) < d.minChars {
		return false
	}
	if d.lastChecked > 0 && len(accumulated) < d.lastChecked+500 {
		return false
	}
	d.lastChecked = len(accumulated)

	lines := make([]string, 0, strings.Count(accumulated, "\n")+1)
	lineRunes := make([]int, 0, cap(lines))
	for _, line := range strings.Split(accumulated, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.Join(strings.Fields(line), " ")
		lines = append(lines, line)
		lineRunes = append(lineRunes, utf8.RuneCountInString(line))
	}
	if len(lines) == 0 {
		return false
	}

	bestCount := 0
	bestSize := 0
	for n := 1; n <= 3; n++ {
		counts := make(map[string]int)
		sizes := make(map[string]int)
		for i := 0; i+n <= len(lines); i++ {
			size := 0
			for j := 0; j < n; j++ {
				size += lineRunes[i+j] + 1
			}
			if size < d.minBlockChars {
				continue
			}
			block := strings.Join(lines[i:i+n], "\n")
			counts[block]++
			sizes[block] = size
		}
		for block, count := range counts {
			if count > bestCount {
				bestCount = count
				bestSize = sizes[block]
			}
		}
	}

	if bestCount < d.minRepeats {
		return false
	}
	d.cutChars = utf8.RuneCountInString(accumulated)
	d.blockChars = bestSize
	d.repeats = bestCount
	return true
}

// withRecoveryNote returns a copy of messages with the retry-only note appended.
func withRecoveryNote(messages []providers.Message) []providers.Message {
	out := make([]providers.Message, 0, len(messages)+1)
	out = append(out, messages...)
	out = append(out, providers.Message{Role: "user", Content: recoveryRetryNote})
	return out
}

// shouldRecoverEmptyLength reports whether a response consumed the output
// budget without producing any answer.
func shouldRecoverEmptyLength(response *providers.LLMResponse) bool {
	if response == nil || len(response.ToolCalls) > 0 {
		return false
	}
	if strings.TrimSpace(response.Content) != "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(response.FinishReason)) {
	case "length", "max_tokens":
		return true
	default:
		return false
	}
}
