package local

import (
	"fmt"
	"strings"
)

// VoiceOption is one selectable speaker exposed by a local TTS model.
type VoiceOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Recipe describes a supported local TTS model and how to install it.
type Recipe struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Language         string        `json:"language"`
	License          string        `json:"license"`
	HFRepo           string        `json:"hf_repo"`
	ModelFile        string        `json:"model_file"`
	ConfigFile       string        `json:"config_file"`
	Adapter          string        `json:"adapter"`
	ExtraPrefixes    []string      `json:"extra_prefixes,omitempty"`
	PhonemizerSource string        `json:"phonemizer_source"`
	SizeBytes        int64         `json:"size_bytes"`
	SampleRate       int           `json:"sample_rate"`
	Voices           []VoiceOption `json:"voices"`
	DefaultVoice     string        `json:"default_voice"`
}

func clarviaVoices() []VoiceOption {
	labels := []struct {
		id    string
		label string
	}{
		{"female_1", "Female 1"},
		{"female_2", "Female 2"},
		{"female_3", "Female 3"},
		{"female_4", "Female 4"},
		{"female_5", "Female 5"},
		{"male_1", "Male 1"},
		{"male_2", "Male 2"},
		{"male_3", "Male 3"},
		{"male_4", "Male 4"},
		{"male_5", "Male 5"},
	}
	voices := make([]VoiceOption, 0, len(labels))
	for _, item := range labels {
		voices = append(voices, VoiceOption{ID: item.id, Label: item.label})
	}
	return voices
}

// Catalog returns the curated local TTS models. The catalog is intentionally
// small: arbitrary repositories are not executed automatically.
func Catalog() []Recipe {
	return []Recipe{
		{
			ID:               "clarvia-tts-tiny",
			Name:             "Clarvia TTS Tiny (Turkish)",
			Language:         "tr",
			License:          "Apache-2.0",
			HFRepo:           "wite-tech/clarvia-tts-tiny",
			ModelFile:        "onnx/model.onnx",
			ConfigFile:       "config.json",
			Adapter:          "clarvia",
			PhonemizerSource: "hf:wite-tech/clarvia-phonemizer",
			SizeBytes:        22 << 20,
			SampleRate:       24000,
			Voices:           clarviaVoices(),
			DefaultVoice:     "female_1",
		},
		{
			ID:               "tiny-voca",
			Name:             "tiny-voca (English)",
			Language:         "en",
			License:          "Apache-2.0",
			HFRepo:           "n4ze3m/tiny-voca",
			ModelFile:        "onnx/model.onnx",
			ConfigFile:       "config.json",
			Adapter:          "tiny-voca",
			ExtraPrefixes:    []string{"phonemizer/"},
			PhonemizerSource: "github:n4ze3m/dizione",
			SizeBytes:        28 << 20,
			SampleRate:       24000,
			Voices: []VoiceOption{
				{ID: "F2", Label: "F2 (female)"},
				{ID: "M2", Label: "M2 (male)"},
			},
			DefaultVoice: "F2",
		},
	}
}

// ResolveRecipe accepts a catalog id, an HF repo id, or a Hugging Face URL and
// returns the matching recipe.
func ResolveRecipe(input string) (*Recipe, error) {
	value := strings.ToLower(strings.TrimSpace(input))
	if value == "" {
		return nil, fmt.Errorf("model id or URL is required")
	}
	value = strings.TrimSuffix(value, "/")
	for _, recipe := range Catalog() {
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
