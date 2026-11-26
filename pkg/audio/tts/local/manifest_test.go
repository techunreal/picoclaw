package local

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinManifestIsValid(t *testing.T) {
	manifest, err := BuiltinManifest()
	if err != nil {
		t.Fatalf("BuiltinManifest() error = %v", err)
	}
	if manifest.Version != manifestVersion {
		t.Fatalf("version = %d", manifest.Version)
	}
	if len(manifest.Models) != 4 {
		t.Fatalf("models = %d, want 4", len(manifest.Models))
	}
	if manifest.Models[0].ID != "clarvia-tts-tiny" || manifest.Models[1].ID != "tiny-voca" {
		t.Fatalf("models = %#v", manifest.Models)
	}
	if manifest.Models[2].ID != "tiny-voca-es" || manifest.Models[3].ID != "tiny-voca-fr" {
		t.Fatalf("models = %#v", manifest.Models)
	}
}

func validUserManifest() Manifest {
	return Manifest{
		Version: manifestVersion,
		Models: []Recipe{{
			ID:         "tiny-voca-es-user",
			Name:       "tiny-voca (Spanish)",
			Language:   "es",
			License:    "Apache-2.0",
			LicenseURL: "https://huggingface.co/marcosmaceo/tiny-voca-es",
			SourceURL:  "https://huggingface.co/marcosmaceo/tiny-voca-es",
			HFRepo:     "marcosmaceo/tiny-voca-es",
			ModelFile:  "tiny-voca-es.onnx",
			ConfigFile: "tiny-voca-es.json",
			Adapter:    "espeak",
			SizeBytes:  22 << 20,
			SampleRate: 24000,
			Voices: []VoiceOption{
				{ID: "F2", Label: "F2 (female)"},
				{ID: "M2", Label: "M2 (male)"},
			},
			DefaultVoice: "F2",
			ThirdParty:   true,
			Checksums: map[string]string{
				"tiny-voca-es.onnx": strings.Repeat("a", 64),
				"tiny-voca-es.json": strings.Repeat("b", 64),
			},
			Frontend: &FrontendSpec{
				Kind:        "espeak",
				EspeakVoice: "es",
				Symbols: []string{
					"<pad>", "<bos>", "<eos>", "<sp>", "<unk>",
					",", ".", "?", "!", "a", "e", "i", "o", "u", "k",
				},
				Vowels: []string{"a", "e", "i", "o", "u"},
			},
		}},
	}
}

func TestParseManifestAcceptsUserModel(t *testing.T) {
	raw, err := json.Marshal(validUserManifest())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ParseManifest(raw)
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}
	recipe := manifest.Models[0]
	if recipe.ID != "tiny-voca-es-user" || !recipe.ThirdParty {
		t.Fatalf("recipe = %#v", recipe)
	}
	if !containsString(recipe.Requires, "espeak-ng") {
		t.Fatalf("requires = %#v, want espeak-ng", recipe.Requires)
	}
	if recipe.Frontend == nil || recipe.Frontend.EspeakVoice != "es" {
		t.Fatalf("frontend = %#v", recipe.Frontend)
	}
}

