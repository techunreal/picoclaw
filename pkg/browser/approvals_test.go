package browser

import (
	"context"
	"testing"
	"time"
)

func TestApprovalStore_ResolveOnce(t *testing.T) {
	store := NewApprovalStore()
	result := make(chan ApprovalDecision, 1)
	go func() {
		result <- store.Request(context.Background(), "example.com", 5*time.Second)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if pending := store.List(); len(pending) == 1 {
			if pending[0].Domain != "example.com" {
				t.Fatalf("domain = %q", pending[0].Domain)
			}
			if _, ok := store.Resolve(pending[0].ID, AllowOnce); !ok {
				t.Fatal("Resolve() failed")
			}
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	select {
	case decision := <-result:
		if decision != AllowOnce {
			t.Fatalf("decision = %q, want once", decision)
		}
	case <-time.After(time.Second):
		t.Fatal("Request() did not return")
	}
	if store.IsAlwaysAllowed("example.com") {
		t.Fatal("AllowOnce must not persist")
	}
}

func TestApprovalStore_AlwaysAndAutoApprove(t *testing.T) {
	store := NewApprovalStore()
	store.AddAlwaysAllowed("example.com")
	if !store.IsAlwaysAllowed("example.com") {
		t.Fatal("domain was not stored")
	}
	if decision := store.Request(context.Background(), "example.com", time.Second); decision != AllowAlways {
		t.Fatalf("decision = %q, want always", decision)
	}
}

func TestApprovalStore_TimeoutDenies(t *testing.T) {
	store := NewApprovalStore()
	start := time.Now()
	decision := store.Request(context.Background(), "slow.example", 30*time.Millisecond)
	if decision != Deny {
		t.Fatalf("decision = %q, want deny", decision)
	}
	if time.Since(start) > time.Second {
		t.Fatal("timeout took too long")
	}
	if pending := store.List(); len(pending) != 0 {
		t.Fatalf("pending approvals left behind: %v", pending)
	}
}
