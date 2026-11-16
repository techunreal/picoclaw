package browser

import (
	"strings"
	"sync"
)

// runtimeState holds browser settings applied after startup. The browser tool
// and allowlist proxy are created once when the gateway starts, so config edits
// must be pushed into the running process to take effect without a restart.
type runtimeSettings struct {
	mu          sync.RWMutex
	applied     bool
	domains     []string
	allowAll    bool
	policy      string
	allowRemote bool
}

var runtimeState runtimeSettings

// ApplyRuntimeSettings pushes allowlist and permission settings into the running
// gateway process. The shared allowlist proxy is updated as well so existing
// browser sessions pick up the new rules immediately.
func ApplyRuntimeSettings(domains []string, allowAll bool, policy string, allowRemote bool) error {
	normalized := normalizeDomainPatterns(domains)
	if !allowAll && len(normalized) == 0 {
		return ErrNoAllowedDomains
	}

	proxy, err := sharedAllowlistProxy()
	if err != nil {
		return err
	}
	proxy.setAllowlist(domains, allowAll)

	policy = strings.ToLower(strings.TrimSpace(policy))
	switch policy {
	case "ask", "allow", "deny":
	default:
		policy = "ask"
	}

	runtimeState.mu.Lock()
	runtimeState.applied = true
	runtimeState.domains = append([]string(nil), normalized...)
	runtimeState.allowAll = allowAll
	runtimeState.policy = policy
	runtimeState.allowRemote = allowRemote
	runtimeState.mu.Unlock()
	return nil
}

// RuntimeSettings reports the live settings when they have been pushed into this
// process. The second return value is false when no runtime override exists.
func RuntimeSettings() (policy string, allowRemote bool, ok bool) {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()
	if !runtimeState.applied {
		return "", false, false
	}
	return runtimeState.policy, runtimeState.allowRemote, true
}

// runtimeDomainOverride returns the live allowlist when one has been applied.
func runtimeDomainOverride() ([]string, bool, bool) {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()
	if !runtimeState.applied {
		return nil, false, false
	}
	return append([]string(nil), runtimeState.domains...), runtimeState.allowAll, true
}
