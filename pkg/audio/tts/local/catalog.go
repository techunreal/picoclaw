package local

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// VoiceOption is one selectable speaker exposed by a local TTS model.
type VoiceOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// FrontendSpec describes the text-to-phoneme front end used by a model whose
// adapter needs more than the built-in phonemizer. It is data only: PicoClaw
// never executes code from a model or manifest.
type FrontendSpec struct {
	Kind          string              `json:"kind"`
	EspeakVoice   string              `json:"espeak_voice,omitempty"`
	Symbols       []string            `json:"symbols,omitempty"`
	Vowels        []string            `json:"vowels,omitempty"`
	Fallback      map[string][]string `json:"fallback,omitempty"`
	Punctuation   []string            `json:"punctuation,omitempty"`
	Abbreviations []string            `json:"abbreviations,omitempty"`
	NotAnEnd      []string            `json:"not_an_end,omitempty"`
	SentenceStart string              `json:"sentence_start,omitempty"`
}

// Recipe describes a supported local TTS model and how to install it.
type Recipe struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Language         string            `json:"language"`
	License          string            `json:"license"`
	LicenseURL       string            `json:"license_url,omitempty"`
	SourceURL        string            `json:"source_url,omitempty"`
	HFRepo           string            `json:"hf_repo"`
	ModelFile        string            `json:"model_file"`
	ConfigFile       string            `json:"config_file"`
	Adapter          string            `json:"adapter"`
	ExtraPrefixes    []string          `json:"extra_prefixes,omitempty"`
	PhonemizerSource string            `json:"phonemizer_source"`
	SizeBytes        int64             `json:"size_bytes"`
	SampleRate       int               `json:"sample_rate"`
	MaxTokens        int               `json:"max_tokens,omitempty"`
	Voices           []VoiceOption     `json:"voices"`
	DefaultVoice     string            `json:"default_voice"`
	ThirdParty       bool              `json:"third_party,omitempty"`
	Checksums        map[string]string `json:"checksums,omitempty"`
	Requires         []string          `json:"requires,omitempty"`
	Frontend         *FrontendSpec     `json:"frontend,omitempty"`
}

// Manifest is the versioned list of local TTS recipes. Built-in models and
// user-supplied models share the same schema.
type Manifest struct {
	Version int      `json:"version"`
	Models  []Recipe `json:"models"`
}

const (
	manifestVersion = 1
	maxManifestSize = 1 << 20
	maxModelSize    = 200 << 20
)

var (
	//go:embed manifest.json
	builtinManifestJSON []byte

	builtinOnce sync.Once
	builtinList []Recipe
	builtinErr  error

	idPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}$`)
	langPattern  = regexp.MustCompile(`^[a-z]{2,3}$`)
	repoPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`)
	hex64Pattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
)

func allowedAdapter(adapter string) bool {
	switch adapter {
	case "clarvia", "tiny-voca", "espeak":
		return true
	}
	return false
}

func allowedDependency(name string) bool {
	return name == "espeak-ng"
}

// BuiltinManifest returns the curated catalog compiled into the binary.
func BuiltinManifest() (Manifest, error) {
	builtinOnce.Do(func() {
		var manifest Manifest
		if err := json.Unmarshal(builtinManifestJSON, &manifest); err != nil {
			builtinErr = fmt.Errorf("built-in TTS manifest is invalid: %w", err)
			return
		}
		if err := validateManifest(&manifest, nil); err != nil {
			builtinErr = fmt.Errorf("built-in TTS manifest is invalid: %w", err)
			return
		}
		builtinList = manifest.Models
	})
	return Manifest{Version: manifestVersion, Models: append([]Recipe(nil), builtinList...)}, builtinErr
}

// Catalog returns the curated local TTS models. The catalog is intentionally
// small: arbitrary repositories are not executed automatically.
func Catalog() []Recipe {
	manifest, err := BuiltinManifest()
	if err != nil {
		return nil
	}
	return append([]Recipe(nil), manifest.Models...)
}

