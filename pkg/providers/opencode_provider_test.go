package providers

import "testing"

func TestOpenCodeProviderCatalog(t *testing.T) {
	goOption, ok := modelProviderOptionForName("opencode-go")
	if !ok {
		t.Fatal("opencode-go provider is missing")
	}
	if goOption.DefaultAPIBase != "https://opencode.ai/zen/go/v1" {
		t.Fatalf("opencode-go base = %q", goOption.DefaultAPIBase)
	}
	if !goOption.CreateAllowed || !goOption.DefaultModelAllowed || !goOption.SupportsFetch {
		t.Fatalf("opencode-go flags = %#v", goOption)
	}
	if !IsHTTPAPIProtocol("opencode-go") {
		t.Fatal("opencode-go should use the HTTP API path")
	}
	if !IsSupportedModelProvider("opencode") {
		t.Fatal("the opencode alias should resolve to a supported provider")
	}
	aliased, _ := modelProviderOptionForName("opencode")
	if aliased.ID != "opencode-go" {
		t.Fatalf("opencode alias = %q, want opencode-go", aliased.ID)
	}

	zenOption, ok := modelProviderOptionForName("opencode-zen")
	if !ok {
		t.Fatal("opencode-zen provider is missing")
	}
	if zenOption.DefaultAPIBase != "https://opencode.ai/zen/v1" {
		t.Fatalf("opencode-zen base = %q", zenOption.DefaultAPIBase)
	}
	if !IsSupportedModelProvider("opencode-zen") || !IsHTTPAPIProtocol("opencode-zen") {
		t.Fatal("opencode-zen should be a supported HTTP API provider")
	}
}
