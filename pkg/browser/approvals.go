package browser

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// ApprovalDecision is the operator's answer to a domain approval request.
type ApprovalDecision string

const (
	AllowOnce   ApprovalDecision = "once"
	AllowAlways ApprovalDecision = "always"
	Deny        ApprovalDecision = "deny"
)

// PendingApproval is a domain waiting for the operator's answer.
type PendingApproval struct {
	ID        string `json:"id"`
	Domain    string `json:"domain"`
	CreatedAt int64  `json:"created_at"`
}

type approvalWaiter struct {
	request PendingApproval
	result  chan ApprovalDecision
	expires time.Time
}

// resolvedApproval remembers an answered request for a short while so a second
// answer (another tab, or a duplicate click that raced a poll) succeeds
// silently instead of failing with "already answered".
type resolvedApproval struct {
	request  PendingApproval
	decision ApprovalDecision
	at       time.Time
}

// ApprovalStore tracks domain approvals and the domains approved for the
// lifetime of this process. Persistent "always allow" entries are also written
// to config by the launcher API.
type ApprovalStore struct {
	mu       sync.Mutex
	pending  map[string]*approvalWaiter
	resolved map[string]resolvedApproval
	always   map[string]struct{}
}

// NewApprovalStore creates an empty approval store.
func NewApprovalStore() *ApprovalStore {
	return &ApprovalStore{
		pending:  make(map[string]*approvalWaiter),
		resolved: make(map[string]resolvedApproval),
		always:   make(map[string]struct{}),
	}
}

var defaultApprovals = NewApprovalStore()

// DefaultApprovals returns the process-wide approval store used by the tool.
func DefaultApprovals() *ApprovalStore { return defaultApprovals }

// Request waits for an operator decision for domain. Defaults to Deny when the
// timeout or context expires.
func (s *ApprovalStore) Request(ctx context.Context, domain string, timeout time.Duration) ApprovalDecision {
	domain = NormalizeDomainPattern(domain)
	if domain == "" {
		return Deny
	}
	if s.IsAlwaysAllowed(domain) {
		return AllowAlways
	}

	request := PendingApproval{
		ID:        newApprovalID(),
		Domain:    domain,
		CreatedAt: time.Now().UnixMilli(),
	}
	waiter := &approvalWaiter{
		request: request,
		result:  make(chan ApprovalDecision, 1),
		expires: time.Now().Add(timeout),
	}

	s.mu.Lock()
	s.pending[request.ID] = waiter
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.pending, request.ID)
		s.mu.Unlock()
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case decision := <-waiter.result:
		if decision == AllowAlways {
			s.AddAlwaysAllowed(domain)
		}
		return decision
	case <-timer.C:
		return Deny
	case <-ctx.Done():
		return Deny
	}
}

// List returns pending requests that have not expired.
func (s *ApprovalStore) List() []PendingApproval {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]PendingApproval, 0, len(s.pending))
	for id, waiter := range s.pending {
		if now.After(waiter.expires) {
			delete(s.pending, id)
			continue
		}
		result = append(result, waiter.request)
	}
	return result
}

// Resolve answers a pending request. It returns the request and false when the
// ID is unknown. Answering the same request again within a short window returns
// the original decision instead of an error.
func (s *ApprovalStore) Resolve(id string, decision ApprovalDecision) (PendingApproval, bool) {
	switch decision {
	case AllowOnce, AllowAlways, Deny:
	default:
		return PendingApproval{}, false
	}

	s.mu.Lock()
	waiter, ok := s.pending[id]
	if ok {
		delete(s.pending, id)
		s.resolved[id] = resolvedApproval{
			request:  waiter.request,
			decision: decision,
			at:       time.Now(),
		}
		s.pruneResolvedLocked()
	}
	previous, wasResolved := s.resolved[id]
	s.mu.Unlock()
	if !ok {
		if wasResolved {
			return previous.request, true
		}
		return PendingApproval{}, false
	}

	select {
	case waiter.result <- decision:
	default:
	}
	if decision == AllowAlways {
		s.AddAlwaysAllowed(waiter.request.Domain)
	}
	return waiter.request, true
}

// pruneResolvedLocked drops remembered answers older than a minute. Callers
// must hold s.mu.
func (s *ApprovalStore) pruneResolvedLocked() {
	if len(s.resolved) == 0 {
		return
	}
	cutoff := time.Now().Add(-time.Minute)
	for id, entry := range s.resolved {
		if entry.at.Before(cutoff) {
			delete(s.resolved, id)
		}
	}
}

// AddAlwaysAllowed remembers a domain until the process restarts.
func (s *ApprovalStore) AddAlwaysAllowed(domain string) {
	domain = NormalizeDomainPattern(domain)
	if domain == "" {
		return
	}
	s.mu.Lock()
	s.always[domain] = struct{}{}
	s.mu.Unlock()
	allowProxyHostForever(domain)
}

// IsAlwaysAllowed reports whether the domain was approved with "Always allow".
func (s *ApprovalStore) IsAlwaysAllowed(domain string) bool {
	domain = NormalizeDomainPattern(domain)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.always[domain]
	return ok
}

func newApprovalID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return strings.ReplaceAll(time.Now().Format("150405.000000"), ".", "")
	}
	return hex.EncodeToString(buf)
}

// AllowTemporarily permits a host through the browser proxy for ttl.
func AllowTemporarily(host string, ttl time.Duration) {
	allowProxyHostTemporarily(host, ttl)
}

// AllowAlways permits a host through the proxy and records it in the store.
func AllowHostAlways(host string) {
	DefaultApprovals().AddAlwaysAllowed(host)
}
