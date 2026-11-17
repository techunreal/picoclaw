// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"sort"
	"strings"
)

// ListActiveTurns returns a snapshot of root turns that are currently running.
// Child subturns are excluded; use ListSubagentRuns for those.
func (al *AgentLoop) ListActiveTurns() []ActiveTurnInfo {
	if al == nil {
		return nil
	}
	turns := make([]ActiveTurnInfo, 0)
	al.activeTurnStates.Range(func(_, value any) bool {
		ts, ok := value.(*turnState)
		if !ok || ts == nil {
			return true
		}
		snapshot := ts.snapshot()
		if snapshot.Depth != 0 || strings.TrimSpace(snapshot.ChatID) == "" {
			return true
		}
		turns = append(turns, snapshot)
		return true
	})
	sort.Slice(turns, func(i, j int) bool {
		return turns[i].StartedAt.Before(turns[j].StartedAt)
	})
	return turns
}
