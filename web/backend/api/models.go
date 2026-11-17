package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/audio/asr"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// registerModelRoutes binds model list management endpoints to the ServeMux.
func (h *Handler) registerModelRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/models", h.handleListModels)
	mux.HandleFunc("GET /api/models/default-chain", h.handleGetDefaultChain)
	mux.HandleFunc("POST /api/models/fetch", h.handleFetchModels)
	mux.HandleFunc("GET /api/models/catalog", h.handleListCatalogs)
	mux.HandleFunc("DELETE /api/models/catalog/{id}", h.handleDeleteCatalog)
	mux.HandleFunc("POST /api/models", h.handleAddModel)
	mux.HandleFunc("POST /api/models/default", h.handleSetDefaultModel)
	mux.HandleFunc("PUT /api/models/default-chain", h.handleUpdateDefaultChain)
	mux.HandleFunc("PUT /api/models/by-name/{name...}", h.handleUpdateModelByName)
	mux.HandleFunc("DELETE /api/models/by-name/{name...}", h.handleDeleteModelByName)
	mux.HandleFunc("PUT /api/models/enabled/{name...}", h.handleSetModelEnabled)
	mux.HandleFunc("PUT /api/models/{index}", h.handleUpdateModel)
	mux.HandleFunc("DELETE /api/models/{index}", h.handleDeleteModel)
	mux.HandleFunc("POST /api/models/{index}/test", h.handleTestModel)
	mux.HandleFunc("POST /api/models/test-inline", h.handleTestInlineModel)
}

// modelResponse is the JSON structure returned for each model in the list.
// All ModelConfig fields are included so the frontend can display and edit them.
type modelResponse struct {
	Index       int      `json:"index"`
	ModelName   string   `json:"model_name"`
	Provider    string   `json:"provider,omitempty"`
	Model       string   `json:"model"`
	APIBase     string   `json:"api_base,omitempty"`
	APIKey      string   `json:"api_key"`
	APIKeys     []string `json:"api_keys,omitempty"`
	APIKeyCount int      `json:"api_key_count,omitempty"`
	Proxy       string   `json:"proxy,omitempty"`
	AuthMethod  string   `json:"auth_method,omitempty"`
	// Advanced fields
	ConnectMode          string                      `json:"connect_mode,omitempty"`
	Workspace            string                      `json:"workspace,omitempty"`
	RPM                  int                         `json:"rpm,omitempty"`
	MaxTokensField       string                      `json:"max_tokens_field,omitempty"`
	RequestTimeout       int                         `json:"request_timeout,omitempty"`
	ThinkingLevel        string                      `json:"thinking_level,omitempty"`
	ToolSchemaTransform  string                      `json:"tool_schema_transform,omitempty"`
	Streaming            config.ModelStreamingConfig `json:"streaming,omitempty"`
	ExtraBody            map[string]any              `json:"extra_body,omitempty"`
	CustomHeaders        map[string]string           `json:"custom_headers,omitempty"`
	ContextWindow        int                         `json:"context_window,omitempty"`
	InputCostPerMTokens  float64                     `json:"input_cost_per_million,omitempty"`
	OutputCostPerMTokens float64                     `json:"output_cost_per_million,omitempty"`
	Currency             string                      `json:"currency,omitempty"`
	// Meta
	Enabled             bool   `json:"enabled"`
	Available           bool   `json:"available"`
	Status              string `json:"status"`
	IsDefault           bool   `json:"is_default"`
	IsVirtual           bool   `json:"is_virtual"`
	DefaultModelAllowed bool   `json:"default_model_allowed"`
}

type defaultChainResponse struct {
	DefaultModel       string   `json:"default_model"`
	FallbackChain      []string `json:"fallback_chain"`
	Applied            bool     `json:"applied"`
	PendingAgents      []string `json:"pending_agents,omitempty"`
	GatewayUnavailable bool     `json:"gateway_unavailable,omitempty"`
	Warnings           []string `json:"warnings,omitempty"`
	RestartRequired    bool     `json:"restart_required"`
}

type defaultChainRequest struct {
	DefaultModel  string   `json:"default_model"`
	FallbackChain []string `json:"fallback_chain"`
}

func normalizeStoredModelConfig(mc *config.ModelConfig, defaultProvider string) bool {
	if mc == nil {
		return false
	}

	changed := false
	modelName := strings.TrimSpace(mc.ModelName)
	if modelName != mc.ModelName {
		mc.ModelName = modelName
		changed = true
	}
	model := strings.TrimSpace(mc.Model)
	if model != mc.Model {
		mc.Model = model
		changed = true
	}
	provider := strings.TrimSpace(mc.Provider)
	if provider != mc.Provider {
		mc.Provider = provider
		changed = true
	}
	authMethod := strings.ToLower(strings.TrimSpace(mc.AuthMethod))
	if authMethod != mc.AuthMethod {
		mc.AuthMethod = authMethod
		changed = true
	}

	if provider != "" {
		normalizedProvider := providers.NormalizeProvider(provider)
		if providers.IsSupportedModelProvider(normalizedProvider) && normalizedProvider != provider {
			mc.Provider = normalizedProvider
			changed = true
		}
		if mc.Provider == "elevenlabs" {
			if _, strippedModel, found := strings.Cut(
				model,
				"/",
			); found &&
				providers.NormalizeProvider(strings.TrimSpace(provider)) == "elevenlabs" {
				strippedModel = strings.TrimSpace(strippedModel)
				if strippedModel != "" && strippedModel != mc.Model {
					mc.Model = strippedModel
					changed = true
				}
			}
			if strings.TrimSpace(mc.Model) != asr.ElevenLabsSupportedModelID() {
				mc.Model = asr.ElevenLabsSupportedModelID()
				changed = true
			}
		}
		return changed
	}

	effectiveProvider, modelID := providers.SplitModelProviderAndID(model, defaultProvider)
	if effectiveProvider == "" {
		return changed
	}
	if mc.Provider != effectiveProvider {
		mc.Provider = effectiveProvider
		changed = true
	}
	if mc.Model != modelID {
		mc.Model = modelID
		changed = true
	}
	return changed
}

func normalizeIncomingModelConfig(mc *config.ModelConfig, defaultProvider string) {
	if mc == nil {
		return
	}

	mc.ModelName = strings.TrimSpace(mc.ModelName)
	mc.Model = strings.TrimSpace(mc.Model)
	mc.Provider = strings.TrimSpace(mc.Provider)
	mc.AuthMethod = strings.ToLower(strings.TrimSpace(mc.AuthMethod))
	if mc.Provider == "" {
		mc.Provider, mc.Model = providers.SplitModelProviderAndID(mc.Model, defaultProvider)
	} else {
		mc.Provider = providers.NormalizeProvider(mc.Provider)
		if mc.Provider == "elevenlabs" {
			if _, strippedModel, found := strings.Cut(mc.Model, "/"); found {
				strippedModel = strings.TrimSpace(strippedModel)
				if strippedModel != "" {
					mc.Model = strippedModel
				}
			}
		}
	}
	if mc.Provider == "antigravity" && mc.AuthMethod == "" {
		mc.AuthMethod = "oauth"
	}
}

func createAllowedForProvider(provider string) bool {
	normalized := providers.NormalizeProvider(provider)
	switch normalized {
	case "bedrock":
		// Bedrock currently authenticates through the AWS SDK credential chain
		// (env vars, shared profiles, IAM roles, etc.), and this Web layer does
		// not yet have a reliable preflight check for those credential sources.
		// Keep it creatable in the catalog and let provider construction/runtime
		// return the concrete AWS error when the environment is incomplete.
		return true
	case "claude-cli", "codex-cli":
		return cliProviderCreateAllowedFromCurrentStatus(normalized)
	default:
		return providers.IsCreatableModelProvider(normalized)
	}
}

// cliProviderCreateAllowedFromCurrentStatus intentionally reuses the existing
// local model status pipeline so provider catalog gating follows the same CLI
// executable probe used by launcher readiness.
func cliProviderCreateAllowedFromCurrentStatus(provider string) bool {
	status := modelConfigurationStatus(&config.ModelConfig{
		Provider: provider,
		Model:    provider,
	})
	return status.Available
}

func modelProviderOptionsForResponse() []providers.ModelProviderOption {
	options := providers.ModelProviderOptions()
	for i := range options {
		options[i].CreateAllowed = createAllowedForProvider(options[i].ID)
	}
	return options
}

