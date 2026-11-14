package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	runtimeevents "github.com/sipeed/picoclaw/pkg/events"
	"github.com/sipeed/picoclaw/pkg/tools"
)

func newMonitorTestTurn(id string, depth int) *turnState {
	return &turnState{
		turnID:       id,
		agentID:      "main",
		sessionKey:   id,
		parentTurnID: "main-turn-1",
		userMessage:  "You are a subagent. Complete the task.\n\nTask: analyze the logs",
		depth:        depth,
		startedAt:    time.Now().Add(-1500 * time.Millisecond),
		ctx:          context.Background(),
	}
}

func TestSubagentMonitorTracksLifecycle(t *testing.T) {
	mon := newSubagentMonitor()
	ts := newMonitorTestTurn("subturn-1", 1)

	mon.recordStart(ts)

	runs := mon.list()
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	if runs[0].Status != "running" {
		t.Fatalf("status = %q, want running", runs[0].Status)
	}
	if runs[0].Task != "analyze the logs" {
		t.Fatalf("task = %q, want the prompt task text", runs[0].Task)
	}
	if runs[0].Depth != 1 || runs[0].ParentTurnID != "main-turn-1" {
		t.Fatalf("unexpected run metadata: %#v", runs[0])
	}

	ts.setFinalContent("found three errors")
	mon.recordEnd(ts, "completed")

	runs = mon.list()
	if runs[0].Status != "completed" {
		t.Fatalf("status = %q, want completed", runs[0].Status)
	}
	if runs[0].ResultPreview != "found three errors" {
		t.Fatalf("result = %q, want final content", runs[0].ResultPreview)
	}
	if runs[0].EndedAtMS == 0 || runs[0].DurationMS < 1000 {
		t.Fatalf("duration was not recorded: %#v", runs[0])
	}
}

func TestSubagentMonitorMapsAbortToCanceled(t *testing.T) {
	mon := newSubagentMonitor()
	ts := newMonitorTestTurn("subturn-2", 1)
	mon.recordStart(ts)

	ts.requestHardAbort()
	mon.recordEnd(ts, "error")

	runs := mon.list()
	if len(runs) != 1 || runs[0].Status != "canceled" {
		t.Fatalf("status = %q, want canceled", runs[0].Status)
	}
}

func TestSubagentMonitorPrunesFinishedRuns(t *testing.T) {
	mon := newSubagentMonitor()

	for i := 0; i < maxSubagentRunHistory+30; i++ {
		ts := newMonitorTestTurn(fmt.Sprintf("subturn-%d", i), 1)
		mon.recordStart(ts)
		mon.recordEnd(ts, "completed")
	}

	runs := mon.list()
	if len(runs) != maxSubagentRunHistory {
		t.Fatalf("runs = %d, want %d", len(runs), maxSubagentRunHistory)
	}
	if runs[0].ID != "subturn-129" {
		t.Fatalf("newest run = %q, want subturn-129", runs[0].ID)
	}
	oldest := runs[len(runs)-1].ID
	if oldest != "subturn-30" {
		t.Fatalf("oldest retained run = %q, want subturn-30", oldest)
	}
}

func TestSubagentMonitorCancelWinsOverGracefulEnd(t *testing.T) {
	mon := newSubagentMonitor()
	ts := newMonitorTestTurn("subturn-cancel-payload", 1)
	mon.recordStart(ts)

	mon.markCanceled(ts.turnID)
	runs := mon.list()
	if len(runs) != 1 || runs[0].Status != "canceled" {
		t.Fatalf("status = %q, want canceled", runs[0].Status)
	}

	mon.recordEndPayload(SubTurnEndPayload{
		AgentID:       ts.agentID,
		Status:        "completed",
		ChildTurnID:   ts.turnID,
		ParentTurnID:  ts.parentTurnID,
		ResultPreview: "late result",
	})
	runs = mon.list()
	if runs[0].Status != "canceled" {
		t.Fatalf("status = %q, want canceled to win over a graceful end", runs[0].Status)
	}
	if runs[0].ResultPreview != "late result" {
		t.Fatalf("result preview = %q, want late result", runs[0].ResultPreview)
	}
}

