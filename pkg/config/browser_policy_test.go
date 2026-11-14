package config

import "testing"

func TestBrowserToolsConfig_UnknownDomainPolicy(t *testing.T) {
	tests := []struct {
		name   string
		cfg    BrowserToolsConfig
		expect string
	}{
		{name: "default", cfg: BrowserToolsConfig{}, expect: "ask"},
		{name: "explicit ask", cfg: BrowserToolsConfig{PermissionPolicy: "ask"}, expect: "ask"},
		{name: "allow", cfg: BrowserToolsConfig{PermissionPolicy: "allow"}, expect: "allow"},
		{name: "deny", cfg: BrowserToolsConfig{PermissionPolicy: "deny"}, expect: "deny"},
		{name: "legacy allow", cfg: BrowserToolsConfig{AllowAllDomains: true}, expect: "allow"},
		{name: "unknown value", cfg: BrowserToolsConfig{PermissionPolicy: "sometimes"}, expect: "ask"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.cfg.UnknownDomainPolicy(); got != test.expect {
				t.Fatalf("UnknownDomainPolicy() = %q, want %q", got, test.expect)
			}
		})
	}
}

func TestDefaultBrowserPolicyIsAsk(t *testing.T) {
	cfg := DefaultConfig()
	if got := cfg.Tools.Browser.UnknownDomainPolicy(); got != "ask" {
		t.Fatalf("default policy = %q, want ask", got)
	}
	if cfg.Tools.Browser.IsAllowAllDomains() {
		t.Fatal("default policy must not allow all domains")
	}
}
