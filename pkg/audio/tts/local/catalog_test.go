package local

import "testing"

func TestResolveRecipe(t *testing.T) {
	forms := []string{
		"clarvia-tts-tiny",
		"wite-tech/clarvia-tts-tiny",
		"https://huggingface.co/wite-tech/clarvia-tts-tiny",
		"https://huggingface.co/wite-tech/clarvia-tts-tiny/",
	}
	for _, form := range forms {
		recipe, err := ResolveRecipe(form)
		if err != nil {
			t.Fatalf("ResolveRecipe(%q) error = %v", form, err)
		}
		if recipe.ID != "clarvia-tts-tiny" {
			t.Fatalf("ResolveRecipe(%q) id = %q", form, recipe.ID)
		}
	}
	if _, err := ResolveRecipe("https://example.com/unknown"); err == nil {
		t.Fatal("unknown model should be rejected")
	}
}

func TestClarviaCatalogVoices(t *testing.T) {
	recipe, err := ResolveRecipe("clarvia-tts-tiny")
	if err != nil {
		t.Fatal(err)
	}
	if len(recipe.Voices) != 10 {
		t.Fatalf("voices = %d, want 10", len(recipe.Voices))
	}
	if recipe.DefaultVoice != "female_1" {
		t.Fatalf("default voice = %q", recipe.DefaultVoice)
	}
	if recipe.Language != "tr" || recipe.SampleRate != 24000 {
		t.Fatalf("recipe metadata = %#v", recipe)
	}
}
