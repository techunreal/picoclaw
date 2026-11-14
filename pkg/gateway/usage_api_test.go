package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sipeed/picoclaw/pkg/agent"
)

type fakeUsageController struct {
	snapshot agent.UsageSnapshot
}

func (f *fakeUsageController) UsageSnapshot() agent.UsageSnapshot {
	return f.snapshot
}

func TestUsageAPIRequiresToken(t *testing.T) {
	h := newUsageAPIHandler(&fakeUsageController{}, "secret", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/usage/summary", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestUsageAPIComputesEstimatedCost(t *testing.T) {
	controller := &fakeUsageController{snapshot: agent.UsageSnapshot{
		Totals: agent.UsageTotals{InputTokens: 1_000_000, OutputTokens: 500_000, TotalTokens: 1_500_000, LLMCalls: 2},
		Models: []agent.UsageModelTotals{
			{Model: "priced", UsageTotals: agent.UsageTotals{InputTokens: 1_000_000, OutputTokens: 500_000, TotalTokens: 1_500_000}},
			{Model: "unpriced", UsageTotals: agent.UsageTotals{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}},
		},
		Sessions: []agent.UsageSessionTotals{{
			SessionKey:  "s1",
			AgentID:     "alpha",
			Channel:     "pico",
			ChatID:      "pico:1",
			UsageTotals: agent.UsageTotals{InputTokens: 1_000_010, OutputTokens: 500_005, TotalTokens: 1_500_015},
			Models: []agent.UsageModelTotals{
				{Model: "priced", UsageTotals: agent.UsageTotals{InputTokens: 1_000_000, OutputTokens: 500_000, TotalTokens: 1_500_000}},
				{Model: "unpriced", UsageTotals: agent.UsageTotals{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}},
			},
		}},
	}}
	h := newUsageAPIHandler(controller, "secret", map[string]usageModelPricing{
		"priced": {inputPerMTokens: 3, outputPerMTokens: 6, currency: "USD"},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/usage/summary", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp usageSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.EstimatedCost == nil {
		t.Fatal("estimated_cost is nil")
	}
	// 1M input * $3/M + 0.5M output * $6/M = 3 + 3 = 6 USD.
	if got := *resp.EstimatedCost; got < 5.999 || got > 6.001 {
		t.Fatalf("estimated_cost = %v, want 6", got)
	}
	if resp.PricedModels != 1 || resp.UnpricedModels != 1 {
		t.Fatalf("priced/unpriced = %d/%d, want 1/1", resp.PricedModels, resp.UnpricedModels)
	}
	if resp.CostComplete {
		t.Fatal("cost_complete = true with an unpriced model")
	}
	if len(resp.Models) != 2 || resp.Models[0].EstimatedCost == nil {
		t.Fatalf("unexpected models: %#v", resp.Models)
	}
	if len(resp.Sessions) != 1 || resp.Sessions[0].EstimatedCost == nil {
		t.Fatalf("unexpected sessions: %#v", resp.Sessions)
	}
}

func TestUsageAPIUnknownEndpoint(t *testing.T) {
	h := newUsageAPIHandler(&fakeUsageController{}, "secret", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/usage/nope", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
