# PicoClaw Ã¢â‚¬â€ Windows fork

A Windows-first, WebUI-centered fork of [PicoClaw](https://github.com/sipeed/picoclaw)
by Sipeed. Upstream is MIT-licensed; this fork keeps the same license and adds
desktop usability, security hardening, agent management, and local document
tools.

- **Version:** `0.1.3-fork`
- **Upstream:** https://github.com/sipeed/picoclaw
- **Fork:** https://github.com/techunreal/picoclaw
- **License:** MIT (see `LICENSE` and `NOTICE.md`)

> This is not an official Sipeed release. Report fork-specific issues on the
> fork repository, not upstream.

## What's new in 0.1.3-fork

- **Per-model context window:** each model entry can store its input context size
  (`context_window`), editable in the model sheet. Catalog entries fetched from
  providers that advertise `context_length` keep that value and reuse it when
  the model is added.
- **Context window auto-detection:** self-hosted/compatible endpoints advertise
  their context length in `/models`; the chat budget now uses it instead of the
  `4 x max_tokens` heuristic.
- **Connection test tolerance:** an endpoint that answers `/models` no longer
  fails with "unreachable" when the configured model id uses a deployment alias.
- **Tour:** the agent step highlights profiles/chats and explains the settings
  menu.
- **Exec safety:** failing commands are redacted from tool output and logs.
- Built with the GUI subsystem, so no console/terminal window is opened.

- **Chats sidebar:** settings moved behind a gear; the sidebar lists agent
  profiles with their recent chats. History button removed.
- **Models page:** compact single-row cards, virtual multi-key rows hidden,
  "Hide not configured" filter, fetch dialog no longer selects everything.
- **Memory switches apply immediately:** the launcher pushes them into the
  running gateway instead of only editing the config file.
- **Browser settings apply on save:** the gateway restarts automatically so
  allowlist and permission policy take effect; suspicious marketplace skills ask
  for confirmation before install.
- **Web search:** DuckDuckGo is the default provider (Sogou disabled), which fixes
  empty research results outside China.
- **Launcher:** no console window on start, fewer stale gateway states, and
  deleting an agent no longer fails on delegation allowlists.
- **Exec safety:** failing commands are redacted from tool output and logs.


## What this fork adds

### WebUI and daily use

- First-run setup with a dashboard password; persistent session across launcher
  restarts.
- Chat with model picker, memory toggle per session, incognito chats, file
  attachments (PDF, text, images, audio), and drag-and-drop.
- Dashboard for models, credentials, channels, usage, logs, and configuration.
- Backup & restore as a ZIP with manifest and safe restore.
- Scheduled tasks (cron) with validation, run-now, and enable/disable.
- Guided tour with fork-specific steps; can be restarted from the header.

### Models and providers

- Multiple API keys per model with failover and masked display.
- Enable/disable models without deleting them.
- Custom / OpenAI-compatible providers with required base URL.
- Provider-aware connection tests, proxy support, catalog fetch.
- Stable name-based edit/delete; duplicate names rejected.
- Model rename in the editor, including automatic updates to agent and fallback
  references.
- Usage and estimated cost page per model/session.

### Agents and skills

- Multiple agents with separate workspaces, models, skills, subagent
  permissions, and channel routing.
- Subagent master switch, asynchronous `spawn`, live monitoring, cancel.
- Per-agent personality files (`AGENT.md`, `SOUL.md`, Ã¢â‚¬Â¦) with revision history,
  backups, and conflict detection.
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

1. Extract `PicoClaw-Windows-x86_64-0.1.3-fork.zip` to a folder such as
   `C:\Users\<you>\PicoClaw`.
2. Run `picoclaw-launcher.exe`.
3. Open `http://localhost:18800` if the browser does not open automatically.
4. Create the dashboard password on first run.
5. Add a model and API key in **Models**.
6. Click **Start Gateway**.
7. Chat. Open **Agent Ã¢â€ â€™ Skills** to see the bundled skills.

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
  workspace/
    AGENT.md, SOUL.md, USER.md, HEARTBEAT.md, memory/, skills/, tools/
  sessions/
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
powershell -File scripts/build-release.ps1 -Version 0.1.3-fork
```

Lean build without the Office runtime:

```powershell
powershell -File scripts/build-release.ps1 -Version 0.1.3-fork -SkipOfficeRuntime
```

## Tests

```powershell
go test -tags "goolm,stdjson" ./... -count=1 -timeout 300s
```

Linux (Docker, `/integration` excluded):

```bash
docker run --rm -v "$PWD:/src" -w /src golang:1.27 \
  sh -c 'go test -tags "goolm,stdjson" $(go list ./... | grep -v /integration)'
```

Office Pack smoke test:

```powershell
powershell -File scripts/test-office-pack.ps1 -RuntimeDir build/office-runtime
```

## Package status

- Packages 0, A, B, C: complete.
- Package D (swarm, plugin ecosystem, mobile control): deferred.
- Package E.1: resilience, persistence, model references, deterministic tests,
  curated skills, Office Pack.
- Package E.2: release, clean install, README, roadmap, fork push.
- Portable/USB mode (E.3): future idea.

See [`docs/FORK-ROADMAP.md`](docs/FORK-ROADMAP.md) for the detailed roadmap and
[`docs/OFFICE-PACK.md`](docs/OFFICE-PACK.md) for Office details.

## Known limitations

- Windows x64 build is the primary target; macOS verification is deferred.
- This build is not code-signed; Windows SmartScreen may show an "unknown publisher"
  warning on first run. Choose More info -> Run anyway.
- No OCR for scanned PDFs.
- Complex Office documents may lose advanced formatting.
- `openpyxl` does not calculate formulas; use Excel/LibreOffice for recalculation.
- The fork is not audited for production; do not expose it to the public internet
  without TLS, strong passwords, and network controls.

## Credits

PicoClaw upstream: Sipeed and contributors (MIT). Bundled third-party skills and
licenses are documented in [`docs/THIRD-PARTY-SKILLS.md`](docs/THIRD-PARTY-SKILLS.md).
Office runtime licenses are in [`docs/OFFICE-PACK.md`](docs/OFFICE-PACK.md).
