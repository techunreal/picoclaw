package local

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindEspeakExe(t *testing.T) {
	root := t.TempDir()
	if got := findEspeakExe(root); got != "" {
		t.Fatalf("findEspeakExe(empty) = %q", got)
	}
	exe := filepath.Join(root, "eSpeak NG", "espeak-ng.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findEspeakExe(root); got != exe {
		t.Fatalf("findEspeakExe() = %q, want %q", got, exe)
	}
}

func TestManagedEspeakIsPreferred(t *testing.T) {
	manager := NewManager(t.TempDir())
	exe := filepath.Join(manager.espeakDir(), "eSpeak NG", "espeak-ng.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, managed := manager.espeakAvailable()
	if !managed || got != exe {
		t.Fatalf("espeakAvailable() = %q managed=%v, want %q managed=true", got, managed, exe)
	}
}

func TestEspeakEnvSetsDataDirectory(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "espeak-ng.exe")
	env := espeakEnv(exe)
	want := "ESPEAK_DATA_PATH=" + filepath.Join(dir, "espeak-ng-data")
	found := false
	for _, item := range env {
		if item == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("environment does not contain %q", want)
	}
}