func defaultModelAllowedForModelConfig(mc *config.ModelConfig, defaultProvider string) bool {
	provider, _ := modelConfigProviderAndID(mc, defaultProvider)
	if providers.IsDefaultModelProvider(provider) {
		return true
	}
	return mc != nil && providers.IsCustomOpenAICompatibleProvider(provider, mc.APIBase)
}

func validateIncomingModelConfig(mc *config.ModelConfig, existing *config.ModelConfig) error {
	if mc == nil {
		return fmt.Errorf("model config is required")
	}
	if err := mc.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(mc.Provider) == "" {
		return fmt.Errorf("provider is required")
	}
	customProvider := providers.IsCustomOpenAICompatibleProvider(mc.Provider, mc.APIBase)
	if !providers.IsSupportedModelProvider(mc.Provider) && !customProvider {
		return fmt.Errorf("provider %q is not supported", mc.Provider)
	}
	if mc.Provider == "openai-compatible" && strings.TrimSpace(mc.APIBase) == "" {
		return fmt.Errorf("api_base is required for provider %q", mc.Provider)
	}
	if mc.Provider == "elevenlabs" && strings.TrimSpace(mc.Model) != asr.ElevenLabsSupportedModelID() {
		return fmt.Errorf("provider %q only supports model %q", mc.Provider, asr.ElevenLabsSupportedModelID())
	}
	if !createAllowedForProvider(mc.Provider) && !customProvider {
		if existing == nil {
			return fmt.Errorf("provider %q is not available for new models", mc.Provider)
		}
		existingProvider, _ := providers.ExtractProtocol(existing)
		if providers.NormalizeProvider(existingProvider) != mc.Provider {
			return fmt.Errorf("provider %q is not available for selection", mc.Provider)
		}
	}
	return nil
}

func normalizeStoredModelProviders(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}

	changed := false
	defaultProvider := defaultChainProvider(cfg)
	for _, model := range cfg.ModelList {
		if normalizeStoredModelConfig(model, defaultProvider) {
			changed = true
		}
	}
	return changed
}

func normalizeStoredModelNames(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	changed := false
	for _, model := range cfg.ModelList {
		if model == nil {
			continue
		}
		modelName := strings.TrimSpace(model.ModelName)
		if modelName != model.ModelName {
			model.ModelName = modelName
			changed = true
		}
	}
	return changed
}

func trimModelNameList(names []string) []string {
	trimmed := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		trimmed = append(trimmed, name)
	}
	return trimmed
}

func dedupeModelNameList(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(names))
	deduped := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		deduped = append(deduped, name)
	}
	return deduped
}

// modelNameTaken reports whether another model in cfg already uses modelName.
// exceptIndex skips the entry being updated (-1 when creating a new model).
func modelNameTaken(cfg *config.Config, modelName string, exceptIndex int) bool {
	modelName = strings.TrimSpace(modelName)
	if cfg == nil || modelName == "" {
		return false
	}
	for i, model := range cfg.ModelList {
		if i == exceptIndex || model == nil {
			continue
		}
		if strings.TrimSpace(model.ModelName) == modelName {
			return true
		}
	}
	return false
}

func modelConfigsByName(cfg *config.Config) map[string][]*config.ModelConfig {
	models := make(map[string][]*config.ModelConfig, len(cfg.ModelList))
	for _, model := range cfg.ModelList {
		if model == nil {
			continue
		}
		name := strings.TrimSpace(model.ModelName)
		if name == "" {
			continue
		}
		models[name] = append(models[name], model)
	}
	return models
}

func modelConfigsByNameExceptIndex(
	cfg *config.Config,
	modelName string,
	exceptIndex int,
) []*config.ModelConfig {
	modelName = strings.TrimSpace(modelName)
	if cfg == nil || modelName == "" {
		return nil
	}
	models := make([]*config.ModelConfig, 0, 1)
	for i, model := range cfg.ModelList {
		if i == exceptIndex || model == nil {
			continue
		}
		if strings.TrimSpace(model.ModelName) == modelName {
			models = append(models, model)
		}
	}
	return models
}

func validateDefaultChainModelSet(
	modelName string,
	models []*config.ModelConfig,
	defaultContext bool,
	defaultProvider string,
) error {
	if len(models) == 0 {
		if defaultContext {
			return fmt.Errorf("default model %q not found in model_list", modelName)
		}
		return fmt.Errorf("fallback model %q not found in model_list", modelName)
	}
	for _, modelCfg := range models {
		if modelCfg.Disabled {
			if defaultContext {
				return fmt.Errorf("model %q is disabled and cannot be used as the default chat model", modelName)
			}
			return fmt.Errorf("model %q is disabled and cannot be used in the fallback chain", modelName)
		}
		if modelCfg.IsVirtual() {
			if defaultContext {
				return fmt.Errorf("cannot set virtual model %q as default", modelName)
			}
			return fmt.Errorf("cannot add virtual model %q to fallback_chain", modelName)
		}
		if !defaultModelAllowedForModelConfig(modelCfg, defaultProvider) {
			if defaultContext {
				return fmt.Errorf("model %q cannot be used as the default chat model", modelName)
			}
			return fmt.Errorf("model %q cannot be used in the fallback chain", modelName)
		}
		effectiveModelCfg := modelCfg
		if strings.TrimSpace(modelCfg.Provider) == "" {
			clone := *modelCfg
			normalizeStoredModelConfig(&clone, defaultProvider)
			effectiveModelCfg = &clone
		}
		if !hasModelConfiguration(effectiveModelCfg) {
			return rawModelConfigurationError(modelName, defaultContext)
		}
	}
	return nil
}

func validateDefaultChainReference(
	cfg *config.Config,
	modelName string,
	models []*config.ModelConfig,
	defaultContext bool,
) error {
	if len(models) > 0 {
		return validateDefaultChainModelSet(
			modelName,
			models,
			defaultContext,
			defaultChainProvider(cfg),
		)
	}

	resolved, err := providers.ResolveModelConfig(cfg, modelName)
	if err != nil || resolved == nil {
		ref := providers.ParseModelRef(modelName, defaultChainProvider(cfg))
		if ref != nil &&
			strings.TrimSpace(ref.Provider) != "" &&
			strings.TrimSpace(ref.Model) != "" &&
			!providers.IsDefaultModelProvider(ref.Provider) {
			if defaultContext {
				return fmt.Errorf("model %q cannot be used as the default chat model", modelName)
			}
			return fmt.Errorf("model %q cannot be used in the fallback chain", modelName)
		}
		if ref != nil &&
			strings.TrimSpace(ref.Provider) != "" &&
			strings.TrimSpace(ref.Model) != "" &&
			providers.IsDefaultModelProvider(ref.Provider) {
			return rawModelConfigurationError(modelName, defaultContext)
		}
		return validateDefaultChainModelSet(
			modelName,
			nil,
			defaultContext,
			defaultChainProvider(cfg),
		)
	}
	if err := validateDefaultChainModelSet(
		modelName,
		[]*config.ModelConfig{resolved},
		defaultContext,
		defaultChainProvider(cfg),
	); err != nil {
		return err
	}
	if !rawTemplateHasConfiguration(resolved) {
		return rawModelConfigurationError(modelName, defaultContext)
	}
	return nil
}

func rawTemplateHasConfiguration(modelCfg *config.ModelConfig) bool {
	return hasModelConfiguration(modelCfg)
}

func rawModelConfigurationError(modelName string, defaultContext bool) error {
	if defaultContext {
		return fmt.Errorf("default model %q has no configured provider", modelName)
	}
	return fmt.Errorf("fallback model %q has no configured provider", modelName)
}

func defaultChainProvider(cfg *config.Config) string {
	if cfg != nil && strings.TrimSpace(cfg.Agents.Defaults.Provider) != "" {
		return providers.NormalizeProvider(cfg.Agents.Defaults.Provider)
	}
	return "openai"
}

func defaultChainReferenceIdentity(cfg *config.Config, modelName string) string {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return ""
	}

	defaultProvider := defaultChainProvider(cfg)
	var modelList []*config.ModelConfig
	if cfg != nil {
		modelList = cfg.ModelList
	}
	matches := modelConfigsMatchingSignatureRef(modelList, modelName, defaultProvider)
	if len(matches) > 0 {
		modelCfg := matches[0].model
		if alias := strings.TrimSpace(modelCfg.ModelName); alias != "" {
			return "model_name:" + alias
		}
		provider, modelID := providers.ExtractProtocol(modelCfg)
		return providers.ModelKey(provider, modelID)
	}

	ref := providers.ParseModelRef(modelName, defaultProvider)
	if ref == nil {
		return ""
	}
	return providers.ModelKey(ref.Provider, ref.Model)
}

