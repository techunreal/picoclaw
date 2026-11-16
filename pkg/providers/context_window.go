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
// right endpoint. It falls back to scanning the model list by alias or model id.
func FindModelConfigForContext(cfg *config.Config, name string) *config.ModelConfig {
	if cfg == nil {
		return nil
	}

	target := strings.ToLower(strings.TrimSpace(name))
	if target != "" {
		var fallback *config.ModelConfig
		for _, item := range cfg.ModelList {
			if item == nil {
				continue
			}
			alias := strings.ToLower(strings.TrimSpace(item.ModelName))
			modelID := strings.ToLower(strings.TrimSpace(item.Model))
			if alias != target && modelID != target {
				continue
			}
			if strings.TrimSpace(item.APIBase) == "" {
				continue
			}
			if alias == target {
				return item
			}
			if fallback == nil {
				fallback = item
			}
		}
		if fallback != nil {
			return fallback
		}
	}

	if resolved, err := ResolveModelConfig(cfg, name); err == nil && strings.TrimSpace(resolved.APIBase) != "" {
		return resolved
	}
	return nil
}

// DetectModelContextWindow asks the model endpoint (/models) for the advertised
// context length. OpenAI-compatible servers such as llama.cpp, vLLM, LM Studio
// and Ollama expose this either as context_length, context_window,
// max_model_len or max_context_length. When the value cannot be determined it
// returns 0 so callers can fall back to their own heuristic.
func DetectModelContextWindow(m *config.ModelConfig) int {
	if m == nil {
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
			Meta             struct {
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
