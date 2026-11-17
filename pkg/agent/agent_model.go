// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// DefaultModelSwitchWait is how long an API caller waits for an in-flight turn
// to finish before the switch is reported as pending. The switch itself keeps
// waiting in the background and applies as soon as the turn releases the model
// lock.
const DefaultModelSwitchWait = 2 * time.Second

// ErrAgentNotFound is returned when a requested agent ID is not registered.
var ErrAgentNotFound = errors.New("agent not found")

// GetAgentModel returns the currently active model for an agent.
func (al *AgentLoop) GetAgentModel(agentID string) (string, bool) {
	if al == nil {
		return "", false
	}
	registry := al.GetRegistry()
	if registry == nil {
		return "", false
	}
	agent, ok := registry.GetAgent(agentID)
	if !ok {
		return "", false
	}
	return agent.ModelSnapshot().Model, true
}

// SetDefaultModelName updates the in-memory global default model alias. It is
// used when Pico's model changes so subsequently created turns and inherited
// agents resolve the new default without a gateway restart.
func (al *AgentLoop) SetDefaultModelName(modelName string) {
	if al == nil {
		return
	}
	modelName = strings.TrimSpace(modelName)
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.cfg != nil {
		al.cfg.Agents.Defaults.ModelName = modelName
	}
}

// SwitchAgentModel switches one agent's model without restarting the gateway.
// In-flight turns keep the read lock until they finish; this method waits up to
// wait for that to happen. When the wait expires it returns applied=false and
// the switch continues in the background.
func (al *AgentLoop) SwitchAgentModel(
	agentID, modelName string,
	wait time.Duration,
) (oldModel string, applied bool, err error) {
	if al == nil {
		return "", false, fmt.Errorf("agent loop is nil")
	}
	agentID = strings.TrimSpace(agentID)
	modelName = strings.TrimSpace(modelName)
	if agentID == "" {
		return "", false, fmt.Errorf("agent id is required")
	}
	registry := al.GetRegistry()
	if registry == nil {
		return "", false, fmt.Errorf("agent registry is unavailable")
	}
	agent, ok := registry.GetAgent(agentID)
	if !ok {
		return "", false, fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}
	cfg := al.GetConfig()
	if err := agent.ValidateModel(cfg, modelName); err != nil {
		return "", false, err
	}
	if wait <= 0 {
		wait = DefaultModelSwitchWait
	}

	type switchResult struct {
		old string
		err error
	}
	done := make(chan switchResult, 1)
	go func() {
		old, switchErr := agent.SwitchModelSerialized(cfg, modelName)
		done <- switchResult{old: old, err: switchErr}
	}()

	select {
	case result := <-done:
		return result.old, true, result.err
	case <-time.After(wait):
		return "", false, nil
	}
}