func validateDefaultModelChain(
	cfg *config.Config,
	defaultModel string,
	fallbacks []string,
) ([]string, error) {
	defaultModel = strings.TrimSpace(defaultModel)
	fallbacks = dedupeModelNameList(trimModelNameList(fallbacks))

	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if defaultModel == "" && len(fallbacks) > 0 {
		return nil, fmt.Errorf("default_model is required when fallback_chain is not empty")
	}

	modelsByName := modelConfigsByName(cfg)
	defaultIdentity := ""
	if defaultModel != "" {
		if err := validateDefaultChainReference(
			cfg,
			defaultModel,
			modelsByName[defaultModel],
			true,
		); err != nil {
			return nil, err
		}
		defaultIdentity = defaultChainReferenceIdentity(cfg, defaultModel)
	}

	seenIdentities := make(map[string]struct{}, len(fallbacks))
	validated := make([]string, 0, len(fallbacks))
	for _, fallback := range fallbacks {
		if fallback == defaultModel {
			return nil, fmt.Errorf("default model %q cannot also appear in fallback_chain", defaultModel)
		}
		if err := validateDefaultChainReference(cfg, fallback, modelsByName[fallback], false); err != nil {
			return nil, err
		}
		identity := defaultChainReferenceIdentity(cfg, fallback)
		if identity != "" && identity == defaultIdentity {
			return nil, fmt.Errorf(
				"fallback model %q resolves to the same runtime model as default model %q",
				fallback,
				defaultModel,
			)
		}
		if _, exists := seenIdentities[identity]; identity != "" && exists {
			continue
		}
		if identity != "" {
			seenIdentities[identity] = struct{}{}
		}
		validated = append(validated, fallback)
	}

	return validated, nil
}

func normalizeDefaultChainReferences(cfg *config.Config) {
	if cfg == nil {
		return
	}

	defaultModel := strings.TrimSpace(cfg.Agents.Defaults.ModelName)
	fallbacks := dedupeModelNameList(trimModelNameList(cfg.Agents.Defaults.ModelFallbacks))
	modelsByName := modelConfigsByName(cfg)
	defaultIdentity := ""

	if defaultModel != "" {
		if err := validateDefaultChainReference(
			cfg,
			defaultModel,
			modelsByName[defaultModel],
			true,
		); err != nil {
			defaultModel = ""
		}
		defaultIdentity = defaultChainReferenceIdentity(cfg, defaultModel)
	}
	if defaultModel == "" {
		cfg.Agents.Defaults.ModelName = ""
		cfg.Agents.Defaults.ModelFallbacks = nil
		return
	}

	seenIdentities := make(map[string]struct{}, len(fallbacks))
	validated := make([]string, 0, len(fallbacks))
	for _, fallback := range fallbacks {
		if fallback == defaultModel {
			continue
		}
		if err := validateDefaultChainReference(cfg, fallback, modelsByName[fallback], false); err != nil {
			continue
		}
		identity := defaultChainReferenceIdentity(cfg, fallback)
		if identity != "" && identity == defaultIdentity {
			continue
		}
		if _, exists := seenIdentities[identity]; identity != "" && exists {
			continue
		}
		if identity != "" {
			seenIdentities[identity] = struct{}{}
		}
		validated = append(validated, fallback)
	}

	cfg.Agents.Defaults.ModelName = defaultModel
	cfg.Agents.Defaults.ModelFallbacks = validated
}

// handleListModels returns all model_list entries with masked API keys.
//
//	GET /api/models
func (h *Handler) handleListModels(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	// Normalize legacy provider/model storage in memory so GET can round-trip
	// through the current API shape without mutating the on-disk config.
	normalizeStoredModelProviders(cfg)
	normalizeDefaultChainReferences(cfg)

	defaultModel := cfg.Agents.Defaults.GetModelName()
	fallbackChain := cfg.Agents.Defaults.ModelFallbacks
	modelStatuses := make([]modelConfigurationSummary, len(cfg.ModelList))

	var wg sync.WaitGroup
	wg.Add(len(cfg.ModelList))
	for i, m := range cfg.ModelList {
		go func(i int, m *config.ModelConfig) {
			defer wg.Done()
			modelStatuses[i] = modelConfigurationStatus(m)
		}(i, m)
	}
	wg.Wait()

	models := make([]modelResponse, 0, len(cfg.ModelList))
	for i, m := range cfg.ModelList {
		provider, modelID := providers.ExtractProtocol(m)
		maskedKeys := maskAPIKeyList(m.APIKeys.Values())
		models = append(models, modelResponse{
			Index:                i,
			ModelName:            m.ModelName,
			Provider:             provider,
			Model:                modelID,
			APIBase:              m.APIBase,
			APIKey:               maskAPIKey(m.APIKey()),
			APIKeys:              maskedKeys,
			APIKeyCount:          len(maskedKeys),
			Proxy:                m.Proxy,
			AuthMethod:           m.AuthMethod,
			ConnectMode:          m.ConnectMode,
			Workspace:            m.Workspace,
			RPM:                  m.RPM,
			MaxTokensField:       m.MaxTokensField,
			RequestTimeout:       m.RequestTimeout,
			ThinkingLevel:        m.ThinkingLevel,
			ToolSchemaTransform:  m.ToolSchemaTransform,
			Streaming:            m.Streaming,
			ExtraBody:            m.ExtraBody,
			CustomHeaders:        m.CustomHeaders,
			ContextWindow:        m.ContextWindow,
			InputCostPerMTokens:  m.InputCostPerMTokens,
			OutputCostPerMTokens: m.OutputCostPerMTokens,
			Currency:             m.Currency,
			Enabled:              !m.Disabled,
			Available:            modelStatuses[i].Available,
			Status:               modelStatuses[i].Status,
			IsDefault:            m.ModelName == defaultModel,
			IsVirtual:            m.IsVirtual(),
			DefaultModelAllowed:  defaultModelAllowedForModelConfig(m, defaultChainProvider(cfg)),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"models":           models,
		"total":            len(models),
		"default_model":    defaultModel,
		"default_provider": defaultChainProvider(cfg),
		"fallback_chain":   fallbackChain,
		"provider_options": modelProviderOptionsForResponse(),
	})
}

// handleGetDefaultChain returns the persisted default model and fallback chain.
//
//	GET /api/models/default-chain
func (h *Handler) handleGetDefaultChain(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}
	normalizeStoredModelProviders(cfg)
	normalizeDefaultChainReferences(cfg)

	resp := defaultChainResponse{
		DefaultModel:  strings.TrimSpace(cfg.Agents.Defaults.GetModelName()),
		FallbackChain: trimModelNameList(cfg.Agents.Defaults.ModelFallbacks),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleAddModel appends a new model configuration entry.
//
//	POST /api/models
func (h *Handler) handleAddModel(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	type custom struct {
		config.ModelConfig
		APIKey  string   `json:"api_key"`
		APIKeys []string `json:"api_keys"`
		Enabled *bool    `json:"enabled"`
	}

	var mc custom
	if err = json.Unmarshal(body, &mc); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	if mc.Enabled != nil {
		mc.ModelConfig.Disabled = !*mc.Enabled
	}

	if apiKeys := normalizeAPIKeyList(mc.APIKeys); len(apiKeys) > 0 {
		mc.ModelConfig.APIKeys = config.SimpleSecureStrings(apiKeys...)
	} else if mc.APIKey != "" {
		mc.ModelConfig.SetAPIKey(mc.APIKey)
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}
	normalizeIncomingModelConfig(&mc.ModelConfig, defaultChainProvider(cfg))
	apiKeyReused := false
	if len(mc.ModelConfig.APIKeys) == 0 {
		if key, reusedFrom, ok := findStoredAPIKeyForEndpoint(
			cfg.ModelList,
			mc.Provider,
			mc.APIBase,
		); ok {
			mc.ModelConfig.SetAPIKey(key)
			apiKeyReused = true
			logger.Debugf(
				"reusing stored API key from model %q for new model %q",
				reusedFrom,
				mc.ModelConfig.ModelName,
			)
		}
	}
	if err = validateIncomingModelConfig(&mc.ModelConfig, nil); err != nil {
		http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
		return
	}
	if modelNameTaken(cfg, mc.ModelConfig.ModelName, -1) {
		http.Error(
			w,
			fmt.Sprintf("model name %q already exists", strings.TrimSpace(mc.ModelConfig.ModelName)),
			http.StatusConflict,
		)
		return
	}

	cfg.ModelList = append(cfg.ModelList, &mc.ModelConfig)
	normalizeStoredModelProviders(cfg)
	normalizeDefaultChainReferences(cfg)

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":         "ok",
		"index":          len(cfg.ModelList) - 1,
		"api_key_reused": apiKeyReused,
		"api_key_count":  len(mc.ModelConfig.APIKeys.Values()),
	})
}

