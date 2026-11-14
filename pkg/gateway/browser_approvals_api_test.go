package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/browser"
)

func TestBrowserApprovalAPI_ListAndResolve(t *testing.T) {
	store := browser.NewApprovalStore()
	handler := newBrowserApprovalAPI(store, "test-token")

	decisionCh := make(chan browser.ApprovalDecision, 1)
	go func() {
		decisionCh <- store.Request(context.Background(), "news.example", 5*time.Second)
	}()

	var approval browser.PendingApproval
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/browser/approvals", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Approvals []browser.PendingApproval `json:"approvals"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal error = %v", err)
		}
		if len(payload.Approvals) > 0 {
			approval = payload.Approvals[0]
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if approval.ID == "" {
		t.Fatal("pending approval not listed")
	}

	body := bytes.NewBufferString(`{"decision":"always"}`)
	req := httptest.NewRequest(http.MethodPost, "/browser/approvals/"+approval.ID, body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("resolve status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if decision := <-decisionCh; decision != browser.AllowAlways {
		t.Fatalf("decision = %q, want always", decision)
	}

	unknown := httptest.NewRequest(http.MethodPost, "/browser/approvals/unknown",
		bytes.NewBufferString(`{"decision":"once"}`))
	unknown.Header.Set("Authorization", "Bearer test-token")
	unknownRec := httptest.NewRecorder()
	handler.ServeHTTP(unknownRec, unknown)
	if unknownRec.Code != http.StatusNotFound {
		t.Fatalf("unknown status = %d, want 404", unknownRec.Code)
	}

	unauthorized := httptest.NewRequest(http.MethodGet, "/browser/approvals", nil)
	unauthorizedRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedRec, unauthorized)
	if unauthorizedRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401", unauthorizedRec.Code)
	}
}
