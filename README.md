# PicoClaw — Windows fork

A Windows-first, WebUI-centered fork of [PicoClaw](https://github.com/sipeed/picoclaw)
by Sipeed. Upstream is MIT-licensed; this fork keeps the same license and adds
desktop usability, security hardening, agent management, and local document
tools.

- **Version:** `0.1.58-fork`
- **Upstream:** https://github.com/sipeed/picoclaw
- **Fork:** https://github.com/techunreal/picoclaw
- **License:** MIT (see `LICENSE` and `NOTICE.md`)

> This is not an official Sipeed release. Report fork-specific issues on the
> fork repository, not upstream.

## What's new in 0.1.58-fork

- **Local TTS models on demand:** Tools → Text to Speech can install the
  Turkish `clarvia-tts-tiny` model from its Hugging Face URL/id. Model files,
  the Python venv and the local sidecar live under `~/.picoclaw/tts`; the ZIP
  stays small. Python 3.10+ is detected and the user is asked before anything is
  created or downloaded.
- **Voice selection:** the model's 10 voices are listed in TTS settings and
  saved per model. Read-aloud and `send_tts` use the selected voice.
- **Safe local runtime:** the sidecar listens on 127.0.0.1 with a random token,
  starts lazily, logs to `~/.picoclaw/tts/logs` and is stopped with its process
  tree. Only curated models are accepted; unknown URLs are rejected.

- **Read-aloud uses the TTS model:** the speaker button in chats now synthesizes
  the reply with the configured TTS model when one is ready, and falls back to
  the browser voice in `Auto` mode. Reading a reply no longer requires the
  `send_tts` tool.
- **Read-aloud settings:** choose the engine (`Auto` / TTS model / browser),
  turn language detection on or off, and override the language with Turkish or
  English. A manual language always wins.
- **Separate fallback voice:** the browser voice selector is now explicitly the
  fallback used when no TTS model is available.

- **Read-aloud speaks the right language:** the message language is checked
  before the browser language, so Turkish replies are read with a Turkish voice
  (or tagged `tr-TR`) even when no speech voice was chosen in TTS settings. The
  TTS test sample also follows the selected voice language instead of always
  being English.

- **Thinking off works on local models:** setting a model's **Thinking Level**
  to `off` now disables Qwen thinking on self-hosted OpenAI-compatible servers
  (llama.cpp, vLLM and similar) by sending the chat-template flag. No manual
  `Extra Body` JSON is needed. Public/unknown endpoints are left unchanged.

- **Cloudflare-protected pages work again:** `web_fetch` sends normal browser
  navigation headers and enables HTTP/2 for page fetches. Britannica and
  GameSpot return content instead of a 403 challenge; sites that still block the
  request get an explicit "Site protection blocked the request" error instead
  of a successful empty result.
- **Reasoning-loop guard removed:** the 0.1.53 detector, early stream cut and
  one-shot retry are gone. Responses stream and finish normally; the model may
  draft a passage more than once without the turn being interrupted.

- **Profile agents keep their own identity:** an agent with its own workspace
  no longer receives "You are picoclaw, a helpful AI assistant." Its identity
  comes from `AGENT.md` and `SOUL.md`, and the neutral kernel line says so.
  The default Pico/main agent is unchanged.
- **Smaller persona prompt:** profile agents see only their assigned skills
  plus `picoclaw-agent` in the catalog. The 33 KB `picoclaw-agent` body is not
  injected any more; it can still be read or forced with `/use picoclaw-agent`.
  For the sample Sokrates agent this cuts the system prompt from ~48.5k to
  ~9.9k characters (about 19.4k to 4.1k estimated tokens).
- **Prompt size endpoint:** `GET /api/agents/{id}/prompt-size` shows the
  identity, workspace, catalog and active-skill shares with the same token
  heuristic as the context meter.
- **Safer profile tool defaults:** import preselects the tools declared in
  `AGENT.md`; without a declaration it uses a safe baseline (file read/write,
  web search/fetch, message) and warns about exec, browser, cron, spawn and
  skill installation.

- **Permanent agent delete:** the Agents page asks for confirmation and then
  removes the agent from config and deletes its managed workspace, including
  sessions, profiles, memory and screenshots.
- **Agent bundles:** Export downloads a ZIP with `manifest.json`, `agent.json`,
  `README.txt` and the whole `workspace/`. Import agent restores it on this or
  another PicoClaw (or another harness via `AGENT.md`/`SOUL.md`).

- **Readable web pages:** `web_fetch` now selects the main content root and
  removes navigation, site headers, footers, asides, forms, hidden nodes and
  common chrome, then prepends the page title. A 500-character / 20% safety net
  falls back to the old extractor.
- **Long pages:** `web_fetch` accepts `offset` and returns `total_chars` and
  `next_offset`, so an agent can read the rest of a page in a second call.
  Markdown output uses the same cleaned content root.

- **Honest context meter:** usage now counts the skills injected from
  `AGENT.md` or the agent config, so `Used` and `/context` match the real
  prompt. `/context` shows the system/skill split and the input budget, and the
  ring tooltip shows the skill share.
- **Live context updates:** each tool-call message carries usage, and the open
  stream is refreshed after every tool batch, so the context ring moves while a
  turn is still running.

- **DuckDuckGo resilience:** requests are spaced two seconds apart through a
  process-wide gate. A 202/403/429 response or a challenge page is reported as a
  block, logged with its status, and skipped for 60 seconds, so the tool falls
  through to the next provider instead of wasting turns.
- **Cleaner DuckDuckGo results:** result URLs are unwrapped through the `uddg`
  parameter (`&rut=...` tracking tails are gone), and titles/snippets are parsed
  from their own result block instead of being paired by index.

- **Smaller web results:** `web_fetch` treats its `maxChars` as a hard ceiling
  (`tools.web.fetch_max_chars`, default 12,000) and `web_search` caps each
  provider response (`tools.web.search_max_chars`, default 12,000), so research
  turns no longer fill the context with a few long pages.
- **Cheaper Tavily:** `search_depth` now defaults to `basic` (1 credit instead of
  2), with `chunks_per_source: 1` and a 600-character per-result clamp. All three
  are configurable under `tools.web.tavily`.
- **Browser launches fixed under Windows compatibility shims:** PicoClaw no
  longer passes `__COMPAT_LAYER` to agent-browser. When Windows started the app
  under `DetectorsAppHealth`, Chromium exited instantly with code 0 and the tool
  showed `Auto-launch failed: Chrome exited early`.
- **Browser permission dialog:** an answered request is not shown a second time,
  and repeated answers are treated as the same decision instead of failing with
  "expired or already answered". The action policy now also allows `press` and
  `back`, so keyboard keys and Back work.
- **Search fallback:** `web_search` tries free providers first (DuckDuckGo,
  SearXNG, Sogou) and then paid ones (Tavily, Brave, Kagi, Perplexity, Gemini,
  Baidu, GLM). Errors and empty results fall through to the next ready provider;
  an explicitly selected provider is used alone.
- **Safer profile import:** an imported profile starts with only the locked
  runtime skill instead of every workspace skill. The preview shows the estimated
  prompt cost and a tools allowlist, and writes `name`, `description`, `emoji`,
  `skills` and `tools` into `AGENT.md` frontmatter. `IDENTITY.md` is a metadata
  fallback only.
- **Per-model output budget:** `max_tokens` can be set per model and overrides
  `agents.defaults.max_tokens`. A model edit that an agent uses reports
  `restart_required` and offers a Restart action.
- **Context safety:** when a prompt no longer fits, trimming keeps the summary
  and the last two turns, and a turn that still does not fit fails with a clear
  message instead of calling the model with an empty history. A short notice
  appears when older messages are trimmed.
- **Stop stays visible:** the composer shows Stop for the whole turn; typing in
  the composer switches back to Send so an inter-message can still be sent.
- **Agents:** per-agent models and workspaces (`agents/<id>/`), agent and session
  menus (hide, reorder, pin, rename, delete), and a session list that scans every
  agent workspace.
- **Chat:** Read aloud with browser speech and voice selection, a Text to Speech
  settings tab, an audio player for attachments, incognito chats and a
  per-session memory switch.
- **Browser:** a research allowlist (Wikipedia, Stanford Encyclopedia of
  Philosophy, Britannica, archive.org, Project Gutenberg) plus Ask/Allow/Deny for
  other domains.
- **Sessions:** optional Seahorse session memory with `short_grep` /
  `short_expand`.
- **Launcher:** restart-required warning with a Restart button, a Windows
  Firewall card, and helper processes that never open a console window.
- **Models:** OpenCode Go/Zen providers, per-model context window, streaming by
  default for new models, and stable name-based edit/delete.
- **Stability:** context-window probing caches only successes; firewall and
  agent-browser helper calls run without console windows.

## What this fork adds

### WebUI and daily use

- First-run setup with a dashboard password; persistent session across launcher
  restarts.
- Chat with model picker, memory toggle per session, incognito chats, file
  attachments (PDF, text, images, audio), and drag-and-drop.
- Stop button while a turn is running; Read aloud for assistant replies.
- Sidebar with agent groups, session pin/rename/delete, and a running spinner.
- Dashboard for models, credentials, channels, usage, logs, and configuration.
- Backup & restore as a ZIP with manifest and safe restore.
- Scheduled tasks (cron) with validation, run-now, and enable/disable.
- Guided tour with fork-specific steps; can be restarted from the header.

### Models and providers

- Multiple API keys per model with failover and masked display.
- Enable/disable models without deleting them.
- Custom / OpenAI-compatible providers with required base URL.
- Provider-aware connection tests, proxy support, catalog fetch.
- Per-model `context_window` and `max_tokens`; per-agent model overrides.
- Stable name-based edit/delete; duplicate names rejected.
- Model rename in the editor, including automatic updates to agent and fallback
  references.
- Usage and estimated cost page per model/session.
- OpenCode Go and Zen ready-made providers.

### Agents and skills

- Multiple agents with separate workspaces, models, skills, subagent
  permissions, and channel routing.
- Per-agent personality files (`AGENT.md`, `SOUL.md`, `IDENTITY.md`) with
  revision history, backups, and conflict detection.
- Shared skills root: one copy in the main workspace, readable by every agent;
  `install_skill` writes there.
- Subagent master switch, asynchronous `spawn`, live monitoring, cancel.
- MCP server management and connection test.
- Skills marketplace (ClawHub), GitHub URL install, security badges, updates,
  local editing with revision/conflict handling.
- 13 bundled skills on first setup:
  `weather`, `agent-browser`, `picoclaw-agent`, `skill-creator`, `humanizer`,
  `brainstorming`, `skill-safety-check`, `repo-research`, `deep-research`,
  `office-word`, `office-excel`, `office-powerpoint`, `office-pdf`.

### Office Pack

The Windows ZIP includes a portable Python 3.13 runtime and pinned libraries, so
the agent can create and read Word, Excel, PowerPoint, and PDF files without
Microsoft Office, Adobe Acrobat, or a system Python install.

- Word: documents with headings, paragraphs, lists, tables.
- Excel: workbooks with values, formulas, styles, widths.
- PowerPoint: decks with title/content slides, notes, images.
- PDF: create, extract text, merge, split, rotate, list/fill forms.
- Limitations: no OCR; complex Office templates, tracked changes, and
  animations are only partially supported.
- Runtime lives in `tools\python`; no background process and no idle RAM use.

### Security and reliability defaults

- `allow_remote=false` by default for remote exec and browser.
- Workspace isolation with path restrictions; per-agent workspaces.
- Browser domain allowlist and Ask/Allow/Deny policy for unknown sites.
- Secret filtering in logs; `.security.yml` for credentials.
- Dashboard session secret persisted in `%USERPROFILE%\.picoclaw\launcher-session.key`,
  rotated on password change and sign-out.
- Broken/BOM `config.json` no longer crashes gateway status; the API returns a
  readable `config_error`.
- Deterministic test suite; no 10-minute shell-test hangs.

## Quick start (Windows)

1. Extract the release ZIP (for example
   `PicoClaw-Windows-x86_64-0.1.58-fork.zip`) to a folder such as
   `C:\Users\<you>\PicoClaw`.
2. Run `picoclaw-launcher.exe`.
3. Open `http://localhost:18800` if the browser does not open automatically.
4. Create the dashboard password on first run.
5. Add a model and API key in **Models**.
6. Click **Start Gateway**.
7. Chat. Open **Agent â†’ Skills** to see the bundled skills.

`picoclaw.exe` is the gateway/engine; keep it next to the launcher. The launcher
finds it automatically.

### Data directory

Everything lives under `%USERPROFILE%\.picoclaw` unless `PICOCLAW_HOME` is set:

```text
.picoclaw/
  config.json
  .security.yml
  launcher-auth.db
  launcher-session.key
  workspace/              main agent (Pico): AGENT.md, SOUL.md, memory/, skills/, tools/
  workspace/sessions/     main agent sessions (seahorse.db when enabled)
  agents/<id>/            per-agent workspace: AGENT.md, SOUL.md, memory/, sessions/, tools/
  logs/
  backups/
```

To reset, stop the launcher and remove that folder. Back up first if you care
about models, keys, sessions, skills, or memory.

## Build from source

Requirements: Go 1.25+, Node.js 20.19+/22.13+, pnpm 10+, Windows x64 for the
Office/agent-browser bundle.

```powershell
cd web/frontend
pnpm install --frozen-lockfile
pnpm build:backend
cd ../..
go build -tags "goolm,stdjson" -o build/picoclaw.exe ./cmd/picoclaw
go build -tags "goolm,stdjson" -o build/picoclaw-launcher.exe ./web/backend
```

Release build with version metadata, Office runtime, ZIP, and SHA256:

```powershell
powershell -File scripts/build-release.ps1 -Version 0.1.58-fork
```

Lean build without the Office runtime:

```powershell
powershell -File scripts/build-release.ps1 -Version 0.1.58-fork -SkipOfficeRuntime
```

## Tests

```powershell
go test -tags "goolm,stdjson" ./... -count=1 -timeout 300s
```

## Package status

- Packages 0, A, B, C: complete.
- Package D (swarm, plugin ecosystem, mobile control): deferred.
- Package E.1: resilience, persistence, model references, deterministic tests,
  curated skills, Office Pack.
- Package E.2: release, clean install, README, roadmap, fork push.
- 0.1.4-0.1.58: WebUI, agents, models, profile import, TTS/read-aloud, browser
  allowlist, launcher/firewall, context safety. See [`CHANGELOG.md`](CHANGELOG.md).
- Portable/USB mode (E.3): future idea.

See [`docs/FORK-ROADMAP.md`](docs/FORK-ROADMAP.md) for the detailed roadmap and
[`docs/OFFICE-PACK.md`](docs/OFFICE-PACK.md) for Office details.

## Known limitations

- Windows x64 build is the primary target; macOS verification is deferred.
- This build is not code-signed; Windows SmartScreen may show an "unknown publisher"
  warning on first run. Choose More info -> Run anyway.
- Context-window detection depends on what the model endpoint advertises. Set
  `context_window` explicitly for self-hosted models if detection is unreliable.
- The TTS settings tab supports OpenAI-compatible `/v1/audio/speech` providers
  and MiMo; ElevenLabs is supported for speech-to-text only.
- No OCR for scanned PDFs.
- Complex Office documents may lose advanced formatting.
- `openpyxl` does not calculate formulas; use Excel/LibreOffice for recalculation.
- The fork is not audited for production; do not expose it to the public internet
  without TLS, strong passwords, and network controls.

## Credits

PicoClaw upstream: Sipeed and contributors (MIT). Bundled third-party skills and
licenses are documented in [`docs/THIRD-PARTY-SKILLS.md`](docs/THIRD-PARTY-SKILLS.md).
Office runtime licenses are in [`docs/OFFICE-PACK.md`](docs/OFFICE-PACK.md).