// errModelNotFound is returned by resolveModelIndexByName when no model in the
// current configuration carries the requested name.
var errModelNotFound = errors.New("model not found")

// ambiguousModelNameError reports that several models share one name, so a
// name-based update or delete cannot pick a target safely.
type ambiguousModelNameError struct {
	name  string
	count int
}

func (e *ambiguousModelNameError) Error() string {
	return fmt.Sprintf("model name %q is ambiguous: %d models share it", e.name, e.count)
}

// resolveModelIndexByName finds the single model index whose model_name
// matches name. Names with slashes are supported because callers use a
// wildcard path segment.
func resolveModelIndexByName(cfg *config.Config, name string) (int, error) {
	name = strings.TrimSpace(name)
	if cfg != nil {
		matched := -1
		for i, model := range cfg.ModelList {
			if model == nil || strings.TrimSpace(model.ModelName) != name {
				continue
			}
			if matched >= 0 {
				count := 0
				for _, other := range cfg.ModelList {
					if other != nil && strings.TrimSpace(other.ModelName) == name {
						count++
					}
				}
				return -1, &ambiguousModelNameError{name: name, count: count}
			}
			matched = i
		}
		if matched >= 0 {
			return matched, nil
		}
	}
	return -1, fmt.Errorf("%w: %q", errModelNotFound, name)
}

func writeModelResolutionError(w http.ResponseWriter, err error) {
	var ambiguous *ambiguousModelNameError
	switch {
	case errors.As(err, &ambiguous):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, errModelNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleUpdateModel replaces a model configuration entry at the given index.
// If the request body omits api_key (or sends an empty string), the existing
// stored key is preserved so callers can update only api_base / proxy without
// exposing or clearing the secret.
//
//	PUT /api/models/{index}
func (h *Handler) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		http.Error(w, "Invalid index", http.StatusBadRequest)
		return
	}
	h.updateModel(w, r, func(*config.Config) (int, error) { return idx, nil })
}

// handleUpdateModelByName replaces the model resolved by its model_name. This
// keeps updates stable when another client adds or removes models between the
// list request and the save, which would otherwise shift list indices.
//
//	PUT /api/models/by-name/{name}
func (h *Handler) handleUpdateModelByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		http.Error(w, "model name is required", http.StatusBadRequest)
		return
	}
	h.updateModel(w, r, func(cfg *config.Config) (int, error) {
		return resolveModelIndexByName(cfg, name)
	})
}

func (h *Handler) updateModel(
	w http.ResponseWriter,
	r *http.Request,
	resolveIndex func(cfg *config.Config) (int, error),
) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var rawFields map[string]json.RawMessage
	if err = json.Unmarshal(body, &rawFields); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	type custom struct {
		config.ModelConfig
		APIKey      string   `json:"api_key"`
		APIKeys     []string `json:"api_keys"`
		APIKeysKeep []int    `json:"api_keys_keep"`
		Enabled     *bool    `json:"enabled"`
	}

	var mc custom
	if err = json.Unmarshal(body, &mc); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	_, apiKeysProvided := rawFields["api_keys"]
	_, apiKeysKeepProvided := rawFields["api_keys_keep"]
	if _, enabledProvided := rawFields["enabled"]; enabledProvided && mc.Enabled != nil {
		mc.ModelConfig.Disabled = !*mc.Enabled
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}
	normalizeStoredModelProviders(cfg)

	idx, err := resolveIndex(cfg)
	if err != nil {
		writeModelResolutionError(w, err)
		return
	}
	if idx < 0 || idx >= len(cfg.ModelList) {
		http.Error(w, fmt.Sprintf("Index %d out of range (0-%d)", idx, len(cfg.ModelList)-1), http.StatusNotFound)
		return
	}
	oldModelName := strings.TrimSpace(cfg.ModelList[idx].ModelName)

	// Resolve the key list. Omitting both api_keys and api_keys_keep preserves
	// every stored key so unrelated edits never drop failover credentials.
	if _, enabledProvided := rawFields["enabled"]; !enabledProvided {
		if _, disabledProvided := rawFields["disabled"]; !disabledProvided {
			mc.ModelConfig.Disabled = cfg.ModelList[idx].Disabled
		}
	}

	existingKeys := cfg.ModelList[idx].APIKeys.Values()
	nextKeys, keyErr := resolveUpdatedAPIKeys(
		mc.APIKey,
		mc.APIKeys,
		mc.APIKeysKeep,
		apiKeysKeepProvided,
		apiKeysProvided,
		existingKeys,
	)
	if keyErr != nil {
		http.Error(w, fmt.Sprintf("Validation error: %v", keyErr), http.StatusBadRequest)
		return
	}
	mc.ModelConfig.APIKeys = config.SimpleSecureStrings(nextKeys...)
	// Preserve existing ExtraBody when omitted (nil), but clear it when
	// the frontend sends an empty object {} to indicate the field should
	// be removed.
	if mc.ExtraBody == nil {
		mc.ExtraBody = cfg.ModelList[idx].ExtraBody
	} else if len(mc.ExtraBody) == 0 {
		mc.ExtraBody = nil
	}
	// Preserve existing CustomHeaders when omitted (nil), but clear it when
	// the frontend sends an empty object {} to indicate the field should
	// be removed.
	if mc.CustomHeaders == nil {
		mc.CustomHeaders = cfg.ModelList[idx].CustomHeaders
	} else if len(mc.CustomHeaders) == 0 {
		mc.CustomHeaders = nil
	}
	if _, ok := rawFields["tool_schema_transform"]; !ok {
		mc.ToolSchemaTransform = cfg.ModelList[idx].ToolSchemaTransform
	}
	if _, ok := rawFields["streaming"]; !ok {
		mc.Streaming = cfg.ModelList[idx].Streaming
	}
	if _, ok := rawFields["input_cost_per_million"]; !ok {
		mc.InputCostPerMTokens = cfg.ModelList[idx].InputCostPerMTokens
	}
	if _, ok := rawFields["output_cost_per_million"]; !ok {
		mc.OutputCostPerMTokens = cfg.ModelList[idx].OutputCostPerMTokens
	}
	if _, ok := rawFields["currency"]; !ok {
		mc.Currency = cfg.ModelList[idx].Currency
	}
	// Preserve the existing Provider when the caller omits it. This keeps the
	// update API backward-compatible for clients that haven't started sending
	// the new field yet, while still allowing explicit clearing via "".
	if _, ok := rawFields["provider"]; !ok {
		mc.Provider = cfg.ModelList[idx].Provider
		// Older clients still round-trip the legacy model field only. When the
		// stored config encodes provider/model in Model and has no explicit
		// Provider field yet, continue preserving that hidden provider prefix.
		// This keeps provider-omitted updates backward-compatible even when an
		// older client edits the visible model ID.
		if strings.TrimSpace(cfg.ModelList[idx].Provider) == "" {
			existingRawModel := strings.TrimSpace(cfg.ModelList[idx].Model)
			incomingModel := strings.TrimSpace(mc.Model)
			existingProtocol, existingModelID := providers.ExtractProtocol(cfg.ModelList[idx])
			if existingRawModel != "" && existingRawModel != existingModelID && incomingModel != "" {
				if incomingModel == existingModelID {
					mc.Model = existingRawModel
				} else if strings.Contains(incomingModel, "/") && !strings.Contains(existingModelID, "/") {
					// Older clients never saw the hidden provider prefix for simple
					// legacy entries such as "openai/gpt-4o". If they now send an
					// explicit provider/model string, treat it as the caller's full
					// intent instead of re-applying the old hidden prefix.
					mc.Model = incomingModel
				} else if !strings.HasPrefix(incomingModel, existingProtocol+"/") {
					mc.Model = existingProtocol + "/" + incomingModel
				}
			}
		}
	}

	normalizeIncomingModelConfig(&mc.ModelConfig, defaultChainProvider(cfg))
	if err = validateIncomingModelConfig(&mc.ModelConfig, cfg.ModelList[idx]); err != nil {
		http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
		return
	}
	newModelName := strings.TrimSpace(mc.ModelConfig.ModelName)
	if newModelName != oldModelName && modelNameTaken(cfg, newModelName, idx) {
		http.Error(
			w,
			fmt.Sprintf("model name %q already exists", newModelName),
			http.StatusConflict,
		)
		return
	}
	defaultReferenceRenamed := false
	fallbackReferenceRenamed := false
	if oldModelName != "" && newModelName != "" && oldModelName != newModelName {
		remainingOldNameModels := modelConfigsByNameExceptIndex(cfg, oldModelName, idx)
		if err := validateDefaultChainModelSet(
			oldModelName,
			remainingOldNameModels,
			true,
			defaultChainProvider(cfg),
		); err != nil {
			defaultReferenceRenamed = true
			if strings.TrimSpace(cfg.Agents.Defaults.ModelName) == oldModelName {
				cfg.Agents.Defaults.ModelName = newModelName
			}
		}
		if err := validateDefaultChainModelSet(
			oldModelName,
			remainingOldNameModels,
			false,
			defaultChainProvider(cfg),
		); err != nil {
			fallbackReferenceRenamed = true
			cfg.Agents.Defaults.ModelFallbacks = replaceModelName(
				cfg.Agents.Defaults.ModelFallbacks,
				oldModelName,
				newModelName,
			)
		}
	}
	cfg.ModelList[idx] = &mc.ModelConfig
	normalizeStoredModelProviders(cfg)
	normalizeDefaultChainReferences(cfg)
	var agentReferenceRenames []string
	if len(modelConfigsByName(cfg)[oldModelName]) == 0 {
		agentReferenceRenames = renameAgentModelReferences(cfg, oldModelName, newModelName)
	}

	logger.Debugf("update model config: %#v", mc.ModelConfig)

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	response := map[string]any{"status": "ok"}
	if oldModelName != "" && newModelName != "" && oldModelName != newModelName {
		rename := map[string]any{
			"from":     oldModelName,
			"to":       newModelName,
			"default":  defaultReferenceRenamed,
			"fallback": fallbackReferenceRenamed,
		}
		if len(agentReferenceRenames) > 0 {
			rename["agents"] = agentReferenceRenames
		}
		response["reference_rename"] = rename
	}
	json.NewEncoder(w).Encode(response)
}

