// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"fmt"

	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tokenizer"
)

// contextBudgetMinTurns is the number of most recent turns that context
// trimming must always keep. A turn starts at a user message.
const contextBudgetMinTurns = 2

// contextBudgetError reports that the prompt still does not fit the model
// context window after compaction and trimming down to the retention floor.
// The turn is failed instead of being sent with an empty history.
type contextBudgetError struct {
	ContextWindow int
	MaxTokens     int
	PromptTokens  int
}

func newContextBudgetError(contextWindow, maxTokens, promptTokens int) *contextBudgetError {
	return &contextBudgetError{
		ContextWindow: contextWindow,
		MaxTokens:     maxTokens,
		PromptTokens:  promptTokens,
	}
}

func (e *contextBudgetError) Error() string {
	return fmt.Sprintf(
		"Context window exceeded: the prompt needs about %d tokens but the model window is %d "+
			"(max_tokens reserves %d). The summary and the last %d turns still do not fit. "+
			"Start a new chat, raise this model's context window, or lower its max_tokens override.",
		e.PromptTokens,
		e.ContextWindow,
		e.MaxTokens,
		contextBudgetMinTurns,
	)
}

// parseTurnBoundaries returns the starting index of each Turn in the history.
// A Turn is a complete "user input → LLM iterations → final response" cycle
// (as defined in #1316). Each Turn begins at a user message and extends
// through all subsequent assistant/tool messages until the next user message.
//
// Cutting at a Turn boundary guarantees that no tool-call sequence
// (assistant+ToolCalls → tool results) is split across the cut.
func parseTurnBoundaries(history []providers.Message) []int {
	var starts []int
	for i, msg := range history {
		if msg.Role == "user" {
			starts = append(starts, i)
		}
	}
	return starts
}

// isSafeBoundary reports whether index is a valid Turn boundary — i.e.,
// a position where the kept portion (history[index:]) begins at a user
// message, so no tool-call sequence is torn apart.
func isSafeBoundary(history []providers.Message, index int) bool {
	if index <= 0 || index >= len(history) {
		return true
	}
	return history[index].Role == "user"
}

// findSafeBoundary locates the nearest Turn boundary to targetIndex.
// It prefers the boundary at or before targetIndex (preserving more recent
// context). Falls back to the nearest boundary after targetIndex, and
// returns targetIndex unchanged only when no Turn boundary exists at all.
func findSafeBoundary(history []providers.Message, targetIndex int) int {
	if len(history) == 0 {
		return 0
	}
	if targetIndex <= 0 {
		return 0
	}
	if targetIndex >= len(history) {
		return len(history)
	}

	turns := parseTurnBoundaries(history)
	if len(turns) == 0 {
		return targetIndex
	}

	// Find the last Turn boundary at or before targetIndex.
	// Prefer backward: keeps more recent messages.
	backward := -1
	for _, t := range turns {
		if t <= targetIndex {
			backward = t
		}
	}
	if backward > 0 {
		return backward
	}

	// No valid Turn boundary before target (or only at index 0 which
	// would keep everything). Use the first Turn after targetIndex.
	for _, t := range turns {
		if t > targetIndex {
			return t
		}
	}

	// No Turn boundary after targetIndex either. The only boundary is at
	// index 0, meaning the entire history is a single Turn. Return 0 to
	// signal that safe compression is not possible — callers check for
	// mid <= 0 and skip compression in that case.
	return 0
}

// EstimateMessageTokens estimates the token count for a single message.
// Delegates to the shared tokenizer package for consistency across agent and seahorse.
func EstimateMessageTokens(msg providers.Message) int {
	return tokenizer.EstimateMessageTokens(msg)
}

// EstimateToolDefsTokens estimates the total token cost of tool definitions
// as they appear in the LLM request. Delegates to the shared tokenizer package.
func EstimateToolDefsTokens(defs []providers.ToolDefinition) int {
	return tokenizer.EstimateToolDefsTokens(defs)
}

// estimatePromptTokens returns the estimated prompt size in tokens, including
// tool definitions but excluding the output reservation.
func estimatePromptTokens(messages []providers.Message, toolDefs []providers.ToolDefinition) int {
	msgTokens := 0
	for _, m := range messages {
		msgTokens += EstimateMessageTokens(m)
	}
	return msgTokens + EstimateToolDefsTokens(toolDefs)
}

// isOverContextBudget checks whether the assembled messages plus tool definitions
// and output reserve would exceed the model's context window. This enables
// proactive compression before calling the LLM, rather than reacting to 400 errors.
func isOverContextBudget(
	contextWindow int,
	messages []providers.Message,
	toolDefs []providers.ToolDefinition,
	maxTokens int,
) bool {
	total := estimatePromptTokens(messages, toolDefs) + maxTokens

	return total > contextWindow
}

// trimHistoryToFitContextWindow rebuilds the prompt from progressively newer
// history slices until it fits within the context window. Oldest complete turns
// are dropped first so tool-call sequences remain intact.
//
// minTurns is the number of most recent turns that must be kept. A turn starts
// at a user message, so trimming never leaves the model without the current
// task: the summary and the last minTurns turns survive even when the prompt
// still does not fit (fit reports false in that case).
func trimHistoryToFitContextWindow(
	history []providers.Message,
	build func([]providers.Message) []providers.Message,
	contextWindow int,
	toolDefs []providers.ToolDefinition,
	maxTokens int,
	minTurns int,
) ([]providers.Message, []providers.Message, bool) {
	if minTurns < 0 {
		minTurns = 0
	}

	messages := build(history)
	if !isOverContextBudget(contextWindow, messages, toolDefs, maxTokens) {
		return history, messages, true
	}

	trimmedHistory := append([]providers.Message(nil), history...)
	for len(trimmedHistory) > 0 {
		// Stop at the retention floor: never drop the most recent turns.
		if len(parseTurnBoundaries(trimmedHistory)) <= minTurns {
			break
		}
		dropUntil := nextHistoryTrimStart(trimmedHistory)
		if dropUntil <= 0 || dropUntil >= len(trimmedHistory) {
			trimmedHistory = nil
		} else {
			trimmedHistory = append([]providers.Message(nil), trimmedHistory[dropUntil:]...)
		}

		messages = build(trimmedHistory)
		if !isOverContextBudget(contextWindow, messages, toolDefs, maxTokens) {
			return trimmedHistory, messages, true
		}
	}

	messages = build(trimmedHistory)
	return trimmedHistory, messages, !isOverContextBudget(contextWindow, messages, toolDefs, maxTokens)
}

func nextHistoryTrimStart(history []providers.Message) int {
	if len(history) == 0 {
		return 0
	}

	turns := parseTurnBoundaries(history)
	if len(turns) >= 2 {
		return turns[1]
	}
	if len(turns) == 1 {
		if turns[0] > 0 {
			return turns[0]
		}
		return len(history)
	}

	return len(history)
}
