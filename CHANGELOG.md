# Changelog

All notable changes to this fork of PicoClaw. Versions use the
`0.1.x-fork` scheme; each version is marked with a local git tag.

Earlier versions up to `0.1.27` are recorded as git tags. This file starts
with the first batch that was built locally before tagging.

## [0.1.46] - 2026-11-22

### Added
- `web_search` tries providers in order instead of picking one: free providers
  first (DuckDuckGo, SearXNG, Sogou), then paid ones (Tavily, Brave, Kagi,
  Perplexity, Gemini, Baidu, GLM). A provider error or an empty result moves on to
  the next ready provider. An explicitly selected provider stays strict.

### Fixed
- Browser launches no longer inherit the Windows `__COMPAT_LAYER` compatibility
  shim. When Windows started PicoClaw under `DetectorsAppHealth`, Chromium exited
  immediately with code 0 and the tool reported `Auto-launch failed: Chrome
  exited early`; the agent-browser child now runs with that variable removed.
- The browser permission dialog no longer reopens the request that was just
  answered. A poll response that started before the answer could put the same
  request back on screen, so the second answer failed with "expired or already
  answered". Answers are now deduplicated in the UI, and the gateway treats a
  repeated answer for the same request as the same decision.
- The generated browser action policy allowed neither `press` nor `back`, so
  keyboard keys (`Enter`, `Tab`, ...) and Back failed with "denied by policy".

## [0.1.45] - 2026-11-21

### Added
- Profile import can choose a tools allowlist; the selection is written to the
  `AGENT.md` frontmatter.
- The import preview estimates how many prompt tokens the selected skills add
  per turn.

### Changed
- Imported profiles start with only the locked runtime skill instead of every
  workspace skill.
- On import, `name`, `description`, `emoji`, `skills` and `tools` are merged into
  `AGENT.md` frontmatter. `IDENTITY.md` is read as a metadata fallback only and
  is no longer copied to the agent workspace.

### Fixed
- Remaining mojibake in `config.go` comments and messages. Adds
  `scripts/check-encoding.ps1` and `.editorconfig` to prevent recurrence.

## [0.1.44] - 2026-11-20

### Fixed
- The chat Stop button stays visible while a turn is running. Typing in the
  composer switches back to Send so an inter-message can still be sent.

### Documentation
- README refreshed for 0.1.44 and the current feature set.

## [0.1.43] - 2026-11-20

### Added
- Per-model max output tokens override (Models → Advanced). It falls back to
  the global `agents.defaults.max_tokens` when left empty.

### Fixed
- Context trimming no longer drops the whole conversation when the prompt
  exceeds the window: the summary and the last two turns are kept.
- A turn that still does not fit now fails with a clear message instead of
  calling the model with an empty history. Trimming older messages also shows
  a short notice in the chat.
- Model add/edit reports when a gateway restart is required and offers a
  "Restart now" action.
- Context-window detection no longer caches failed probes, and the fallback
  heuristic no longer follows a model's output limit.

## [0.1.42] - 2026-11-18

### Added
- Default browser allowlist for research domains (Wikipedia, Stanford
  Encyclopedia of Philosophy, Britannica, archive.org, Project Gutenberg).
  The `ask` permission policy stays in effect.

## [0.1.41] - 2026-11-18

### Added
- Stop button in the chat composer that aborts the active turn.
- Seahorse session memory: `short_grep` / `short_expand` tools and a session
  index, enabled through `agents.defaults.context_manager`.

## [0.1.40] - 2026-11-18

### Changed
- New model entries enable streaming output by default.

## [0.1.39] - 2026-11-18

### Fixed
- Deleting the open session from the sidebar clears the chat view and starts a
  new session for the same agent.

## [0.1.38] - 2026-11-17

### Fixed
- Firewall status also recognizes Windows program rules, avoiding a false
  "no rule" state.

## [0.1.37] - 2026-11-17

### Fixed
- Firewall status and helper calls no longer open a console window; firewall
  status is cached for 30 seconds.

## [0.1.36] - 2026-11-17

### Fixed
- Launcher helper processes are created without a console window.

## [0.1.35] - 2026-11-17

### Fixed
- Gateway PID checks no longer spawn PowerShell, removing a periodic console
  flash.
- Mobile chat header no longer overflows; new chat and incognito collapse into
  a menu on small screens.

## [0.1.34] - 2026-11-17

### Fixed
- Launcher retries binding for up to eight seconds during a restart so the
  replacement process can take over the port.

## [0.1.33] - 2026-11-17

### Added
- Launcher settings report `restart_required` and expose a restart action.
- Windows Firewall card with allow/remove actions (UAC).

## [0.1.32] - 2026-11-17

### Fixed
- Read-aloud text strips emoji and symbol glyphs while keeping language
  characters.

## [0.1.31] - 2026-11-17

### Fixed
- Read-aloud prefers the browser language and offers a voice picker.

## [0.1.30] - 2026-11-17

### Added
- Read assistant replies aloud with the browser speech engine (no server
  setup).
- TTS settings list only models that can speak.

## [0.1.29] - 2026-11-17

### Added
- Text to Speech settings tab (model selection, test and play) and an audio
  player for audio attachments.

## [0.1.28] - 2026-11-17

### Fixed
- Browser `ask` policy can be enabled without a site list; only `deny` with an
  empty list is rejected, and the real error is shown in the UI.

> 0.1.28 and 0.1.29 share the same commit and tag.