// handleDeleteModel removes a model configuration entry at the given index.
//
//	DELETE /api/models/{index}
func (h *Handler) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		http.Error(w, "Invalid index", http.StatusBadRequest)
		return
	}
	h.deleteModel(w, r, func(*config.Config) (int, error) { return idx, nil })
}

// handleDeleteModelByName removes the model resolved by its model_name. Like
// the matching update endpoint, it avoids deleting an unrelated model when
// list indices shifted after the client loaded the model list.
//
//	DELETE /api/models/by-name/{name}
func (h *Handler) handleDeleteModelByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		http.Error(w, "model name is required", http.StatusBadRequest)
		return
	}
	h.deleteModel(w, r, func(cfg *config.Config) (int, error) {
		return resolveModelIndexByName(cfg, name)
	})
}

// handleSetModelEnabled toggles whether a model participates in runtime
// selection. Disabled models stay in model_list and the model list UI, but are
// removed from the default/fallback chain and skipped by resolution.
//
//	PUT /api/models/enabled/{name}
func (h *Handler) handleSetModelEnabled(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		http.Error(w, "model name is required", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	if req.Enabled == nil {
		http.Error(w, "enabled is required", http.StatusBadRequest)
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}
	normalizeStoredModelProviders(cfg)

	idx, err := resolveModelIndexByName(cfg, name)
	if err != nil {
		writeModelResolutionError(w, err)
		return
	}
	if cfg.ModelList[idx].IsVirtual() {
		http.Error(w, "cannot change enabled state of a virtual model", http.StatusBadRequest)
		return
	}

	cfg.ModelList[idx].Disabled = !*req.Enabled
	normalizeDefaultChainReferences(cfg)

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":         "ok",
		"enabled":        *req.Enabled,
		"default_model":  cfg.Agents.Defaults.ModelName,
		"fallback_chain": cfg.Agents.Defaults.ModelFallbacks,
	})
}

func (h *Handler) deleteModel(
	w http.ResponseWriter,
	r *http.Request,
	resolveIndex func(cfg *config.Config) (int, error),
) {
	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}
	normalizeStoredModelNames(cfg)

	idx, err := resolveIndex(cfg)
	if err != nil {
		writeModelResolutionError(w, err)
		return
	}

	if idx < 0 || idx >= len(cfg.ModelList) {
		http.Error(w, fmt.Sprintf("Index %d out of range (0-%d)", idx, len(cfg.ModelList)-1), http.StatusNotFound)
		return
	}

	deletedModelName := strings.TrimSpace(cfg.ModelList[idx].ModelName)

	cfg.ModelList = append(cfg.ModelList[:idx], cfg.ModelList[idx+1:]...)

	if deletedModelName != "" {
		remainingModels := modelConfigsByName(cfg)[deletedModelName]
		if strings.TrimSpace(cfg.Agents.Defaults.ModelName) == deletedModelName {
			if err := validateDefaultChainModelSet(
				deletedModelName,
				remainingModels,
				true,
				defaultChainProvider(cfg),
			); err != nil {
				cfg.Agents.Defaults.ModelName = ""
			}
		}
		if err := validateDefaultChainModelSet(
			deletedModelName,
			remainingModels,
			false,
			defaultChainProvider(cfg),
		); err != nil {
			cfg.Agents.Defaults.ModelFallbacks = removeModelName(
				cfg.Agents.Defaults.ModelFallbacks,
				deletedModelName,
			)
		}
	}
	normalizeDefaultChainReferences(cfg)
	var agentReferenceCleanup []string
	if deletedModelName != "" && len(modelConfigsByName(cfg)[deletedModelName]) == 0 {
		agentReferenceCleanup = removeAgentModelReferences(cfg, deletedModelName)
	}

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	response := map[string]any{"status": "ok"}
	if deletedModelName != "" && len(agentReferenceCleanup) > 0 {
		response["reference_cleanup"] = map[string]any{
			"model":  deletedModelName,
			"agents": agentReferenceCleanup,
		}
	}
	json.NewEncoder(w).Encode(response)
}

