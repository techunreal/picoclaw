package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
)

const contextProbeTimeout = 3 * time.Second

type cachedContextWindow struct {
	value     int
	expiresAt time.Time
}

var contextWindowCache sync.Map

// FindModelConfigForContext resolves a model alias or raw model id to a model
// entry that carries an API base, so context-window detection can query the
// right endpoint. Entries without an explicit api_base inherit their provider's
// default endpoint instead of borrowing another provider's template.
func FindModelConfigForContext(cfg *config.Config, name string) *config.ModelConfig {
	if cfg == nil {
		return nil
	}

	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return nil
	}

	var aliasMatch, modelMatch *config.ModelConfig
	for _, item := range cfg.ModelList {
		if item == nil || item.Disabled {
			continue
		}
		if aliasMatch == nil && strings.ToLower(strings.TrimSpace(item.ModelName)) == target {
			aliasMatch = item
		}
		if modelMatch == nil && strings.ToLower(strings.TrimSpace(item.Model)) == target {
			modelMatch = item
		}
	}
	for _, candidate := range []*config.ModelConfig{aliasMatch, modelMatch} {
		if candidate == nil {
			continue
		}
		if resolved := modelConfigWithAPIBase(candidate); resolved != nil {
			return resolved
		}
	}

	// Fall back to the resolver for explicit "provider/model" references. A
	// bare model id with no configured default provider must not fall back to
	// the OpenAI template: that would probe the wrong endpoint.
	if !strings.Contains(name, "/") && strings.TrimSpace(cfg.Agents.Defaults.Provider) == "" {
		return nil
	}
	resolved, err := ResolveModelConfig(cfg, name)
	if err != nil {
		return nil
	}
	return modelConfigWithAPIBase(resolved)
}

// modelConfigWithAPIBase returns a copy of the model config with a usable API
// base, deriving it from the provider metadata when the entry omits it.
func modelConfigWithAPIBase(model *config.ModelConfig) *config.ModelConfig {
	if model == nil {
		return nil
	}
	clone := *model
	if strings.TrimSpace(clone.APIBase) == "" {
		clone.APIBase = ResolveAPIBase(&clone)
	}
	if strings.TrimSpace(clone.APIBase) == "" {
		return nil
	}
	return &clone
}

// SupportsContextWindowDetection limits automatic probing to endpoints that
// advertise context limits in their /models response. Self-hosted servers use
// a variety of field names; OpenCode Go/Zen use limit.context.
func SupportsContextWindowDetection(m *config.ModelConfig) bool {
	if m == nil {
		return false
	}
	protocol := strings.ToLower(strings.TrimSpace(m.Provider))
	if prefix, _ := ExtractProtocol(m); strings.TrimSpace(prefix) != "" {
		protocol = strings.ToLower(strings.TrimSpace(prefix))
	}
	switch protocol {
	case "openai-compatible", "vllm", "lmstudio", "ollama", "llamacpp", "llama.cpp", "custom", "",
		"opencode-go", "opencode-zen":
		return true
	default:
		return false
	}
}

// DetectModelContextWindow asks the model endpoint (/models) for the advertised
// context length. OpenAI-compatible servers such as llama.cpp, vLLM, LM Studio
// and Ollama expose this either as context_length, context_window,
// max_model_len or max_context_length. When the value cannot be determined it
// returns 0 so callers can fall back to their own heuristic.
func DetectModelContextWindow(m *config.ModelConfig) int {
	if m == nil || !SupportsContextWindowDetection(m) {
		return 0
	}

	base := strings.TrimRight(strings.TrimSpace(m.APIBase), "/")
	if base == "" {
		base = strings.TrimRight(DefaultAPIBaseForProtocol(strings.ToLower(strings.TrimSpace(m.Provider))), "/")
	}
	if base == "" {
		return 0
	}

	modelID := strings.ToLower(strings.TrimSpace(m.Model))
	key := strings.ToLower(base) + "|" + modelID
	if cached, ok := contextWindowCache.Load(key); ok {
		entry := cached.(cachedContextWindow)
		if time.Now().Before(entry.expiresAt) {
			return entry.value
		}
	}

	value := probeContextWindow(base, modelID, m.APIKey())
	contextWindowCache.Store(key, cachedContextWindow{
		value:     value,
		expiresAt: time.Now().Add(10 * time.Minute),
	})
	return value
}

func probeContextWindow(base, modelID, apiKey string) int {
	ctx, cancel := context.WithTimeout(context.Background(), contextProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return 0
	}
	if key := strings.TrimSpace(apiKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0
	}

	var payload struct {
		Data []struct {
			ID               string `json:"id"`
			ContextLength    int    `json:"context_length"`
			ContextWindow    int    `json:"context_window"`
			MaxModelLen      int    `json:"max_model_len"`
			MaxContextLength int    `json:"max_context_length"`
			Limit            struct {
				Context int `json:"context"`
				Input   int `json:"input"`
				Output  int `json:"output"`
			} `json:"limit"`
			Meta struct {
				Nctx int `json:"n_ctx"`
			} `json:"meta"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0
	}

	best := 0
	for _, item := range payload.Data {
		value := firstPositive(
			item.Limit.Context,
			item.ContextLength,
			item.ContextWindow,
			item.MaxModelLen,
			item.MaxContextLength,
			item.Meta.Nctx,
		)
		if value <= 0 {
			continue
		}
		if modelID != "" && strings.EqualFold(strings.TrimSpace(item.ID), modelID) {
			return value
		}
		if best == 0 {
			best = value
		}
	}
	// Some servers (llama.cpp) report a path-like id; when there is exactly one
	// model in the listing, its context length still belongs to this endpoint.
	if len(payload.Data) == 1 {
		return best
	}
	return 0
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
