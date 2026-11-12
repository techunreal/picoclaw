// Package constants provides shared constants across the codebase.
package constants

// internalChannels defines channels that are used for internal communication
// and should not be exposed to external users or recorded as last active channel.
var internalChannels = map[string]struct{}{
	"cli":      {},
	"system":   {},
	"subagent": {},
}

// IsInternalChannel returns true if the channel is an internal channel.
func IsInternalChannel(channel string) bool {
	_, found := internalChannels[channel]
	return found
}

// localUIChannels are served by the local web UI. They are not internal
// channels (replies must still be delivered and sessions recorded), but they
// are local from the operator's point of view and may use local-only
// capabilities such as shell execution.
var localUIChannels = map[string]struct{}{
	"pico": {},
}

// IsLocalUIChannel reports whether channel is the local web UI channel.
func IsLocalUIChannel(channel string) bool {
	_, found := localUIChannels[channel]
	return found
}

// IsLocalCommandChannel reports whether channel may use local-only command
// capabilities such as shell execution and command scheduling.
//
// This includes internal channels (cli, system, subagent) and the local web
// UI (pico), but deliberately excludes external chat channels such as
// Telegram, Discord, Slack, and WhatsApp. Those require an explicit
// tools.exec.allow_remote opt-in.
func IsLocalCommandChannel(channel string) bool {
	return IsInternalChannel(channel) || IsLocalUIChannel(channel)
}
