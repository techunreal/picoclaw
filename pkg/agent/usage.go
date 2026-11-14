package agent

import (
	"sort"
	"sync"
	"time"
)

const maxUsageSessions = 500

// UsageTotals aggregates token usage for a model or session.
type UsageTotals struct {
	InputTokens  int   `json:"input_tokens"`
	OutputTokens int   `json:"output_tokens"`
	TotalTokens  int   `json:"total_tokens"`
	LLMCalls     int   `json:"llm_calls"`
	LastUsedMS   int64 `json:"last_used_ms,omitempty"`
}

// UsageModelTotals is the usage of one model alias.
type UsageModelTotals struct {
	Model string `json:"model"`
	UsageTotals
}

// UsageSessionTotals is the usage of one session, including per-model buckets.
type UsageSessionTotals struct {
	SessionKey  string `json:"session_key"`
	AgentID     string `json:"agent_id,omitempty"`
	Channel     string `json:"channel,omitempty"`
	ChatID      string `json:"chat_id,omitempty"`
	UsageTotals `json:"totals"`
	Models      []UsageModelTotals `json:"models,omitempty"`
}

// UsageSnapshot is an immutable copy of the runtime usage ledger.
type UsageSnapshot struct {
	Totals      UsageTotals          `json:"totals"`
	Models      []UsageModelTotals   `json:"models"`
	Sessions    []UsageSessionTotals `json:"sessions"`
	StartedAtMS int64                `json:"started_at_ms"`
}

type usageSession struct {
	agentID string
	channel string
	chatID  string
	totals  UsageTotals
	models  map[string]*UsageTotals
}

type usageStore struct {
	mu      sync.RWMutex
	started time.Time
	totals  UsageTotals
	models  map[string]*UsageTotals
	session map[string]*usageSession
}

func newUsageStore() *usageStore {
	return &usageStore{
		started: time.Now(),
		models:  make(map[string]*UsageTotals),
		session: make(map[string]*usageSession),
	}
}

// Record adds one LLM call to the global, per-model, and per-session ledgers.
func (s *usageStore) Record(
	agentID, sessionKey, channel, chatID, model string,
	inputTokens, outputTokens, totalTokens int,
) {
	if s == nil {
		return
	}
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	if totalTokens <= 0 {
		totalTokens = inputTokens + outputTokens
	}
	if totalTokens <= 0 {
		return
	}
	if model == "" {
		model = "unknown"
	}
	if sessionKey == "" {
		sessionKey = "unknown"
	}

	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	addUsage(&s.totals, inputTokens, outputTokens, totalTokens, now)
	modelTotals := s.models[model]
	if modelTotals == nil {
		modelTotals = &UsageTotals{}
		s.models[model] = modelTotals
	}
	addUsage(modelTotals, inputTokens, outputTokens, totalTokens, now)

	session := s.session[sessionKey]
	if session == nil {
		session = &usageSession{
			agentID: agentID,
			channel: channel,
			chatID:  chatID,
			models:  make(map[string]*UsageTotals),
		}
		s.session[sessionKey] = session
	} else {
		if session.agentID == "" {
			session.agentID = agentID
		}
		if session.channel == "" {
			session.channel = channel
		}
		if session.chatID == "" {
			session.chatID = chatID
		}
	}
	addUsage(&session.totals, inputTokens, outputTokens, totalTokens, now)
	sessionModel := session.models[model]
	if sessionModel == nil {
		sessionModel = &UsageTotals{}
		session.models[model] = sessionModel
	}
	addUsage(sessionModel, inputTokens, outputTokens, totalTokens, now)

	s.pruneSessionsLocked()
}

func addUsage(totals *UsageTotals, input, output, total int, now time.Time) {
	totals.InputTokens += input
	totals.OutputTokens += output
	totals.TotalTokens += total
	totals.LLMCalls++
	totals.LastUsedMS = now.UnixMilli()
}

func (s *usageStore) pruneSessionsLocked() {
	if len(s.session) <= maxUsageSessions {
		return
	}
	type entry struct {
		key    string
		lastMS int64
	}
	entries := make([]entry, 0, len(s.session))
	for key, session := range s.session {
		entries = append(entries, entry{key: key, lastMS: session.totals.LastUsedMS})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].lastMS < entries[j].lastMS })
	for i := 0; i < len(entries)-maxUsageSessions; i++ {
		delete(s.session, entries[i].key)
	}
}

func (s *usageStore) Snapshot() UsageSnapshot {
	if s == nil {
		return UsageSnapshot{Models: []UsageModelTotals{}, Sessions: []UsageSessionTotals{}}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := UsageSnapshot{
		Totals:      s.totals,
		Models:      make([]UsageModelTotals, 0, len(s.models)),
		Sessions:    make([]UsageSessionTotals, 0, len(s.session)),
		StartedAtMS: s.started.UnixMilli(),
	}
	for model, totals := range s.models {
		snapshot.Models = append(snapshot.Models, UsageModelTotals{Model: model, UsageTotals: *totals})
	}
	sort.Slice(snapshot.Models, func(i, j int) bool {
		if snapshot.Models[i].TotalTokens != snapshot.Models[j].TotalTokens {
			return snapshot.Models[i].TotalTokens > snapshot.Models[j].TotalTokens
		}
		return snapshot.Models[i].Model < snapshot.Models[j].Model
	})

	for key, session := range s.session {
		item := UsageSessionTotals{
			SessionKey:  key,
			AgentID:     session.agentID,
			Channel:     session.channel,
			ChatID:      session.chatID,
			UsageTotals: session.totals,
			Models:      make([]UsageModelTotals, 0, len(session.models)),
		}
		for model, totals := range session.models {
			item.Models = append(item.Models, UsageModelTotals{Model: model, UsageTotals: *totals})
		}
		sort.Slice(item.Models, func(i, j int) bool {
			if item.Models[i].TotalTokens != item.Models[j].TotalTokens {
				return item.Models[i].TotalTokens > item.Models[j].TotalTokens
			}
			return item.Models[i].Model < item.Models[j].Model
		})
		snapshot.Sessions = append(snapshot.Sessions, item)
	}
	sort.Slice(snapshot.Sessions, func(i, j int) bool {
		if snapshot.Sessions[i].LastUsedMS != snapshot.Sessions[j].LastUsedMS {
			return snapshot.Sessions[i].LastUsedMS > snapshot.Sessions[j].LastUsedMS
		}
		return snapshot.Sessions[i].SessionKey > snapshot.Sessions[j].SessionKey
	})
	return snapshot
}

// UsageSnapshot returns the current runtime usage ledger.
func (al *AgentLoop) UsageSnapshot() UsageSnapshot {
	if al == nil || al.usage == nil {
		return UsageSnapshot{Models: []UsageModelTotals{}, Sessions: []UsageSessionTotals{}}
	}
	return al.usage.Snapshot()
}
