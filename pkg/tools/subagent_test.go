package tools

import (
	"fmt"
	"testing"
)

func TestSubagentManagerPrunesFinishedTasks(t *testing.T) {
	sm := NewSubagentManager(nil, "test-model", "")

	const runningID = "running-oldest"
	sm.tasks[runningID] = &SubagentTask{
		ID:      runningID,
		Status:  "running",
		Created: 0,
	}

	for i := 0; i < maxRetainedSubagentTasks+20; i++ {
		id := fmt.Sprintf("finished-%d", i)
		sm.tasks[id] = &SubagentTask{
			ID:      id,
			Status:  "completed",
			Created: int64(i + 1),
		}
	}

	sm.mu.Lock()
	sm.pruneTasksLocked()
	sm.mu.Unlock()

	if len(sm.tasks) > maxRetainedSubagentTasks {
		t.Fatalf("retained tasks = %d, want <= %d", len(sm.tasks), maxRetainedSubagentTasks)
	}
	if _, ok := sm.tasks[runningID]; !ok {
		t.Fatal("running task must never be pruned")
	}
	if _, ok := sm.tasks["finished-0"]; ok {
		t.Fatal("oldest finished task should be pruned")
	}
	newestID := fmt.Sprintf("finished-%d", maxRetainedSubagentTasks+19)
	if _, ok := sm.tasks[newestID]; !ok {
		t.Fatalf("newest finished task %q should be retained", newestID)
	}
}

func TestSubagentManagerGetTaskReturnsSnapshot(t *testing.T) {
	sm := NewSubagentManager(nil, "test-model", "")
	sm.tasks["task-1"] = &SubagentTask{
		ID:     "task-1",
		Status: "running",
		Result: "original",
	}

	got, ok := sm.GetTask("task-1")
	if !ok {
		t.Fatal("GetTask() did not find task")
	}
	got.Status = "completed"
	got.Result = "mutated"

	original := sm.tasks["task-1"]
	if original.Status != "running" || original.Result != "original" {
		t.Fatalf("GetTask() returned a shared pointer: %+v", original)
	}
}
