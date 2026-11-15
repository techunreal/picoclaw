# PicoClaw fork roadmap

Last updated: 2026-11-15
Fork version: `0.1.0-fork`
Upstream: https://github.com/sipeed/picoclaw (MIT)

This fork is Windows-first and WebUI-centered. Priorities: security and
stability first, then daily-use features, then agent capabilities. Package D is
deferred by user decision.

## Completed

### Package 0 — Memory and skills

- Daily-note prompt cache invalidation.
- Agent file backend and Memory WebUI (revision, backups, conflicts, creation).
- Skills marketplace moved into Skills; ClawHub improvements, GitHub URL install,
  stable IDs, editing with revision/conflict handling, update/reinstall,
  suspicious-skill confirmation, malware block.
- Memory master switch, per-session memory off, incognito chats, automatic daily
  notes.

### Package A — Models and providers

- Catalog key reuse; stable name-based edit/delete; duplicate-name rejection.
- Custom / OpenAI-compatible provider; multiple API keys with failover.
- Enable/disable models; proxy-aware fetch/test; provider-specific probes.
- Provider-prefix warning; credentials summary; usage/cost page.

### Package B — Daily use

- Chat attachments (PDF, text, audio, images) with drag-and-drop.
- Scheduled tasks UI and API with validation/run-now.
- Backup and restore (ZIP, manifest, dry-run, safe restore).

### Package C — Agent power

- MCP server management and tests.
- Subagent master switch, delegate, spawn status, live monitoring and cancel.
- Multiple agents with workspaces, models, skills, routing, personality files.
- Usage/cost ledger and page.
- Browser automation sidecar with allowlist and approval flow.

### Package E.1 — Resilience and skills

- Broken/BOM config no longer crashes gateway status; readable `config_error`.
- Tour no longer blocks clicks; restart button; fork-specific steps.
- Dashboard session secret persisted across restarts; rotated on password change
  and sign-out.
- Model rename/delete updates agent and subagent references.
- Deterministic shell/telegram tests; repeated full suite green.
- 13 bundled skills, all reviewed for license and safety.
- Office Pack: portable Python + Word/Excel/PowerPoint/PDF skills.

### Package E.2 — Release

- Versioned Windows ZIP, SHA256, README, roadmap, clean install, fork push.

## Next / candidates

- E.2 follow-ups: signed build (SmartScreen), automated release workflow, tag.
- Optional skill packs (not bundled): `github` (requires `gh`), `summarize`,
  `tmux`, `hardware`; see `docs/skills-optional/`.
- Office Pack: OCR (Tesseract) option, Excel recalculation via LibreOffice,
  richer templates.
- Per-skill global on/off switch in the UI.

## Deferred — Package D

### D.1 Swarm / team MVP

Leader agent splits work across members, parallel execution, tagged fan-in,
shared token budget, team cancel, Teams UI. Later: dependencies, retries,
artifacts, nested teams, distributed swarm.

### D.2 Plugin ecosystem

Manifest package (skills, MCP, hooks, agent templates) with permissions,
staging, signatures, update/remove. Process plugins only through MCP/hook/exec;
no in-process code. Later: SDK/ABI, WASM sandbox, signed registry.

### D.3 Mobile control

- D.3a PWA polish: manifest, service worker, safe-area, touch targets, QR code.
- D.3b Secure remote access: TLS/reverse proxy/Tailscale, per-device sessions,
  revocation, optional 2FA, audit log.
- D.3c Notifications/approvals: web push or ntfy, approve/deny/stop from phone,
  quiet hours.

## Deferred — E.3 portable / USB mode

- `portable.marker` or `-portable`; data next to the exe.
- Relative workspace paths and drive-letter migration.
- Single process lock, secret encryption or “no secrets stored” mode, clear UI
  warnings.
- Test drive-letter changes, spaces/Unicode paths, read-only USB, FAT32 limits.
- Simpler alternative: move `.picoclaw` with Backup & Restore.

## Decisions

- No upstream pull requests; push only to `origin`
  (`https://github.com/techunreal/picoclaw`).
- UI stays English; Turkish UI is out of scope.
- macOS verification deferred.
- Upstream attribution and MIT license retained.