// ParseManifest decodes and validates a manifest supplied by a user.
func ParseManifest(raw []byte) (Manifest, error) {
	if len(raw) == 0 {
		return Manifest{}, fmt.Errorf("manifest is empty")
	}
	if len(raw) > maxManifestSize {
		return Manifest{}, fmt.Errorf("manifest is too large (max %d bytes)", maxManifestSize)
	}
	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("manifest is not valid JSON: %w", err)
	}
	if err := validateManifest(&manifest, catalogIDs()); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func catalogIDs() map[string]struct{} {
	manifest, err := BuiltinManifest()
	if err != nil {
		return map[string]struct{}{}
	}
	ids := make(map[string]struct{}, len(manifest.Models))
	for _, recipe := range manifest.Models {
		ids[strings.ToLower(recipe.ID)] = struct{}{}
	}
	return ids
}

func validateManifest(manifest *Manifest, reserved map[string]struct{}) error {
	if manifest.Version != manifestVersion {
		return fmt.Errorf("unsupported manifest version %d (want %d)", manifest.Version, manifestVersion)
	}
	if len(manifest.Models) == 0 {
		return fmt.Errorf("manifest has no models")
	}
	seen := make(map[string]struct{}, len(manifest.Models))
	for index := range manifest.Models {
		recipe := &manifest.Models[index]
		if err := validateRecipe(recipe); err != nil {
			return fmt.Errorf("model %q: %w", recipe.ID, err)
		}
		key := strings.ToLower(recipe.ID)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate model id %q", recipe.ID)
		}
		seen[key] = struct{}{}
		if reserved != nil {
			if _, clash := reserved[key]; clash {
				return fmt.Errorf("model id %q is reserved by a built-in model", recipe.ID)
			}
			if !recipe.ThirdParty {
				return fmt.Errorf("model %q: user manifest models must set third_party", recipe.ID)
			}
		}
	}
	return nil
}

func validateRecipe(recipe *Recipe) error {
	recipe.ID = strings.TrimSpace(recipe.ID)
	recipe.Name = strings.TrimSpace(recipe.Name)
	recipe.Language = strings.ToLower(strings.TrimSpace(recipe.Language))
	recipe.License = strings.TrimSpace(recipe.License)
	recipe.Adapter = strings.TrimSpace(recipe.Adapter)
	recipe.DefaultVoice = strings.TrimSpace(recipe.DefaultVoice)

	if !idPattern.MatchString(recipe.ID) {
		return fmt.Errorf("id must match %s", idPattern.String())
	}
	if recipe.Name == "" {
		return fmt.Errorf("name is required")
	}
	if !langPattern.MatchString(recipe.Language) {
		return fmt.Errorf("language must be a two or three letter code")
	}
	if !allowedAdapter(recipe.Adapter) {
		return fmt.Errorf("adapter %q is not supported", recipe.Adapter)
	}
	if !repoPattern.MatchString(recipe.HFRepo) {
		return fmt.Errorf("hf_repo must look like owner/name")
	}
	if err := validateRelativePath(recipe.ModelFile); err != nil {
		return fmt.Errorf("model_file: %w", err)
	}
	if err := validateRelativePath(recipe.ConfigFile); err != nil {
		return fmt.Errorf("config_file: %w", err)
	}
	for _, prefix := range recipe.ExtraPrefixes {
		if err := validateRelativePrefix(prefix); err != nil {
			return fmt.Errorf("extra_prefixes: %w", err)
		}
	}
	if recipe.SampleRate == 0 {
		recipe.SampleRate = 24000
	}
	if recipe.SampleRate < 8000 || recipe.SampleRate > 48000 {
		return fmt.Errorf("sample_rate %d is outside 8000-48000", recipe.SampleRate)
	}
	if recipe.SizeBytes < 0 || recipe.SizeBytes > maxModelSize {
		return fmt.Errorf("size_bytes %d is outside 0-%d", recipe.SizeBytes, maxModelSize)
	}
	if len(recipe.Voices) == 0 {
		return fmt.Errorf("voices are required")
	}
	known := make(map[string]struct{}, len(recipe.Voices))
	for _, voice := range recipe.Voices {
		if strings.TrimSpace(voice.ID) == "" {
			return fmt.Errorf("voice id is empty")
		}
		known[voice.ID] = struct{}{}
	}
	if recipe.DefaultVoice == "" || !contains(known, recipe.DefaultVoice) {
		return fmt.Errorf("default_voice %q is not one of the voices", recipe.DefaultVoice)
	}
	for _, dependency := range recipe.Requires {
		if !allowedDependency(dependency) {
			return fmt.Errorf("dependency %q is not supported", dependency)
		}
	}
	if recipe.Frontend != nil {
		if recipe.Frontend.Kind != recipe.Adapter {
			return fmt.Errorf("frontend kind %q does not match adapter %q", recipe.Frontend.Kind, recipe.Adapter)
		}
		switch recipe.Frontend.Kind {
		case "espeak":
			if strings.TrimSpace(recipe.Frontend.EspeakVoice) == "" {
				return fmt.Errorf("frontend.espeak_voice is required")
			}
			if len(recipe.Frontend.Symbols) == 0 {
				return fmt.Errorf("frontend.symbols is required")
			}
			if !containsString(recipe.Requires, "espeak-ng") {
				recipe.Requires = append(recipe.Requires, "espeak-ng")
			}
		case "clarvia", "tiny-voca":
			// Native adapters do not need a frontend specification.
		default:
			return fmt.Errorf("frontend kind %q is not supported", recipe.Frontend.Kind)
		}
	}
	if recipe.ThirdParty {
		if recipe.License == "" {
			return fmt.Errorf("license is required for third-party models")
		}
		if recipe.PhonemizerSource != "" {
			return fmt.Errorf("third-party models cannot install phonemizer code; use a supported frontend instead")
		}
		if !isHTTPURL(recipe.SourceURL) {
			return fmt.Errorf("source_url must be an http(s) URL for third-party models")
		}
		if recipe.LicenseURL != "" && !isHTTPURL(recipe.LicenseURL) {
			return fmt.Errorf("license_url must be an http(s) URL")
		}
		for _, file := range []string{recipe.ModelFile, recipe.ConfigFile} {
			checksum := strings.TrimSpace(recipe.Checksums[file])
			if !hex64Pattern.MatchString(checksum) {
				return fmt.Errorf("checksums[%q] must be a SHA-256 hex digest", file)
			}
		}
	}
	for path, checksum := range recipe.Checksums {
		if err := validateRelativePath(path); err != nil {
			return fmt.Errorf("checksums: %w", err)
		}
		if !hex64Pattern.MatchString(strings.TrimSpace(checksum)) {
			return fmt.Errorf("checksums[%q] must be a SHA-256 hex digest", path)
		}
	}
	if recipe.PhonemizerSource != "" {
		if !strings.HasPrefix(recipe.PhonemizerSource, "hf:") &&
			!strings.HasPrefix(recipe.PhonemizerSource, "github:") {
			return fmt.Errorf("phonemizer_source must start with hf: or github:")
		}
	}
	return nil
}

