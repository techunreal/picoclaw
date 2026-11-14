package agent

import (
	"strings"
	"sync"
	"time"

	runtimeevents "github.com/sipeed/picoclaw/pkg/events"
)

const (
	maxSubagentRunHistory    = 100
	maxSubagentTaskRunes     = 300
	maxSubagentResultRunes   = 500
	subagentTaskPromptMarker = "\nTask: "
)

// SubagentRun is a read-only snapshot of a child turn for the WebUI monitor.
type SubagentRun struct {
	ID            string `json:"id"`
	ParentTurnID  string `json:"parent_turn_id,omitempty"`
	AgentID       string `json:"agent_id"`
	Task          string `json:"task"`
	Status        string `json:"status"`
	Depth         int    `json:"depth"`
	StartedAtMS   int64  `json:"started_at_ms"`
	EndedAtMS     int64  `json:"ended_at_ms,omitempty"`
	DurationMS    int64  `json:"duration_ms"`
	ResultPreview string `json:"result_preview,omitempty"`
}

// subagentMonitor keeps a bounded in-memory history of child turns. Running
// rows are never pruned; finished rows are kept until the history cap is hit.
type subagentMonitor struct {
	mu    sync.RWMutex
	runs  map[string]*SubagentRun
	order []string
}

func newSubagentMonitor() *subagentMonitor {
	return &subagentMonitor{runs: make(map[string]*SubagentRun)}
}

func (m *subagentMonitor) recordStart(ts *turnState) {
	if m == nil || ts == nil {
		return
	}
	snap := ts.snapshot()
	id := subagentRunID(snap)

	m.mu.Lock()
	defer m.mu.Unlock()

	run, ok := m.runs[id]
	if !ok {
		run = &SubagentRun{ID: id}
		m.runs[id] = run
		m.order = append(m.order, id)
	}
	run.ParentTurnID = snap.ParentTurnID
	run.AgentID = snap.AgentID
	run.Task = previewText(subagentTaskText(snap.UserMessage), maxSubagentTaskRunes)
	run.Status = "running"
	run.Depth = snap.Depth
	run.StartedAtMS = snap.StartedAt.UnixMilli()
	run.EndedAtMS = 0
	run.DurationMS = 0
	run.ResultPreview = ""

	m.pruneLocked()
}

func (m *subagentMonitor) recordEnd(ts *turnState, reportedStatus string) {
	if m == nil || ts == nil {
		return
	}
	snap := ts.snapshot()
	id := subagentRunID(snap)

	m.mu.Lock()
	defer m.mu.Unlock()

	run, ok := m.runs[id]
	if !ok {
		run = &SubagentRun{
			ID:           id,
			ParentTurnID: snap.ParentTurnID,
			AgentID:      snap.AgentID,
			Task:         previewText(subagentTaskText(snap.UserMessage), maxSubagentTaskRunes),
			Depth:        snap.Depth,
			StartedAtMS:  snap.StartedAt.UnixMilli(),
		}
		m.runs[id] = run
		m.order = append(m.order, id)
	}

	status := normalizeSubagentStatus(reportedStatus, ts)
	now := time.Now()
	duration := now.Sub(snap.StartedAt)
	if duration < 0 {
		duration = 0
	}

	run.Status = status
	run.EndedAtMS = now.UnixMilli()
	run.DurationMS = duration.Milliseconds()
	run.ResultPreview = previewText(ts.finalContentSnapshot(), maxSubagentResultRunes)

	m.pruneLocked()
}

// markCanceled reflects a cancel request immediately, before the child turn
// unwinds and reports its final status.
func (m *subagentMonitor) markCanceled(id string) {
	if m == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	run, ok := m.runs[id]
	if !ok || run.Status != "running" {
		return
	}
	now := time.Now()
	run.Status = "canceled"
	run.EndedAtMS = now.UnixMilli()
	if run.StartedAtMS > 0 {
		run.DurationMS = now.UnixMilli() - run.StartedAtMS
	}
}

// recordEndPayload finishes a run from the self-contained subturn end event.
func (m *subagentMonitor) recordEndPayload(payload SubTurnEndPayload) {
	if m == nil {
		return
	}
	id := strings.TrimSpace(payload.ChildTurnID)
	if id == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	run, ok := m.runs[id]
	if !ok {
		run = &SubagentRun{
			ID:           id,
			ParentTurnID: payload.ParentTurnID,
			AgentID:      payload.AgentID,
			Depth:        1,
			StartedAtMS:  now.UnixMilli(),
		}
		m.runs[id] = run
		m.order = append(m.order, id)
	}

	nextStatus := normalizeSubagentPayloadStatus(payload.Status)
	if run.Status != "canceled" {
		// A local cancel wins over a graceful turn result.
		run.Status = nextStatus
	}
	run.EndedAtMS = now.UnixMilli()
	run.DurationMS = payload.DurationMS
	if run.DurationMS <= 0 && run.StartedAtMS > 0 {
		run.DurationMS = now.UnixMilli() - run.StartedAtMS
	}
	run.ResultPreview = previewText(payload.ResultPreview, maxSubagentResultRunes)

	m.pruneLocked()
}