func removeModelName(names []string, target string) []string {
	target = strings.TrimSpace(target)
	if target == "" || len(names) == 0 {
		return names
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == target {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
}

// renameAgentModelReferences rewrites model references in every configured
// agent (model primary/fallbacks and subagent model). It returns the IDs of the
// agents that changed so the API caller can warn about the restart.
func renameAgentModelReferences(cfg *config.Config, oldName, newName string) []string {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if cfg == nil || oldName == "" || newName == "" || oldName == newName {
		return nil
	}

	var affected []string
	for i := range cfg.Agents.List {
		agent := &cfg.Agents.List[i]
		changed := renameAgentModelConfig(agent.Model, oldName, newName)
		if agent.Subagents != nil {
			changed = renameAgentModelConfig(agent.Subagents.Model, oldName, newName) || changed
		}
		if changed {
			affected = append(affected, agent.ID)
		}
	}
	return affected
}

// removeAgentModelReferences drops a deleted model from every configured agent.
// A primary reference falls back to the global default; fallback entries are
// removed. It returns the IDs of the agents that changed.
func removeAgentModelReferences(cfg *config.Config, modelName string) []string {
	modelName = strings.TrimSpace(modelName)
	if cfg == nil || modelName == "" {
		return nil
	}

	var affected []string
	for i := range cfg.Agents.List {
		agent := &cfg.Agents.List[i]
		changed := removeAgentModelConfig(agent.Model, modelName)
		if changed && agent.Model != nil &&
			strings.TrimSpace(agent.Model.Primary) == "" && len(agent.Model.Fallbacks) == 0 {
			agent.Model = nil
		}
		if agent.Subagents != nil {
			subChanged := removeAgentModelConfig(agent.Subagents.Model, modelName)
			if subChanged && agent.Subagents.Model != nil &&
				strings.TrimSpace(agent.Subagents.Model.Primary) == "" && len(agent.Subagents.Model.Fallbacks) == 0 {
				agent.Subagents.Model = nil
			}
			changed = subChanged || changed
		}
		if changed {
			affected = append(affected, agent.ID)
		}
	}
	return affected
}

func renameAgentModelConfig(ref *config.AgentModelConfig, oldName, newName string) bool {
	if ref == nil {
		return false
	}
	changed := false
	if strings.TrimSpace(ref.Primary) == oldName {
		ref.Primary = newName
		changed = true
	}
	if renamed, ok := replaceModelNameInList(ref.Fallbacks, oldName, newName); ok {
		ref.Fallbacks = renamed
		changed = true
	}
	return changed
}

func removeAgentModelConfig(ref *config.AgentModelConfig, modelName string) bool {
	if ref == nil {
		return false
	}
	changed := false
	if strings.TrimSpace(ref.Primary) == modelName {
		ref.Primary = ""
		changed = true
	}
	if filtered, ok := removeModelNameFromList(ref.Fallbacks, modelName); ok {
		ref.Fallbacks = filtered
		changed = true
	}
	return changed
}

func replaceModelNameInList(names []string, oldName, newName string) ([]string, bool) {
	changed := false
	for i, name := range names {
		if strings.TrimSpace(name) == oldName {
			names[i] = newName
			changed = true
		}
	}
	return names, changed
}

func removeModelNameFromList(names []string, target string) ([]string, bool) {
	if len(names) == 0 {
		return names, false
	}
	filtered := make([]string, 0, len(names))
	changed := false
	for _, name := range names {
		if strings.TrimSpace(name) == target {
			changed = true
			continue
		}
		filtered = append(filtered, name)
	}
	if !changed {
		return names, false
	}
	return filtered, true
}

func replaceModelName(names []string, oldName string, newName string) []string {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || oldName == newName || len(names) == 0 {
		return names
	}
	replaced := make([]string, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == oldName {
			replaced = append(replaced, newName)
			continue
		}
		replaced = append(replaced, name)
	}
	return replaced
}

// handleSetDefaultModel sets the default model for all agents.
//
//	POST /api/models/default
func (h *Handler) handleSetDefaultModel(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req struct {
		ModelName string `json:"model_name"`
	}
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	modelName := strings.TrimSpace(req.ModelName)
	if modelName == "" {
		http.Error(w, "model_name is required", http.StatusBadRequest)
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}
	normalizeStoredModelNames(cfg)

	// Keep this legacy endpoint alias-only. Raw references are managed by the
	// default-chain endpoint and may already exist in persisted configuration.
	if err := validateDefaultChainModelSet(
		modelName,
		modelConfigsByName(cfg)[modelName],
		true,
		defaultChainProvider(cfg),
	); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found in model_list") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}

	cfg.Agents.Defaults.ModelName = modelName
	cfg.Agents.Defaults.ModelFallbacks = removeModelName(cfg.Agents.Defaults.ModelFallbacks, modelName)
	normalizeDefaultChainReferences(cfg)

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	applied, pending, gatewayUnavailable, warnings := h.applyGatewayAgentModels(
		h.globalModelRequests(cfg, modelName),
	)
	h.markGatewayConfigApplied()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":              "ok",
		"default_model":       modelName,
		"applied":             applied,
		"pending_agents":      pending,
		"gateway_unavailable": gatewayUnavailable,
		"warnings":            warnings,
		"restart_required":    false,
	})
}

// handleUpdateDefaultChain updates the persisted default model and fallback chain.
//
//	PUT /api/models/default-chain
func (h *Handler) handleUpdateDefaultChain(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req defaultChainRequest
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}
	normalizeStoredModelNames(cfg)

	defaultModel := strings.TrimSpace(req.DefaultModel)
	fallbacks, err := validateDefaultModelChain(cfg, defaultModel, req.FallbackChain)
	if err != nil {
		http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
		return
	}

	cfg.Agents.Defaults.ModelName = defaultModel
	cfg.Agents.Defaults.ModelFallbacks = fallbacks

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	applied, pending, gatewayUnavailable, warnings := h.applyGatewayAgentModels(
		h.globalModelRequests(cfg, defaultModel),
	)
	h.markGatewayConfigApplied()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(defaultChainResponse{
		DefaultModel:       defaultModel,
		FallbackChain:      fallbacks,
		Applied:            applied,
		PendingAgents:      pending,
		GatewayUnavailable: gatewayUnavailable,
		Warnings:           warnings,
		RestartRequired:    false,
	})
}

// maskAPIKey returns a masked version of an API key for safe display.
// Keys longer than 12 chars show prefix + last 4 chars: "sk-****abcd".
// Keys 9-12 chars show prefix + last 2 chars: "sk-****cd".
// Shorter keys are fully masked as "****".
// Empty keys return empty string.
// Ensure at least 40% of the key will not be displayed.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}

	if len(key) <= 8 {
		return "****"
	}

	// Show first 3 chars and last 2 chars
	if len(key) <= 12 {
		return key[:3] + "****" + key[len(key)-2:]
	}

	// Show first 3 chars and last 4 chars
	return key[:3] + "****" + key[len(key)-4:]
}

// maskAPIKeyList masks every stored key. Plaintext keys never leave the
// launcher through the models API.
func maskAPIKeyList(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	masked := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		masked = append(masked, maskAPIKey(key))
	}
	return masked
}

// normalizeAPIKeyList trims, drops empty values and removes duplicates while
// preserving first-seen order.
func normalizeAPIKeyList(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(keys))
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	return normalized
}

// resolveUpdatedAPIKeys applies the multi-key update contract:
//   - api_keys_keep present: start from those stored indexes and append api_keys
//   - api_keys present without keep: replace the full list with api_keys
//   - only api_key: replace the list with one key
//   - none: preserve the existing list
func resolveUpdatedAPIKeys(
	legacyKey string,
	newKeys []string,
	keepIndexes []int,
	keepProvided bool,
	newKeysProvided bool,
	existing []string,
) ([]string, error) {
	if keepProvided || newKeysProvided {
		var base []string
		if keepProvided {
			selected, err := selectAPIKeys(existing, keepIndexes)
			if err != nil {
				return nil, err
			}
			base = selected
		}
		combined := append(append([]string(nil), base...), normalizeAPIKeyList(newKeys)...)
		return normalizeAPIKeyList(combined), nil
	}
	if trimmed := strings.TrimSpace(legacyKey); trimmed != "" {
		return []string{trimmed}, nil
	}
	return append([]string(nil), existing...), nil
}

// selectAPIKeys returns the stored keys at the requested indexes.
func selectAPIKeys(keys []string, indexes []int) ([]string, error) {
	selected := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		if idx < 0 || idx >= len(keys) {
			return nil, fmt.Errorf("api key index %d out of range (0-%d)", idx, len(keys)-1)
		}
		selected = append(selected, keys[idx])
	}
	return selected, nil
}

// httpClientForProxy returns an HTTP client that routes requests through the
// configured proxy. An empty proxy keeps the default client.
func httpClientForProxy(rawProxy string) (*http.Client, error) {
	rawProxy = strings.TrimSpace(rawProxy)
	if rawProxy == "" {
		return http.DefaultClient, nil
	}
	proxyURL, err := url.Parse(rawProxy)
	if err != nil || proxyURL.Host == "" {
		return nil, fmt.Errorf("invalid proxy URL %q", rawProxy)
	}
	switch strings.ToLower(proxyURL.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("invalid proxy URL %q: unsupported scheme", rawProxy)
	}
	return &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}, nil
}

