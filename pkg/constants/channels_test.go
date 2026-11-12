package constants

import "testing"

func TestIsInternalChannel(t *testing.T) {
	for _, channel := range []string{"cli", "system", "subagent"} {
		if !IsInternalChannel(channel) {
			t.Errorf("IsInternalChannel(%q) = false, want true", channel)
		}
	}
	for _, channel := range []string{"", "pico", "pico_client", "telegram", "discord"} {
		if IsInternalChannel(channel) {
			t.Errorf("IsInternalChannel(%q) = true, want false", channel)
		}
	}
}

func TestIsLocalUIChannel(t *testing.T) {
	if !IsLocalUIChannel("pico") {
		t.Error(`IsLocalUIChannel("pico") = false, want true`)
	}
	for _, channel := range []string{"", "pico_client", "cli", "telegram", "discord"} {
		if IsLocalUIChannel(channel) {
			t.Errorf("IsLocalUIChannel(%q) = true, want false", channel)
		}
	}
}

func TestIsLocalCommandChannel(t *testing.T) {
	for _, channel := range []string{"cli", "system", "subagent", "pico"} {
		if !IsLocalCommandChannel(channel) {
			t.Errorf("IsLocalCommandChannel(%q) = false, want true", channel)
		}
	}
	for _, channel := range []string{"", "pico_client", "telegram", "discord", "slack", "whatsapp"} {
		if IsLocalCommandChannel(channel) {
			t.Errorf("IsLocalCommandChannel(%q) = true, want false", channel)
		}
	}
}
