package agent

import (
	"testing"
	"time"
)

func TestListActiveTurnsReturnsOnlyRootTurns(t *testing.T) {
	al := &AgentLoop{}
	now := time.Now()
	al.activeTurnStates.Store("sk-root", &turnState{
		turnID:     "root-turn",
		agentID:    "main",
		sessionKey: "sk-root",
		chatID:     "pico:session-root",
		phase:      TurnPhaseRunning,
		depth:      0,
		startedAt:  now,
	})
	al.activeTurnStates.Store("child-turn", &turnState{
		turnID:     "child-turn",
		agentID:    "deneme",
		sessionKey: "sk-child",
		chatID:     "pico:session-child",
		phase:      TurnPhaseRunning,
		depth:      1,
		startedAt:  now,
	})
	al.activeTurnStates.Store("pending", &turnState{
		turnID: "pending",
		phase:  TurnPhaseSetup,
	})

	turns := al.ListActiveTurns()
	if len(turns) != 1 {
		t.Fatalf("ListActiveTurns() = %d turns, want 1: %#v", len(turns), turns)
	}
	if turns[0].TurnID != "root-turn" || turns[0].ChatID != "pico:session-root" {
		t.Fatalf("unexpected turn: %#v", turns[0])
	}
}

func TestListActiveTurnsSortsByStartTime(t *testing.T) {
	al := &AgentLoop{}
	base := time.Now()
	al.activeTurnStates.Store("sk-2", &turnState{
		turnID: "second", chatID: "pico:two", startedAt: base.Add(time.Second),
	})
	al.activeTurnStates.Store("sk-1", &turnState{
		turnID: "first", chatID: "pico:one", startedAt: base,
	})

	turns := al.ListActiveTurns()
	if len(turns) != 2 || turns[0].TurnID != "first" || turns[1].TurnID != "second" {
		t.Fatalf("unexpected order: %#v", turns)
	}
}
