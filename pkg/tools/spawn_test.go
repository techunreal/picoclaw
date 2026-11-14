package tools

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// mockSpawner implements SubTurnSpawner for testing.
type mockSpawner struct {
	lastConfig SubTurnConfig
	done       chan struct{}
}

func (m *mockSpawner) SpawnSubTurn(ctx context.Context, cfg SubTurnConfig) (*ToolResult, error) {
	m.lastConfig = cfg
	if m.done != nil {
		close(m.done)
	}

	// Extract task from system prompt for response
	task := cfg.SystemPrompt
	if strings.Contains(task, "Task: ") {
		parts := strings.Split(task, "Task: ")
		if len(parts) > 1 {
			task = parts[1]
		}
	}
	return &ToolResult{
		ForLLM:  "Task completed: " + task,
		ForUser: "Task completed",
	}, nil
}

func TestSpawnTool_Execute_EmptyTask(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test")
	tool := NewSpawnTool(manager)

	ctx := context.Background()

	tests := []struct {
		name string
		args map[string]any
	}{
		{"empty string", map[string]any{"task": ""}},
		{"whitespace only", map[string]any{"task": "   "}},
		{"tabs and newlines", map[string]any{"task": "\t\n  "}},
		{"missing task key", map[string]any{"label": "test"}},
		{"wrong type", map[string]any{"task": 123}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tool.Execute(ctx, tt.args)
			if result == nil {
				t.Fatal("Result should not be nil")
			}
			if !result.IsError {
				t.Error("Expected error for invalid task parameter")
			}
			if !strings.Contains(result.ForLLM, "task is required") {
				t.Errorf("Error message should mention 'task is required', got: %s", result.ForLLM)
			}
		})
	}
}

func TestSpawnTool_Execute_ValidTask(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test")
	tool := NewSpawnTool(manager)
	spawner := &mockSpawner{done: make(chan struct{})}
	tool.SetSpawner(spawner)

	ctx := context.Background()
	args := map[string]any{
		"task":     "Write a haiku about coding",
		"label":    "haiku-task",
		"agent_id": "research",
	}

	result := tool.Execute(ctx, args)
	if result == nil {
		t.Fatal("Result should not be nil")
	}
	if result.IsError {
		t.Errorf("Expected success for valid task, got error: %s", result.ForLLM)
	}
	if !result.Async {
		t.Error("SpawnTool should return async result")
	}
	<-spawner.done
	if spawner.lastConfig.TargetAgentID != "research" {
		t.Errorf("TargetAgentID = %q, want research", spawner.lastConfig.TargetAgentID)
	}
	if !spawner.lastConfig.Critical {
		t.Error("SpawnTool should mark background subturns as critical")
	}
}

func TestSpawnTool_Execute_NilManager(t *testing.T) {
	tool := NewSpawnTool(nil)

	ctx := context.Background()
	args := map[string]any{"task": "test task"}

	result := tool.Execute(ctx, args)
	if !result.IsError {
		t.Error("Expected error for nil manager")
	}
	if !strings.Contains(result.ForLLM, "Subagent manager not configured") {
		t.Errorf("Error message should mention manager not configured, got: %s", result.ForLLM)
	}
}

// controlledSpawner blocks until the test releases it, so task states can be
// inspected while the sub-turn is still running.
type controlledSpawner struct {
	started chan struct{}
	release chan struct{}
	result  *ToolResult
	err     error
	once    sync.Once
}

func (s *controlledSpawner) SpawnSubTurn(context.Context, SubTurnConfig) (*ToolResult, error) {
	s.once.Do(func() { close(s.started) })
	<-s.release
	return s.result, s.err
}

