# Changelog

All notable changes to this fork of PicoClaw. Versions use the
`0.1.x-fork` scheme; each version is marked with a local git tag.

Earlier versions up to `0.1.27` are recorded as git tags. This file starts
with the first batch that was built locally before tagging.

## [0.1.61] - 2026-11-27

### Added
- **Chunked read-aloud:** replies are split at sentence boundaries and played
  in sequence. The first chunk starts quickly, later chunks are prefetched, and
  Stop cancels both playback and pending synthesis.
- **Speech cache:** `~/.picoclaw/tts/cache` stores synthesized audio keyed by
  text, model, voice and speed. Entries expire after 1 hour; the folder is capped
  at 200 MB with LRU eviction. The TTS tab shows usage and can clear it.
- **Speech speed:** `voice.tts_speed` (0.5–2.0) with a slider in TTS settings.
  It applies to read-aloud, Test and play, `send_tts`, and the browser fallback.
- **Idle sidecar shutdown:** unused local TTS sidecars stop after 10 minutes
  without interrupting active requests, then restart lazily. The catalog shows
  “Installed · sleeping”.

## [0.1.60.1] - 2026-11-26

### Added
- **Language cards for Spanish and French:** `tiny-voca-es` and `tiny-voca-fr`
  are listed in Tools → Text to Speech. Selecting one and pressing Download
  installs it; nothing is installed by default.
- **Automatic eSpeak NG download:** models that declare an eSpeak front end can
  download the official eSpeak NG 1.52.0 MSI into `~/.picoclaw/tts/tools` after
  the user accepts GPL-3.0. The MSI is verified against a pinned SHA-256 digest
  and extracted per-user; no system-wide install.
- **HF link or model id:** pasting a catalog model's Hugging Face URL/id opens
  the same license and install dialog.
- The catalog reports whether eSpeak NG is managed, missing or installed.

### Changed
- The third-party manifest editor moved into a collapsed Advanced section.
- The install dialog explains the eSpeak NG download and requires a separate
  consent checkbox only when it is needed.

## [0.1.60] - 2026-11-26

### Added
- **Third-party local TTS manifest:** a versioned JSON manifest
  (`~/.picoclaw/tts/manifest.json`) can add models that use a supported front
  end. The TTS tab can edit, validate and remove it; built-in and user models
  share one schema.
- **License consent:** third-party models show source and license in the install
  dialog. The API requires `accept_license` and the registry records the
  acceptance time.
- **eSpeak front end:** the sidecar can synthesize models that declare an
  `espeak` front end. Phoneme inventory, fallback table and text rules are data
  from the manifest; model repository code is never executed.
- **Checksum verification:** third-party manifests must provide SHA-256 digests
  for the model and config files; downloads are checked before the sidecar
  starts.
- **Dependency reporting:** the catalog shows missing external tools (eSpeak NG)
  and blocks installation with a clear message.
- Spanish and French example manifests under `temp/0.1.60/` (not installed by
  default).

### Changed
- Local TTS recipes moved from Go literals to an embedded `manifest.json`.
- The sidecar version is now 3; older sidecars restart automatically.

## [0.1.59.1] - 2026-11-26

### Fixed
- Local TTS models whose model id does not contain the literal `"tts"` (such as
  `tiny-voca`) are now recognized as speech models in Tools → Text to Speech.
  The check also accepts the `picoclaw_local_tts` marker.
- The automatic TTS model scan and the “Effective provider” resolution use the
  same rule.

## [0.1.59] - 2026-11-26

### Added
- **English tiny-voca model:** the local TTS catalog now offers `n4ze3m/tiny-voca`
  (English, F2/M2 voices) alongside the Turkish Clarvia model. It installs on
  demand with its dizione phonemizer and the shared Python venv.
- **Language-aware read-aloud:** `POST /api/voice/speak` accepts a language and
  picks the installed local model for it. Turkish text uses Clarvia and English
  text uses tiny-voca; if no model supports the language, `Auto` falls back to
  the browser instead of reading with the wrong voice.
- `voice.tts_language_models` records the model installed for each language.

### Changed
- The local sidecar is now a single unified server with `clarvia` and
  `tiny-voca` adapters; existing installs are upgraded on the next use.

## [0.1.58.2] - 2026-11-26