// handleFetchModels fetches available models from an upstream provider.
//
//	POST /api/models/fetch
func (h *Handler) handleFetchModels(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req struct {
		Provider   string `json:"provider"`
		APIKey     string `json:"api_key"`
		APIBase    string `json:"api_base"`
		Proxy      string `json:"proxy"`
		ModelIndex *int   `json:"model_index,omitempty"`
	}
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if req.Provider == "" {
		http.Error(w, "provider is required", http.StatusBadRequest)
		return
	}

	if !providers.IsModelProviderFetchable(req.Provider) {
		http.Error(w, fmt.Sprintf("provider %q does not support model listing", req.Provider), http.StatusBadRequest)
		return
	}

	apiKey := strings.TrimSpace(req.APIKey)
	apiBase := strings.TrimSpace(req.APIBase)
	proxy := strings.TrimSpace(req.Proxy)

	if req.ModelIndex != nil {
		if stored := h.lookupStoredModelConfig(*req.ModelIndex, req.Provider, apiBase); stored != nil {
			if apiKey == "" {
				apiKey = stored.APIKey()
			}
			if proxy == "" {
				proxy = strings.TrimSpace(stored.Proxy)
			}
		}
	}

	if apiBase == "" {
		apiBase = providers.DefaultAPIBaseForProtocol(req.Provider)
	}
	if apiBase == "" {
		http.Error(w, fmt.Sprintf("No default API base for provider %q", req.Provider), http.StatusBadRequest)
		return
	}

	client, err := httpClientForProxy(proxy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	models, err := fetchUpstreamModelsWithClient(ctx, req.Provider, apiBase, apiKey, client)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch models: %v", err), http.StatusBadGateway)
		return
	}

	// Auto-save fetched models to catalog
	catalogModels := make([]CatalogModel, len(models))
	for i, m := range models {
		catalogModels[i] = CatalogModel{ID: m.ID, OwnedBy: m.OwnedBy, ContextWindow: m.ContextWindow}
	}
	if saveErr := SaveCatalog(req.Provider, apiBase, apiKey, catalogModels); saveErr != nil {
		// Log but don't fail the request — saving catalog is non-critical
		logger.Warnf("Failed to save model catalog: %v", saveErr)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"models": models,
		"total":  len(models),
	})
}

// findStoredAPIKeyForEndpoint returns the first configured API key that belongs
// to the same provider and effective API base. Catalog imports use it so models
// added without an explicit key stay usable when the user already configured
// credentials for that endpoint. The API base must match (including the
// provider default) so a credential is never attached to a different endpoint.
func findStoredAPIKeyForEndpoint(
	models []*config.ModelConfig,
	provider, apiBase string,
) (apiKey string, modelName string, found bool) {
	provider = providers.NormalizeProvider(provider)
	if provider == "" {
		return "", "", false
	}
	effectiveBase := strings.TrimSpace(apiBase)
	if effectiveBase == "" {
		effectiveBase = providers.DefaultAPIBaseForProtocol(provider)
	}
	effectiveBase = normalizeAPIBaseForCompare(effectiveBase)

	for _, model := range models {
		if model == nil {
			continue
		}
		modelProvider, _ := providers.ExtractProtocol(model)
		if providers.NormalizeProvider(modelProvider) != provider {
			continue
		}
		modelBase := strings.TrimSpace(model.APIBase)
		if modelBase == "" {
			modelBase = providers.DefaultAPIBaseForProtocol(modelProvider)
		}
		if normalizeAPIBaseForCompare(modelBase) != effectiveBase {
			continue
		}
		if key := model.APIKey(); strings.TrimSpace(key) != "" {
			return key, model.ModelName, true
		}
	}
	return "", "", false
}

// lookupStoredModelConfig returns the stored model at index when it targets the
// same provider and effective API base as the request. Callers use it to reuse
// saved credentials and proxy settings for fetch/test.
func (h *Handler) lookupStoredModelConfig(index int, reqProvider, reqAPIBase string) *config.ModelConfig {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil || index < 0 || index >= len(cfg.ModelList) {
		return nil
	}
	stored := cfg.ModelList[index]
	storedProvider, _ := providers.ExtractProtocol(stored)
	if providers.NormalizeProvider(reqProvider) != providers.NormalizeProvider(storedProvider) {
		return nil
	}
	effectiveReqBase := strings.TrimSpace(reqAPIBase)
	if effectiveReqBase == "" {
		effectiveReqBase = providers.DefaultAPIBaseForProtocol(reqProvider)
	}
	effectiveStoredBase := strings.TrimSpace(stored.APIBase)
	if effectiveStoredBase == "" {
		effectiveStoredBase = providers.DefaultAPIBaseForProtocol(storedProvider)
	}
	if normalizeAPIBaseForCompare(effectiveReqBase) != normalizeAPIBaseForCompare(effectiveStoredBase) {
		return nil
	}
	return stored
}

type upstreamModel struct {
	ID            string `json:"id"`
	OwnedBy       string `json:"owned_by,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func fetchUpstreamModelsWithClient(
	ctx context.Context,
	provider, apiBase, apiKey string,
	client *http.Client,
) ([]upstreamModel, error) {
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	provider = providers.NormalizeProvider(provider)

	var fetchURL string
	switch provider {
	case "ollama":
		// Strip /v1 suffix if present to get the Ollama root
		root := apiBase
		if strings.HasSuffix(root, "/v1") {
			root = root[:len(root)-3]
		}
		root = strings.TrimRight(root, "/")
		fetchURL = root + "/api/tags"
		return fetchOllamaModelsWithClient(ctx, fetchURL, client)
	case "nearai":
		fetchURL = apiBase + "/model/list"
		return fetchNearAIModelsWithClient(ctx, fetchURL, apiKey, client)
	default:
		// OpenAI-compatible: /v1/models
		fetchURL = apiBase + "/models"
		return fetchOpenAICompatibleModelsWithClient(ctx, fetchURL, apiKey, client)
	}
}

func fetchNearAIModelsWithClient(
	ctx context.Context,
	fetchURL, apiKey string,
	client *http.Client,
) ([]upstreamModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return nil, err
	}
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nearai returned status %d", resp.StatusCode)
	}

	var parsed struct {
		Models []struct {
			ModelID  string `json:"modelId"`
			OwnedBy  string `json:"ownedBy"`
			Metadata struct {
				OwnedBy string `json:"ownedBy"`
			} `json:"metadata"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	models := make([]upstreamModel, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		id := strings.TrimSpace(m.ModelID)
		if id == "" {
			continue
		}
		ownedBy := strings.TrimSpace(m.OwnedBy)
		if ownedBy == "" {
			ownedBy = strings.TrimSpace(m.Metadata.OwnedBy)
		}
		models = append(models, upstreamModel{ID: id, OwnedBy: ownedBy})
	}
	return models, nil
}

func fetchOpenAICompatibleModels(ctx context.Context, fetchURL, apiKey string) ([]upstreamModel, error) {
	return fetchOpenAICompatibleModelsWithClient(ctx, fetchURL, apiKey, http.DefaultClient)
}

