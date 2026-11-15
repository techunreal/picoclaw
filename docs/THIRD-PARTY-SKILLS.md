# Third-party skills shipped with the Windows build

The Windows build seeds the agent workspace from `workspace/` (see
`onboard_workspace_embed.go`). The skills below are copied into
`<workspace>/skills/` on first setup. They are ordinary Markdown skills; no
third-party binaries are executed by the skills themselves.

## Bundled skills and licenses

| Skill | Source | License | Notes |
|---|---|---|---|
| `weather` | PicoClaw upstream (`workspace/skills/weather`) | MIT (PicoClaw) | Uses `curl`/wttr.in/Open-Meteo; no API key |
| `agent-browser` | PicoClaw upstream | MIT (PicoClaw) | Uses the `agent-browser` CLI bundled in the ZIP |
| `picoclaw-agent` | PicoClaw upstream | MIT (PicoClaw) | PicoClaw configuration/usage guide |
| `skill-creator` | PicoClaw upstream | MIT (PicoClaw) | Skill authoring guide |
| `humanizer` | https://github.com/blader/humanizer | MIT, Copyright (c) 2025 Siqi Chen | Unmodified; `LICENSE` included in the skill folder |
| `brainstorming` | https://github.com/obra/superpowers | MIT, Copyright (c) 2025 Jesse Vincent | Core skill; the optional Node-based visual companion was removed and replaced with a short text note |
| `skill-safety-check` | PicoClaw fork | MIT | Original skill written for this fork; inspired by the idea of `skill-vetter` but not a copy |

## Optional skills (not bundled by default)

These remain in the repository under `docs/skills-optional/` for a later
opt-in pack. They are not copied into the workspace by the installer.

- `github` — requires the `gh` CLI.
- `hardware` — requires the I2C/SPI tools and Sipeed hardware.
- `summarize` — requires the `summarize` CLI.
- `tmux` — Linux/macOS only.

## Adding a third-party skill

1. Verify the license permits redistribution (MIT/Apache-2.0/BSD/ISC).
2. Review every file with `skill-safety-check`; no silent installers, network
   uploads, credential access, or obfuscated code.
3. Keep the original license file inside the skill folder.
4. Add the skill to this table with source, license, and any local changes.
5. Note the change in `HANDOFF.md` and rebuild the ZIP.