func TestParseManifestRejections(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{"version", func(m *Manifest) { m.Version = 99 }, "unsupported manifest version"},
		{"empty", func(m *Manifest) { m.Models = nil }, "no models"},
		{"id", func(m *Manifest) { m.Models[0].ID = "Bad ID" }, "id must match"},
		{"language", func(m *Manifest) { m.Models[0].Language = "spanish" }, "language"},
		{"adapter", func(m *Manifest) { m.Models[0].Adapter = "custom" }, "not supported"},
		{"model path", func(m *Manifest) { m.Models[0].ModelFile = "../escape.onnx" }, "model_file"},
		{"config path", func(m *Manifest) { m.Models[0].ConfigFile = "sub/../../x" }, "config_file"},
		{"license", func(m *Manifest) { m.Models[0].License = "" }, "license is required"},
		{"source", func(m *Manifest) { m.Models[0].SourceURL = "ftp://example.com" }, "source_url"},
		{"checksum missing", func(m *Manifest) { delete(m.Models[0].Checksums, "tiny-voca-es.onnx") }, "checksums"},
		{"checksum short", func(m *Manifest) { m.Models[0].Checksums["tiny-voca-es.onnx"] = "abc" }, "SHA-256"},
		{"voices", func(m *Manifest) { m.Models[0].Voices = nil }, "voices are required"},
		{"default voice", func(m *Manifest) { m.Models[0].DefaultVoice = "F9" }, "default_voice"},
		{"frontend kind", func(m *Manifest) { m.Models[0].Frontend.Kind = "clarvia" }, "does not match adapter"},
		{"frontend symbols", func(m *Manifest) { m.Models[0].Frontend.Symbols = nil }, "frontend.symbols"},
		{"dependency", func(m *Manifest) { m.Models[0].Requires = []string{"ffmpeg"} }, "dependency"},
		{"duplicate", func(m *Manifest) { m.Models = append(m.Models, m.Models[0]) }, "duplicate"},
		{"reserved", func(m *Manifest) { m.Models[0].ID = "clarvia-tts-tiny" }, "reserved"},
		{"not third party", func(m *Manifest) { m.Models[0].ThirdParty = false }, "third_party"},
		{"phonemizer code", func(m *Manifest) { m.Models[0].PhonemizerSource = "github:evil/pkg" }, "phonemizer"},
		{"size", func(m *Manifest) { m.Models[0].SizeBytes = maxModelSize + 1 }, "size_bytes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validUserManifest()
			test.mutate(&manifest)
			raw, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			_, err = ParseManifest(raw)
			if err == nil {
				t.Fatal("manifest should be rejected")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want it to contain %q", err, test.want)
			}
		})
	}
}

func TestParseManifestRejectsUnknownField(t *testing.T) {
	if _, err := ParseManifest([]byte(`{"version":1,"models":[],"evil":true}`)); err == nil {
		t.Fatal("unknown field should be rejected")
	}
}

func TestManagerUserManifestRoundTrip(t *testing.T) {
	manager := NewManager(t.TempDir())
	raw, err := json.Marshal(validUserManifest())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SaveUserManifest(raw); err != nil {
		t.Fatalf("SaveUserManifest() error = %v", err)
	}
	recipes, err := manager.Catalog()
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	if len(recipes) != 5 {
		t.Fatalf("catalog = %d, want 5", len(recipes))
	}
	recipe, err := manager.ResolveRecipe("tiny-voca-es-user")
	if err != nil {
		t.Fatalf("ResolveRecipe() error = %v", err)
	}
	if recipe.ID != "tiny-voca-es-user" {
		t.Fatalf("recipe = %#v", recipe)
	}
	if raw, err := manager.UserManifestJSON(); err != nil || !strings.Contains(raw, "tiny-voca-es") {
		t.Fatalf("UserManifestJSON() = %q, %v", raw, err)
	}
	if err := manager.ClearUserManifest(); err != nil {
		t.Fatal(err)
	}
	recipes, err = manager.Catalog()
	if err != nil {
		t.Fatalf("Catalog() after clear error = %v", err)
	}
	if len(recipes) != 4 {
		t.Fatalf("catalog after clear = %d, want 4", len(recipes))
	}
}

func TestEspeakFrontendGolden(t *testing.T) {
	python, prefix := pythonCommand()
	if python == "" {
		t.Skip("python is not available")
	}
	script := filepath.Join(t.TempDir(), "server.py")
	if err := os.WriteFile(script, []byte(serverScript), 0o644); err != nil {
		t.Fatal(err)
	}
	suite, err := filepath.Abs(filepath.Join("testdata", "espeak_frontend.json"))
	if err != nil {
		t.Fatal(err)
	}
	args := append(append([]string{}, prefix...), script, "--frontend-test", suite)
	cmd := exec.Command(python, args...)
	output, runErr := cmd.CombinedOutput()
	var result struct {
		OK       bool `json:"ok"`
		Failures []struct {
			Name     string `json:"name"`
			Actual   any    `json:"actual"`
			Expected any    `json:"expected"`
			Error    string `json:"error"`
		} `json:"failures"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("frontend test output is not JSON: %v\n%s", err, output)
	}
	if runErr != nil || !result.OK {
		t.Fatalf("frontend test failed: %v\n%s", runErr, output)
	}
}

func pythonCommand() (string, []string) {
	if path, err := exec.LookPath("python"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("python3"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("py"); err == nil {
		return path, []string{"-3"}
	}
	return "", nil
}
