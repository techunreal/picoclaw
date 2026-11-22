package commands

import (
	"strings"
	"testing"
)

func TestFormatContextStatsShowsSystemAndInputBudget(t *testing.T) {
	stats := &ContextStats{
		UsedTokens:        50000,
		TotalTokens:       65536,
		HistoryTokens:     40000,
		SystemTokens:      8000,
		SkillTokens:       5000,
		CompressAtTokens:  57344,
		SummarizeAtTokens: 49152,
		UsedPercent:       87,
		MessageCount:      20,
	}
	out := formatContextStats(stats)

	for _, want := range []string{
		"Messages: 20",
		"Used: ~50000 / 65536 tokens (76%)",
		"History: ~40000 tokens",
		"System: ~8000 tokens (skills ~5000)",
		"Input budget: 57344 tokens (compress at)",
		"Summarize at: 49152 tokens",
		"Compression progress: 87%",
		"Remaining: ~7344 tokens",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatContextStats() missing %q:\n%s", want, out)
		}
	}
}