func (m *subagentMonitor) list() []SubagentRun {
	if m == nil {
		return []SubagentRun{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]SubagentRun, 0, len(m.order))
	// Newest first: the order slice stores insertion order.
	for i := len(m.order) - 1; i >= 0; i-- {
		if run, ok := m.runs[m.order[i]]; ok {
			out = append(out, *run)
		}
	}
	return out
}

func (m *subagentMonitor) pruneLocked() {
	finished := 0
	for _, run := range m.runs {
		if run.Status != "running" {
			finished++
		}
	}
	if finished <= maxSubagentRunHistory {
		return
	}

	toRemove := finished - maxSubagentRunHistory
	kept := make([]string, 0, len(m.order))
	for _, id := range m.order {
		run, ok := m.runs[id]
		if !ok {
			continue
		}
		if toRemove > 0 && run.Status != "running" {
			delete(m.runs, id)
			toRemove--
			continue
		}
		kept = append(kept, id)
	}
	m.order = kept
}

// subagentRunID prefers the child session key (subturn-N) because it is the
// key used in activeTurnStates and therefore the ID accepted by cancel.
func subagentRunID(snap ActiveTurnInfo) string {
	if strings.TrimSpace(snap.SessionKey) != "" {
		return snap.SessionKey
	}
	return snap.TurnID
}

func normalizeSubagentStatus(reported string, ts *turnState) string {
	if ts != nil && (ts.hardAbortRequested() || (ts.ctx != nil && ts.ctx.Err() != nil)) {
		return "canceled"
	}
	switch strings.TrimSpace(reported) {
	case "completed":
		return "completed"
	case "error", "failed":
		return "failed"
	case "":
		return "completed"
	default:
		return strings.TrimSpace(reported)
	}
}

func normalizeSubagentPayloadStatus(reported string) string {
	switch strings.TrimSpace(reported) {
	case "completed":
		return "completed"
	case "error", "failed":
		return "failed"
	case "canceled", "aborted":
		return "canceled"
	case "":
		return "completed"
	default:
		return strings.TrimSpace(reported)
	}
}

// subagentTaskText strips the boilerplate prompt wrapper from a child turn so
// the monitor shows the actual task.
func subagentTaskText(userMessage string) string {
	msg := strings.TrimSpace(userMessage)
	if idx := strings.LastIndex(msg, subagentTaskPromptMarker); idx >= 0 {
		msg = msg[idx+len(subagentTaskPromptMarker):]
	}
	return strings.TrimSpace(msg)
}

func previewText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

// trackSubagentEvent feeds the monitor from subturn lifecycle events. It runs
// synchronously in the emitting goroutine, so it never misses events even when
// the runtime event bus is lossy.
func (al *AgentLoop) trackSubagentEvent(kind runtimeevents.Kind, meta HookMeta, payload any) {
	if al == nil || al.subagentRuns == nil {
		return
	}
	if kind != runtimeevents.KindAgentSubTurnSpawn && kind != runtimeevents.KindAgentSubTurnEnd {
		return
	}

	// The child turn is removed from activeTurnStates before the subturn end
	// event fires, so prefer the self-contained end payload when present.
	if kind == runtimeevents.KindAgentSubTurnEnd {
		if endPayload, ok := payload.(SubTurnEndPayload); ok && strings.TrimSpace(endPayload.ChildTurnID) != "" {
			al.subagentRuns.recordEndPayload(endPayload)
			return
		}
	}

	val, ok := al.activeTurnStates.Load(meta.SessionKey)
	if !ok {
		val, ok = al.activeTurnStates.Load(meta.TurnID)
	}
	if !ok {
		return
	}
	ts, ok := val.(*turnState)
	if !ok {
		return
	}

	if kind == runtimeevents.KindAgentSubTurnSpawn {
		al.subagentRuns.recordStart(ts)
		return
	}

	reported := ""
	if endPayload, ok := payload.(SubTurnEndPayload); ok {
		reported = endPayload.Status
	}
	al.subagentRuns.recordEnd(ts, reported)
}

// ListSubagentRuns returns a snapshot of tracked child turns, newest first.
func (al *AgentLoop) ListSubagentRuns() []SubagentRun {
	if al == nil || al.subagentRuns == nil {
		return []SubagentRun{}
	}
	return al.subagentRuns.list()
}

// CancelSubTurn hard-aborts a running child turn. It returns false when the ID
// is unknown, is not a child turn, or has already finished.
func (al *AgentLoop) CancelSubTurn(id string) bool {
	if al == nil {
		return false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}

	val, ok := al.activeTurnStates.Load(id)
	if !ok {
		return false
	}
	ts, ok := val.(*turnState)
	if !ok || ts.depth == 0 || ts.isFinished.Load() {
		return false
	}

	ts.requestHardAbort()
	ts.Finish(true)
	al.subagentRuns.markCanceled(id)
	return true
}