### Fixed
- The TTS **Test and play** button now uses the model and voice currently
  selected in the form, without saving first. The override applies only to the
  test request; `config.json` is not changed.

## [0.1.58.1] - 2026-11-26

### Fixed
- Opening the Text to Speech tab no longer flashes console windows. Python
  detection commands run hidden, the result is cached for a minute, and the
  local-model query no longer refetches on browser window focus.

## [0.1.58] - 2026-11-26

### Added
- **Local TTS model manager.** Tools → Text to Speech can install the
  `clarvia-tts-tiny` Turkish model from its Hugging Face id/URL on demand. The
  model, Python venv and sidecar live under `~/.picoclaw/tts`; the release ZIP
  does not grow.
- **Python consent.** PicoClaw detects Python 3.10+ with pip/venv and asks
  before creating an environment or downloading a model. If Python is missing,
  the UI links to python.org instead of installing silently.
- **Local sidecar.** A small OpenAI-compatible `/v1/audio/speech` server is
  started on 127.0.0.1, lazily when the model is first used, with a random
  port/token, logs, and process-tree cleanup.
- **Voices.** The 10 Clarvia voices are listed in TTS settings and saved per
  model (`voice.tts_voices`). The selected voice is used by both read-aloud and
  `send_tts`.
- Automatic model entry: installing a local model adds an OpenAI-compatible
  model entry, selects it when no TTS model is configured, and restarts the
  gateway.

### Changed
- Local TTS models are resolved through a curated catalog. Unknown URLs are
  rejected by default; no arbitrary repository code is executed.

## [0.1.57] - 2026-11-26

### Added
- `POST /api/voice/speak` synthesizes arbitrary assistant text for the WebUI
  read-aloud button.
- Read-aloud settings: engine (`Auto` / PicoClaw TTS model / browser), language
  auto-detection on/off, and a manual Turkish/English override.
- When a TTS model is configured, read-aloud uses it; otherwise, and on failure
  in `Auto` mode, it falls back to the browser voice.

### Changed
- Read-aloud no longer depends on the `send_tts` agent tool; reading a reply is
  a separate action.
- The browser voice section is explicitly the fallback voice.

## [0.1.56] - 2026-11-26

### Fixed
- Read-aloud now detects the language of the message before falling back to
  the browser language. Turkish text is read with a Turkish voice (or at least
  tagged `tr-TR`) even when no speech voice was selected in TTS settings.
- When no voice is installed for the detected language, the utterance still
  carries that language instead of silently using the browser's English
  default.
- The TTS "Test and play" sample follows the selected/browser voice language
  instead of always sending the English sentence.

## [0.1.55] - 2026-11-24

### Fixed
- `thinking_level: "off"` now works on self-hosted OpenAI-compatible Qwen
  endpoints. For loopback/private API bases (llama.cpp, vLLM, LM Studio,
  Ollama and generic OpenAI-compatible providers) PicoClaw sends
  `chat_template_kwargs.enable_thinking = false` instead of a top-level
  `thinking` field the server ignores. Existing `chat_template_kwargs` keys
  configured through `extra_body` are preserved.
- Public/unknown OpenAI-compatible endpoints keep the previous behavior; no
  extra body fields are sent to them.

## [0.1.54] - 2026-11-24

### Removed
- The streaming reasoning-loop detector and its early stream cut.
- The one-shot recovery retry and its "previous attempt got stuck" note.
- The automatic retry for `finish_reason=length` with empty content.

### Kept
- `web_fetch` browser navigation headers, HTTP/2 and the explicit blocked-site
  error from 0.1.53.
- The 0.1.52 profile-agent changes (neutral identity, catalog-only
  `picoclaw-agent`, safer import tool defaults).

## [0.1.53] - 2026-11-24

### Added
- `web_fetch` sends standard browser navigation headers (`Accept`,
  `Accept-Language`, `Upgrade-Insecure-Requests`, `Sec-Fetch-*`) and enables
  HTTP/2 for page fetches. Cloudflare-protected pages such as Britannica and
  GameSpot now return their content instead of a 403 challenge.
