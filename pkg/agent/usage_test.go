package agent

import (
	"context"
	"testing"

	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestUsageStoreRecordsModelAndSessionTotals(t *testing.T) {
	store := newUsageStore()
	store.Record("alpha", "session-1", "pico", "pico:s1", "model-a", 100, 50, 150)
	store.Record("alpha", "session-1", "pico", "pico:s1", "model-a", 20, 10, 30)
	store.Record("alpha", "session-2", "pico", "pico:s2", "model-b", 5, 5, 10)

	snapshot := store.Snapshot()
	if snapshot.Totals.InputTokens != 125 || snapshot.Totals.OutputTokens != 65 ||
		snapshot.Totals.TotalTokens != 190 || snapshot.Totals.LLMCalls != 3 {
		t.Fatalf("unexpected totals: %#v", snapshot.Totals)
	}
	if len(snapshot.Models) != 2 {
		t.Fatalf("models = %d, want 2", len(snapshot.Models))
	}
	if snapshot.Models[0].Model != "model-a" || snapshot.Models[0].TotalTokens != 180 {
		t.Fatalf("unexpected model totals: %#v", snapshot.Models)
	}
	if len(snapshot.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(snapshot.Sessions))
	}
	if snapshot.Sessions[0].SessionKey != "session-2" {
		t.Fatalf("sessions should be newest first, got %#v", snapshot.Sessions)
	}
}

type usageTestProvider struct{}

func (p *usageTestProvider) Chat(
	context.Context,
	[]providers.Message,
	[]providers.ToolDefinition,
	string,
	map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content: "ok",
		Usage: &providers.UsageInfo{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}, nil
}

func (p *usageTestProvider) GetDefaultModel() string { return "test-model" }

func TestRunTurnRecordsUsage(t *testing.T) {
	al, agent, cleanup := newTurnCoordTestLoop(t, &usageTestProvider{})
	defer cleanup()

	pipeline := NewPipeline(al)
	for i := 0; i < 2; i++ {
		ts := newTurnState(agent, makeTestProcessOpts("usage-session"), turnEventScope{
			turnID:  "usage-turn",
			context: newTurnContext(nil, nil, nil),
		})
		if _, err := al.runTurn(context.Background(), ts, pipeline); err != nil {
			t.Fatalf("runTurn() error = %v", err)
		}
	}

	snapshot := al.UsageSnapshot()
	if snapshot.Totals.InputTokens != 20 || snapshot.Totals.OutputTokens != 10 ||
		snapshot.Totals.TotalTokens != 30 || snapshot.Totals.LLMCalls != 2 {
		t.Fatalf("unexpected usage totals: %#v", snapshot.Totals)
	}
	if len(snapshot.Models) != 1 || snapshot.Models[0].Model != "test-model" {
		t.Fatalf("unexpected model buckets: %#v", snapshot.Models)
	}
	if len(snapshot.Sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(snapshot.Sessions))
	}
	if snapshot.Sessions[0].TotalTokens != 30 {
		t.Fatalf("session tokens = %d, want 30", snapshot.Sessions[0].TotalTokens)
	}
}