func validateRelativePath(path string) error {
	value := strings.TrimSpace(path)
	if value == "" {
		return fmt.Errorf("path is required")
	}
	if strings.Contains(value, "\\") {
		return fmt.Errorf("path %q must use forward slashes", value)
	}
	if strings.HasPrefix(value, "/") || strings.Contains(value, "..") {
		return fmt.Errorf("path %q must stay inside the model directory", value)
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." {
			return fmt.Errorf("path %q is not a clean relative path", value)
		}
	}
	return nil
}

// validateRelativePrefix is validateRelativePath for directory prefixes, which
// may end with a slash.
func validateRelativePrefix(path string) error {
	return validateRelativePath(strings.TrimSuffix(strings.TrimSpace(path), "/"))
}

func isHTTPURL(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://")
}

func contains(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// ResolveRecipe accepts a catalog id, an HF repo id, or a Hugging Face URL and
// returns the matching built-in recipe.
func ResolveRecipe(input string) (*Recipe, error) {
	recipes := Catalog()
	return ResolveRecipeIn(recipes, input)
}

// ResolveRecipeIn resolves a recipe against an explicit list.
func ResolveRecipeIn(recipes []Recipe, input string) (*Recipe, error) {
	value := strings.ToLower(strings.TrimSpace(input))
	if value == "" {
		return nil, fmt.Errorf("model id or URL is required")
	}
	value = strings.TrimSuffix(value, "/")
	for index := range recipes {
		recipe := recipes[index]
		id := strings.ToLower(recipe.ID)
		repo := strings.ToLower(recipe.HFRepo)
		switch {
		case value == id, value == repo:
			return &recipe, nil
		case strings.Contains(value, "huggingface.co/"+repo),
			strings.Contains(value, "huggingface.co/"+id):
			return &recipe, nil
		}
	}
	return nil, fmt.Errorf("no local TTS recipe is available for %q", input)
}