func fetchOpenAICompatibleModelsWithClient(
	ctx context.Context,
	fetchURL, apiKey string,
	client *http.Client,
) ([]upstreamModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return nil, err
	}
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	type modelItem struct {
		ID               string `json:"id"`
		OwnedBy          string `json:"owned_by"`
		ContextLength    int    `json:"context_length"`
		ContextWindow    int    `json:"context_window"`
		MaxModelLen      int    `json:"max_model_len"`
		MaxContextLength int    `json:"max_context_length"`
		Limit            struct {
			Context int `json:"context"`
			Input   int `json:"input"`
			Output  int `json:"output"`
		} `json:"limit"`
	}

	// {"data": [...]} envelope. Distinguish "envelope shape with empty list"
	// from "object without a data key" via Data being non-nil after unmarshal:
	// json.Unmarshal sets Data to []modelItem{} for `{"data":[]}` but leaves
	// it as nil when "data" is absent or null.
	var envelope struct {
		Data []modelItem `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Data != nil {
		models := make([]upstreamModel, 0, len(envelope.Data))
		for _, m := range envelope.Data {
			if m.ID != "" {
				models = append(models, upstreamModel{
					ID:            m.ID,
					OwnedBy:       m.OwnedBy,
					ContextWindow: firstPositiveInt(m.Limit.Context, m.ContextLength, m.ContextWindow, m.MaxModelLen, m.MaxContextLength),
				})
			}
		}
		return models, nil
	}

	// Bare-array shape, including `[]`.
	var arr []modelItem
	if err := json.Unmarshal(body, &arr); err == nil {
		models := make([]upstreamModel, 0, len(arr))
		for _, m := range arr {
			if m.ID != "" {
				models = append(models, upstreamModel{
					ID:            m.ID,
					OwnedBy:       m.OwnedBy,
					ContextWindow: firstPositiveInt(m.Limit.Context, m.ContextLength, m.ContextWindow, m.MaxModelLen, m.MaxContextLength),
				})
			}
		}
		return models, nil
	}

	preview := body
	if len(preview) > 256 {
		preview = preview[:256]
	}
	return nil, fmt.Errorf("decode response: unrecognized shape: %s", strings.TrimSpace(string(preview)))
}

func fetchOllamaModelsWithClient(
	ctx context.Context,
	fetchURL string,
	client *http.Client,
) ([]upstreamModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var parsed struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	models := make([]upstreamModel, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		id := m.Name
		if id == "" {
			id = m.Model
		}
		if id != "" {
			models = append(models, upstreamModel{ID: id})
		}
	}
	return models, nil
}

// normalizeAPIBaseForCompare normalizes an API base URL for equality comparison
// by trimming trailing slashes and lowering the scheme/host.
func normalizeAPIBaseForCompare(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.TrimRight(raw, "/")
	u, err := url.Parse(raw)
	if err != nil {
		return strings.ToLower(raw)
	}
	if u.Host == "" {
		u, err = url.Parse("//" + raw)
		if err != nil {
			return strings.ToLower(raw)
		}
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.Path, "/")
}

// handleTestModel tests connectivity to a model endpoint.
//
//	POST /api/models/{index}/test
func (h *Handler) handleTestModel(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		http.Error(w, "Invalid index", http.StatusBadRequest)
		return
	}

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	if idx < 0 || idx >= len(cfg.ModelList) {
		http.Error(w, fmt.Sprintf("Index %d out of range (0-%d)", idx, len(cfg.ModelList)-1), http.StatusNotFound)
		return
	}

	m := cfg.ModelList[idx]
	if !hasModelConfiguration(m) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success":    false,
			"latency_ms": 0,
			"status":     modelStatusUnconfigured,
			"error":      "API key not configured",
		})
		return
	}

	client, err := httpClientForProxy(m.Proxy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	start := time.Now()
	available := probeModelConnectivityWithClient(m, client)
	latency := time.Since(start).Milliseconds()

	result := map[string]any{
		"success":    available,
		"latency_ms": latency,
	}
	if !available && allowsModelListingFallback(m) && endpointModelsReachable(m, client) {
		// The endpoint answers /models but the configured model id is not listed
		// (some OpenAI-compatible servers expose only deployment aliases). Chat
		// still works, so report success with a warning instead of "unreachable".
		available = true
		result["success"] = true
		result["warning"] = fmt.Sprintf("Endpoint reachable, but model %q is not listed in /models", m.Model)
	}
	if available {
		result["status"] = modelStatusAvailable
	} else {
		result["status"] = modelStatusUnreachable
		result["error"] = "Endpoint unreachable"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleTestInlineModel tests connectivity using inline (unsaved) parameters.
// Unlike handleTestModel which only checks saved config, this endpoint performs
// a real network probe (e.g. GET /models) to verify the endpoint is reachable.
//
//	POST /api/models/test-inline
func (h *Handler) handleTestInlineModel(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var req struct {
		Provider   string `json:"provider"`
		Model      string `json:"model"`
		APIBase    string `json:"api_base"`
		APIKey     string `json:"api_key"`
		Proxy      string `json:"proxy"`
		AuthMethod string `json:"auth_method"`
		ModelIndex *int   `json:"model_index"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	m := &config.ModelConfig{
		Provider:   strings.TrimSpace(req.Provider),
		Model:      strings.TrimSpace(req.Model),
		APIBase:    strings.TrimSpace(req.APIBase),
		AuthMethod: strings.TrimSpace(req.AuthMethod),
		Proxy:      strings.TrimSpace(req.Proxy),
	}
	if req.APIKey != "" {
		m.SetAPIKey(req.APIKey)
	}

	// When api_key is empty and model_index is provided, fall back to stored credentials.
	// This lets the edit form test unsaved field changes while using the saved key.
	// Only reuse the stored key when the provider and effective API base match
	// the saved model, to prevent attaching a credential to a different endpoint.
	if req.APIKey == "" && req.ModelIndex != nil {
		cfg, err := config.LoadConfig(h.configPath)
		if err == nil && *req.ModelIndex >= 0 && *req.ModelIndex < len(cfg.ModelList) {
			stored := cfg.ModelList[*req.ModelIndex]
			storedProvider, _ := providers.ExtractProtocol(stored)
			reqProvider := providers.NormalizeProvider(m.Provider)
			providerMatch := reqProvider == "" || reqProvider == providers.NormalizeProvider(storedProvider)

			effectiveReqBase := strings.TrimSpace(m.APIBase)
			if effectiveReqBase == "" {
				effectiveReqBase = providers.DefaultAPIBaseForProtocol(reqProvider)
			}
			effectiveStoredBase := strings.TrimSpace(stored.APIBase)
			if effectiveStoredBase == "" {
				effectiveStoredBase = providers.DefaultAPIBaseForProtocol(storedProvider)
			}
			baseMatch := normalizeAPIBaseForCompare(effectiveReqBase) == normalizeAPIBaseForCompare(effectiveStoredBase)

			if providerMatch && baseMatch {
				if stored.APIKey() != "" {
					m.SetAPIKey(stored.APIKey())
				}
				if m.APIBase == "" && stored.APIBase != "" {
					m.APIBase = stored.APIBase
				}
				if m.Proxy == "" && stored.Proxy != "" {
					m.Proxy = stored.Proxy
				}
			}
		}
	}

	client, err := httpClientForProxy(m.Proxy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check if configuration exists
	if !hasModelConfiguration(m) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success":    false,
			"latency_ms": 0,
			"status":     modelStatusUnconfigured,
			"error":      "API key not configured",
		})
		return
	}

	// Perform a real network probe
	start := time.Now()
	available := probeModelConnectivityWithClient(m, client)
	latency := time.Since(start).Milliseconds()

	result := map[string]any{
		"success":    available,
		"latency_ms": latency,
	}
	if !available && allowsModelListingFallback(m) && endpointModelsReachable(m, client) {
		available = true
		result["success"] = true
		result["warning"] = fmt.Sprintf("Endpoint reachable, but model %q is not listed in /models", m.Model)
	}
	if available {
		result["status"] = modelStatusAvailable
	} else {
		result["status"] = modelStatusUnreachable
		result["error"] = "Endpoint unreachable"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// allowsModelListingFallback reports whether an HTTP 200 from GET /models is
// enough to treat the endpoint as reachable even when the configured model id is
// missing from the listing. Providers with strict model catalogs (Anthropic,
// Gemini, Ollama) must still fail in that case.
func allowsModelListingFallback(m *config.ModelConfig) bool {
	switch modelProtocol(m) {
	case "anthropic", "anthropic-messages", "alibaba-coding-anthropic", "gemini", "ollama":
		return false
	default:
		return true
	}
}

// endpointModelsReachable reports whether GET <base>/models returns 200, even if
// the configured model id is not present in the listing.
func endpointModelsReachable(m *config.ModelConfig, client *http.Client) bool {
	apiBase := strings.TrimRight(modelProbeAPIBase(m), "/")
	if apiBase == "" {
		return false
	}
	if client == nil {
		client = &http.Client{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/models", nil)
	if err != nil {
		return false
	}
	if key := strings.TrimSpace(m.APIKey()); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// probeModelConnectivity performs a real network probe to verify model endpoint reachability.
func probeModelConnectivity(m *config.ModelConfig) bool {
	return probeModelConnectivityWithClient(m, &http.Client{})
}

// probeModelConnectivityWithClient probes a model through the given HTTP client
// so callers can honor the configured proxy.
func probeModelConnectivityWithClient(m *config.ModelConfig, client *http.Client) bool {
	apiBase := modelProbeAPIBase(m)
	protocol, modelID := splitModel(m)

	switch protocol {
	case "ollama":
		return probeOllamaModelWithClient(apiBase, modelID, client)
	case "gemini":
		return probeGeminiModelWithClient(apiBase, modelID, m.APIKey(), client)
	case "anthropic", "anthropic-messages", "alibaba-coding-anthropic":
		return probeAnthropicModelWithClient(apiBase, modelID, m.APIKey(), client)
	case "vllm", "lmstudio", "gpt4free":
		return probeOpenAICompatibleModelWithClient(apiBase, modelID, m.APIKey(), client)
	case "github-copilot":
		return probeTCPService(apiBase)
	case "claude-cli":
		return probeCommandAvailable("claude")
	case "codex-cli":
		return probeCommandAvailable("codex")
	default:
		// For remote providers (OpenAI, Anthropic, Gemini, DeepSeek, etc.),
		// make a real GET /models request to verify connectivity and credentials.
		if apiBase != "" {
			return probeOpenAICompatibleModelWithClient(apiBase, modelID, m.APIKey(), client)
		}
		return false
	}
}