func TestSpawnTool_TracksTaskUntilCompletion(t *testing.T) {
	manager := NewSubagentManager(&MockLLMProvider{}, "test-model", t.TempDir())
	tool := NewSpawnTool(manager)
	spawner := &controlledSpawner{
		started: make(chan struct{}),
		release: make(chan struct{}),
		result:  &ToolResult{ForLLM: "task finished", ForUser: "done"},
	}
	tool.SetSpawner(spawner)

	ctx := WithToolInboundContext(context.Background(), "telegram", "chat-42", "msg-1", "")
	done := make(chan *ToolResult, 1)
	result := tool.ExecuteAsync(ctx, map[string]any{
		"task":     "investigate the logs",
		"label":    "research",
		"agent_id": "beta",
	}, func(_ context.Context, r *ToolResult) {
		done <- r
	})

	if result == nil || result.IsError {
		t.Fatalf("ExecuteAsync() result = %#v, want async success", result)
	}
	if !result.Async {
		t.Error("SpawnTool should return an async result")
	}
	if !strings.Contains(result.ForLLM, "subagent-1") {
		t.Errorf("acknowledgment should include the task ID, got: %s", result.ForLLM)
	}

	<-spawner.started

	task, ok := manager.GetTaskCopy("subagent-1")
	if !ok {
		t.Fatal("spawned task was not recorded in the manager")
	}
	if task.Status != "running" {
		t.Fatalf("task status = %q, want running", task.Status)
	}
	if task.Task != "investigate the logs" || task.Label != "research" || task.AgentID != "beta" {
		t.Fatalf("task metadata mismatch: %#v", task)
	}
	if task.OriginChannel != "telegram" || task.OriginChatID != "chat-42" {
		t.Fatalf("task origin mismatch: %#v", task)
	}

	statusTool := NewSpawnStatusTool(manager)
	statusCtx := WithToolInboundContext(context.Background(), "telegram", "chat-42", "msg-1", "")
	runningReport := statusTool.Execute(statusCtx, nil)
	if !strings.Contains(runningReport.ForLLM, "status=running") {
		t.Fatalf("spawn_status should report a running task, got: %s", runningReport.ForLLM)
	}

	close(spawner.release)
	<-done

	task, ok = manager.GetTaskCopy("subagent-1")
	if !ok {
		t.Fatal("completed task disappeared from the manager")
	}
	if task.Status != "completed" {
		t.Fatalf("task status = %q, want completed", task.Status)
	}
	if task.Result != "task finished" {
		t.Fatalf("task result = %q, want task finished", task.Result)
	}

	completedReport := statusTool.Execute(statusCtx, map[string]any{"task_id": "subagent-1"})
	if !strings.Contains(completedReport.ForLLM, "status=completed") {
		t.Fatalf("spawn_status should report completion, got: %s", completedReport.ForLLM)
	}
	if !strings.Contains(completedReport.ForLLM, "result: task finished") {
		t.Fatalf("spawn_status should include the result, got: %s", completedReport.ForLLM)
	}

	otherConversation := statusTool.Execute(
		WithToolInboundContext(context.Background(), "discord", "chat-9", "msg-2", ""),
		nil,
	)
	if !strings.Contains(otherConversation.ForLLM, "No subagents found for this conversation.") {
		t.Fatalf("spawn_status leaked tasks across conversations: %s", otherConversation.ForLLM)
	}
}

func TestSpawnTool_TracksFailedTask(t *testing.T) {
	manager := NewSubagentManager(&MockLLMProvider{}, "test-model", t.TempDir())
	tool := NewSpawnTool(manager)
	spawner := &controlledSpawner{
		started: make(chan struct{}),
		release: make(chan struct{}),
		err:     errors.New("llm exploded"),
	}
	tool.SetSpawner(spawner)

	done := make(chan *ToolResult, 1)
	tool.ExecuteAsync(context.Background(), map[string]any{"task": "fail please"}, func(_ context.Context, r *ToolResult) {
		done <- r
	})
	close(spawner.release)
	<-done

	task, ok := manager.GetTaskCopy("subagent-1")
	if !ok {
		t.Fatal("failed task was not recorded")
	}
	if task.Status != "failed" {
		t.Fatalf("task status = %q, want failed", task.Status)
	}
	if !strings.Contains(task.Result, "Spawn failed: llm exploded") {
		t.Fatalf("task result = %q, want spawn failure details", task.Result)
	}
}

func TestSpawnTool_TracksCanceledTask(t *testing.T) {
	manager := NewSubagentManager(&MockLLMProvider{}, "test-model", t.TempDir())
	tool := NewSpawnTool(manager)
	spawner := &controlledSpawner{
		started: make(chan struct{}),
		release: make(chan struct{}),
		err:     context.Canceled,
	}
	tool.SetSpawner(spawner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan *ToolResult, 1)
	tool.ExecuteAsync(ctx, map[string]any{"task": "cancel me"}, func(_ context.Context, r *ToolResult) {
		done <- r
	})
	close(spawner.release)
	<-done

	task, ok := manager.GetTaskCopy("subagent-1")
	if !ok {
		t.Fatal("canceled task was not recorded")
	}
	if task.Status != "canceled" {
		t.Fatalf("task status = %q, want canceled", task.Status)
	}
}