- Blocked pages return an explicit error ("Site protection blocked the
  request...") instead of a successful empty result. This applies to both
  `plaintext` and `markdown` output.
- Streaming reasoning is checked for repetition loops. A detected loop cuts the
  stream and retries the turn once with a short "answer directly" note.
- A response that hits the output limit with no content (`finish_reason=length`)
  gets the same single recovery retry.

### Changed
- Recovery is limited to one attempt per turn; if it still fails, the user gets
  a clear message instead of the generic provider error.

## [0.1.52] - 2026-11-24

### Added
- `GET /api/agents/{id}/prompt-size` returns a read-only breakdown of the
  system prompt (identity, workspace files, skill catalog, active skills) with
  the same token heuristic as the context meter.
- The profile import dialog reads the tools and skills declared in the
  profile's `AGENT.md` and preselects them, with a warning for high-impact
  tools.

### Changed
- Agents that own a workspace (every agent except the default Pico/main agent)
  now get a neutral kernel identity line instead of "You are picoclaw, a
  helpful AI assistant". Product header, workspace layout and rules are
  unchanged.
- Those agents only see their assigned skills plus `picoclaw-agent` in the
  skill catalog. The `picoclaw-agent` body is no longer injected into their
  prompt; it stays in the catalog and can still be forced with
  `/use picoclaw-agent`.
- Importing a new profile without its own tool list now applies a safe
  baseline (file read/write, web search/fetch, message) instead of granting
  every globally enabled tool. An explicit profile list always wins.

### Fixed
- Re-importing a profile no longer overwrites its tool budget with all
  enabled tools before the user can review it.

## [0.1.51] - 2026-11-24

### Added
- Agents can be exported as a ZIP bundle (`manifest.json`, `agent.json`,
  `README.txt`, `workspace/...`) and imported into another PicoClaw or harness.
- The Agents page has an Export action and an Import agent action.

### Changed
- Deleting an agent now permanently deletes its managed workspace (sessions,
  profiles, memory, screenshots and every other file) after an explicit
  confirmation. Routing rules and subagent references are cleaned up.
- A custom workspace outside the managed agents folder is removed from config
  but left on disk, with a warning in the response.

### Security
- Agent bundle import limits ZIP size, entry count and extracted size, rejects
  path traversal and symlinks, and de-duplicates agent IDs.

## [0.1.50] - 2026-11-23

### Added
- `web_fetch` supports `offset` and returns `total_chars` plus `next_offset`
  when a response is truncated, so the agent can read the rest of a long page.

### Changed
- `web_fetch` now extracts the main content instead of stripping every tag. It
  selects `#mw-content-text`, `article`, `main`, `[role=main]` or `#content`,
  removes navigation, site headers, footers, asides, forms, hidden nodes and
  common chrome, keeps article headers, and prepends the page title. A 500
  character / 20% safety net falls back to the old extractor.
- The markdown output path uses the same cleaned content root.

## [0.1.49] - 2026-11-22

### Changed
- The context meter now counts the skills that `AGENT.md` or the agent config
  injects on every turn, so `Used` and `/context` match the real prompt instead
  of under-reporting by thousands of tokens. `/context` shows the system/skill
  split and the input budget; the ring tooltip shows the skill share.
- Context usage is pushed while a turn runs: with each tool-call message and
  again after each tool batch, so the context ring moves before the turn ends.

## [0.1.48] - 2026-11-22

### Changed
- DuckDuckGo requests are spaced by two seconds through a process-wide gate.
  A request that would wait longer than two seconds skips DuckDuckGo and falls
  through to the next provider instead of stalling the turn.
- A DuckDuckGo 202, 403 or 429 response, or a challenge page, is reported as a
  block, logged with the status, and skipped for 60 seconds.

### Fixed
- DuckDuckGo result URLs are unwrapped through the `uddg` parameter, so the
  `&rut=...` tracking tail no longer leaks into results and citations.
- DuckDuckGo titles and snippets are parsed per result block instead of being
  paired by index, so ads and uneven blocks cannot shift the snippets.

## [0.1.47] - 2026-11-22

### Changed
- `web_fetch` caps extracted text with `tools.web.fetch_max_chars` (default
  12,000). A larger `maxChars` request from the model is clamped, so one page
  cannot dominate the prompt.
- `web_search` caps each provider response with `tools.web.search_max_chars`
  (default 12,000) and marks the cut at a line boundary.
- Tavily now defaults to `search_depth: basic` (1 credit instead of 2),
  `chunks_per_source: 1` and a 600-character content clamp. All three are
  configurable. Credit usage is logged when Tavily reports it.

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