func TestAgentLoopTracksSubTurnEvents(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	ts := newMonitorTestTurn("subturn-events", 1)
	ts.agent = agent
	al.activeTurnStates.Store(ts.turnID, ts)
	defer al.activeTurnStates.Delete(ts.turnID)

	al.emitEvent(runtimeevents.KindAgentSubTurnSpawn,
		ts.eventMeta("test", "subturn.spawn"),
		SubTurnSpawnPayload{AgentID: ts.agentID, Label: ts.turnID, ParentTurnID: ts.parentTurnID},
	)

	runs := al.ListSubagentRuns()
	if len(runs) != 1 || runs[0].Status != "running" {
		t.Fatalf("runs = %#v, want one running run", runs)
	}

	ts.setFinalContent("child result")
	al.emitEvent(runtimeevents.KindAgentSubTurnEnd,
		ts.eventMeta("test", "subturn.end"),
		SubTurnEndPayload{AgentID: ts.agentID, Status: "completed"},
	)

	runs = al.ListSubagentRuns()
	if len(runs) != 1 || runs[0].Status != "completed" || runs[0].ResultPreview != "child result" {
		t.Fatalf("runs = %#v, want completed with result", runs)
	}
}

func TestAgentLoopTracksRealSpawnSubTurn(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	parentAgent := al.registry.GetDefaultAgent()
	if parentAgent == nil {
		t.Fatal("expected default agent")
	}

	parent := &turnState{
		turnID:         "main-turn-real",
		agentID:        "main",
		agent:          parentAgent,
		depth:          0,
		ctx:            context.Background(),
		pendingResults: make(chan *tools.ToolResult, 4),
		concurrencySem: make(chan struct{}, testMaxConcurrentSubTurns),
		session:        &ephemeralSessionStore{},
		al:             al,
	}
	ctx := withTurnState(context.Background(), parent)

	spawner := NewSubTurnSpawner(al)
	if _, err := spawner.SpawnSubTurn(ctx, tools.SubTurnConfig{
		Model:        "test-model",
		SystemPrompt: "monitor me",
		Async:        false,
	}); err != nil {
		t.Fatalf("SpawnSubTurn() error = %v", err)
	}

	runs := al.ListSubagentRuns()
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	if runs[0].Status != "completed" {
		t.Fatalf("status = %q, want completed", runs[0].Status)
	}
	if runs[0].ParentTurnID != "main-turn-real" || runs[0].Depth != 1 {
		t.Fatalf("unexpected run metadata: %#v", runs[0])
	}
}

func TestAgentLoopCancelSubTurn(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	canceled := false
	child := newMonitorTestTurn("subturn-cancel", 1)
	child.cancelFunc = func() { canceled = true }
	al.activeTurnStates.Store(child.turnID, child)
	defer al.activeTurnStates.Delete(child.turnID)

	if !al.CancelSubTurn(child.turnID) {
		t.Fatal("CancelSubTurn() = false, want true for an active child turn")
	}
	if !canceled {
		t.Fatal("child cancel function was not called")
	}
	if !child.hardAbortRequested() {
		t.Fatal("child should be marked as hard-aborted")
	}
	if al.CancelSubTurn(child.turnID) {
		t.Fatal("second CancelSubTurn() = true, want false after finish")
	}
	if al.CancelSubTurn("missing") {
		t.Fatal("CancelSubTurn() = true for an unknown ID")
	}

	root := newMonitorTestTurn("main-turn-root", 0)
	al.activeTurnStates.Store(root.turnID, root)
	defer al.activeTurnStates.Delete(root.turnID)
	if al.CancelSubTurn(root.turnID) {
		t.Fatal("CancelSubTurn() should not cancel a root turn")
	}
}
