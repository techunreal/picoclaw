package gateway

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
)

const usageAPIBasePath = "/usage/"

type usageController interface {
	UsageSnapshot() agent.UsageSnapshot
}

type usageModelPricing struct {
	inputPerMTokens  float64
	outputPerMTokens float64
	currency         string
}

type usageModelSummary struct {
	Model string `json:"model"`
	agent.UsageTotals
	EstimatedCost   *float64 `json:"estimated_cost,omitempty"`
	Currency        string   `json:"currency,omitempty"`
	PriceConfigured bool     `json:"price_configured"`
}

type usageSessionSummary struct {
	SessionKey string `json:"session_key"`
	AgentID    string `json:"agent_id,omitempty"`
	Channel    string `json:"channel,omitempty"`
	ChatID     string `json:"chat_id,omitempty"`
	agent.UsageTotals
	Models        []usageModelSummary `json:"models,omitempty"`
	EstimatedCost *float64            `json:"estimated_cost,omitempty"`
	Currency      string              `json:"currency,omitempty"`
}

type usageSummaryResponse struct {
	Totals         agent.UsageTotals     `json:"totals"`
	Models         []usageModelSummary   `json:"models"`
	Sessions       []usageSessionSummary `json:"sessions"`
	EstimatedCost  *float64              `json:"estimated_cost,omitempty"`
	Currency       string                `json:"currency,omitempty"`
	PricedModels   int                   `json:"priced_models"`
	UnpricedModels int                   `json:"unpriced_models"`
	CostComplete   bool                  `json:"cost_complete"`
	StartedAtMS    int64                 `json:"started_at_ms"`
}

type usageAPIHandler struct {
	loop    usageController
	token   string
	pricing map[string]usageModelPricing
}

func newUsageAPIHandler(
	loop usageController,
	token string,
	pricing map[string]usageModelPricing,
) http.Handler {
	return &usageAPIHandler{loop: loop, token: token, pricing: pricing}
}

// registerUsageAPI mounts the internal token usage API on the shared gateway
// mux. Pricing is captured from the current config and refreshed on reload.
func registerUsageAPI(
	manager *channels.Manager,
	loop usageController,
	token string,
	cfg *config.Config,
) {
	if manager == nil || loop == nil {
		return
	}
	manager.Mount(usageAPIBasePath, newUsageAPIHandler(loop, token, buildUsagePricing(cfg)))
}

func buildUsagePricing(cfg *config.Config) map[string]usageModelPricing {
	pricing := map[string]usageModelPricing{}
	if cfg == nil {
		return pricing
	}
	for _, model := range cfg.ModelList {
		if model == nil {
			continue
		}
		if model.InputCostPerMTokens <= 0 && model.OutputCostPerMTokens <= 0 {
			continue
		}
		currency := strings.ToUpper(strings.TrimSpace(model.Currency))
		if currency == "" {
			currency = "USD"
		}
		pricing[strings.TrimSpace(model.ModelName)] = usageModelPricing{
			inputPerMTokens:  model.InputCostPerMTokens,
			outputPerMTokens: model.OutputCostPerMTokens,
			currency:         currency,
		}
	}
	return pricing
}

func (h *usageAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeUsageAPIError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing gateway token.")
		return
	}
	if h.loop == nil {
		writeUsageAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "Usage ledger is unavailable.")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(usageAPIBasePath, "/"))
	path = strings.Trim(path, "/")
	if path != "summary" {
		writeUsageAPIError(w, http.StatusNotFound, "not_found", "Endpoint not found.")
		return
	}
	if r.Method != http.MethodGet {
		writeUsageAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
		return
	}

	writeUsageJSON(w, http.StatusOK, h.buildSummary(h.loop.UsageSnapshot()))
}

func (h *usageAPIHandler) buildSummary(snapshot agent.UsageSnapshot) usageSummaryResponse {
	response := usageSummaryResponse{
		Totals:         snapshot.Totals,
		Models:         make([]usageModelSummary, 0, len(snapshot.Models)),
		Sessions:       make([]usageSessionSummary, 0, len(snapshot.Sessions)),
		PricedModels:   0,
		UnpricedModels: 0,
		CostComplete:   true,
		StartedAtMS:    snapshot.StartedAtMS,
	}

	currencies := map[string]struct{}{}
	totalCost := 0.0
	hasPriced := false
	for _, model := range snapshot.Models {
		item := usageModelSummary{
			Model:       model.Model,
			UsageTotals: model.UsageTotals,
		}
		pricing, ok := h.pricing[model.Model]
		if !ok {
			response.UnpricedModels++
			response.CostComplete = false
		} else {
			cost := costForUsage(model.UsageTotals, pricing)
			item.EstimatedCost = &cost
			item.Currency = pricing.currency
			item.PriceConfigured = true
			response.PricedModels++
			hasPriced = true
			currencies[pricing.currency] = struct{}{}
			if response.Currency == "" {
				response.Currency = pricing.currency
			}
			if pricing.currency == response.Currency {
				totalCost += cost
			} else {
				response.CostComplete = false
			}
		}
		response.Models = append(response.Models, item)
	}
	if hasPriced {
		response.EstimatedCost = &totalCost
	}
	if len(currencies) > 1 {
		response.CostComplete = false
	}

	for _, session := range snapshot.Sessions {
		item := usageSessionSummary{
			SessionKey:  session.SessionKey,
			AgentID:     session.AgentID,
			Channel:     session.Channel,
			ChatID:      session.ChatID,
			UsageTotals: session.UsageTotals,
			Models:      make([]usageModelSummary, 0, len(session.Models)),
		}
		sessionCost := 0.0
		sessionPriced := false
		for _, model := range session.Models {
			modelItem := usageModelSummary{Model: model.Model, UsageTotals: model.UsageTotals}
			if pricing, ok := h.pricing[model.Model]; ok {
				cost := costForUsage(model.UsageTotals, pricing)
				modelItem.EstimatedCost = &cost
				modelItem.Currency = pricing.currency
				modelItem.PriceConfigured = true
				if item.Currency == "" {
					item.Currency = pricing.currency
				}
				if pricing.currency == item.Currency {
					sessionCost += cost
				}
				sessionPriced = true
			}
			item.Models = append(item.Models, modelItem)
		}
		if sessionPriced {
			item.EstimatedCost = &sessionCost
		}
		response.Sessions = append(response.Sessions, item)
	}

	return response
}

func costForUsage(totals agent.UsageTotals, pricing usageModelPricing) float64 {
	return float64(totals.InputTokens)/1_000_000*pricing.inputPerMTokens +
		float64(totals.OutputTokens)/1_000_000*pricing.outputPerMTokens
}

func (h *usageAPIHandler) authorized(r *http.Request) bool {
	if h.token == "" {
		return false
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	presented := strings.TrimSpace(strings.TrimPrefix(auth, prefix))
	return subtle.ConstantTimeCompare([]byte(presented), []byte(h.token)) == 1
}

func writeUsageJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeUsageAPIError(w http.ResponseWriter, status int, code, message string) {
	writeUsageJSON(w, status, map[string]any{"code": code, "error": message})
}
