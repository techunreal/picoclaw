package browser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

// NormalizeDomainPattern canonicalizes an allowlist entry. Accepted forms:
// example.com, *.example.com, https://example.com/path, example.com:8080.
func NormalizeDomainPattern(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		if parsed, err := url.Parse(value); err == nil {
			value = parsed.Host
		}
	}
	if idx := strings.IndexByte(value, '/'); idx >= 0 {
		value = value[:idx]
	}
	if host, port, err := splitHostPortLoose(value); err == nil && port != "" {
		value = host
	}
	value = strings.TrimSuffix(value, ".")
	return value
}

func splitHostPortLoose(value string) (string, string, error) {
	if strings.HasPrefix(value, "[") {
		return value, "", nil
	}
	idx := strings.LastIndexByte(value, ':')
	if idx < 0 {
		return value, "", nil
	}
	host, port := value[:idx], value[idx+1:]
	if host == "" || strings.ContainsAny(port, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-") {
		return value, "", fmt.Errorf("not a host:port pair")
	}
	return host, port, nil
}

func normalizeDomainPatterns(patterns []string) []string {
	result := make([]string, 0, len(patterns))
	seen := make(map[string]struct{}, len(patterns))
	for _, pattern := range patterns {
		normalized := NormalizeDomainPattern(pattern)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

// HostAllowed reports whether host matches one of the allowlist patterns.
func HostAllowed(host string, patterns []string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return false
	}
	for _, raw := range patterns {
		pattern := NormalizeDomainPattern(raw)
		switch {
		case pattern == "":
			continue
		case pattern == host:
			return true
		case strings.HasPrefix(pattern, "*."):
			bare := strings.TrimPrefix(pattern, "*.")
			suffix := strings.TrimPrefix(pattern, "*")
			if host == bare || strings.HasSuffix(host, suffix) {
				return true
			}
		}
	}
	return false
}

func validateURL(raw string, patterns []string, allowAll bool) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("invalid url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("only http and https URLs are allowed")
	}
	if parsed.User != nil {
		return fmt.Errorf("URLs with embedded credentials are not allowed")
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("url has no host")
	}
	if allowAll || HostAllowed(parsed.Hostname(), patterns) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrDomainNotAllowed, parsed.Hostname())
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:4])
}
