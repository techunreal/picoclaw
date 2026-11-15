---
name: repo-research
description: "Find, compare, and evaluate GitHub repositories for a user's need. Use when the user asks for a library, tool, template, or project recommendation, wants to know whether a repo is trustworthy or maintained, or asks to research a specific GitHub project. Prioritizes high-star and actively maintained repos, checks publisher trust, and returns evidence-backed recommendations with tradeoffs."
metadata: {"picoclaw":{"emoji":"🔎","requires":{"bins":["git"]}}}
---

# Repository Research

Find the right open-source project, then judge it on evidence instead of star
count alone. The deliverable is a short, decision-ready report with sources and
tradeoffs, not a list of links.

## When to use

- "Find me a tool/library/template that does X."
- "Is this GitHub repo safe, maintained, or worth using?"
- "Compare these repos and tell me which one fits."
- "Research this repository before I install it."

## Step 1 — Pin the requirement

Ask only what changes the answer:

- What problem should it solve, and what is out of scope?
- Must-haves: OS (Windows-first?), language/runtime, self-hosted vs cloud,
  offline/local, CLI vs GUI vs library, license constraints.
- Constraints: weak hardware, no Docker, no Python/Node, company policy.
- How many candidates are enough: one solid pick or a short comparison?

Write the requirement in one or two sentences before searching.

## Step 2 — Search broadly

Prefer GitHub search with explicit filters; use the browser or `web_fetch`:

- Keyword and synonym variants; include the problem, not just the product name.
- Filter by `language:`, `stars:>100`, `pushed:>2025-01-01`, `license:mit`,
  `topic:<topic>`, `archived:false`, `is:public`.
- Sort by stars first, then re-sort by recently updated to catch active smaller
  projects.
- Search topics and awesome-lists (e.g. `awesome <domain>`) for curated options.
- Check package registries and release pages when relevant.

Example queries:

```text
https://github.com/search?q=<keywords>+stars%3A%3E100+pushed%3A%3E2025-01-01&type=repositories&s=stars&o=desc
https://api.github.com/search/repositories?q=<keywords>+stars:>100&sort=stars&order=desc
```

Unauthenticated GitHub API is rate-limited. Respect limits; use the web page
when the API is exhausted. An authenticated `gh` CLI is optional, never
required.

## Step 3 — Shortlist (5–10)

For each candidate capture:

- Repo URL, owner, description, primary language, license.
- Stars, forks, watchers, open issues/PRs.
- Last commit and last release dates.
- Contributors count and whether the owner is an organization.
- Archived/deprecated flag, security policy, release notes.

Drop anything that is archived, unlicensed, abandoned for a year with no
activity, or clearly outside the requirement.

## Step 4 — Trust and maintenance signals

Judge these together; a single number is never enough:

- **Activity:** commits and releases in the last 3–12 months, issue response
  time, whether maintainers answer bug reports.
- **Bus factor:** one maintainer vs a team/org; recent contributor spread.
- **Adoption:** stars *relative to age and niche*, forks, dependents, mentions
  in docs/awesome lists. Treat sudden star spikes or star-only repos with
  suspicion.
- **Publisher:** known organization or maintainer with a track record; verified
  domain; other maintained repos; security policy and disclosure history.
- **Quality:** README with install/usage, tests, CI, changelog, examples,
  reproducible releases, signed artifacts where relevant.
- **Risk:** dependency count, install scripts, binaries, telemetry, network
  calls, license compatibility (MIT/Apache/BSD vs GPL), recent CVEs.
- **Platform fit:** Windows support, prebuilt binaries, Docker/WSL requirements,
  runtime prerequisites.

## Step 5 — Deep-check the top 2–3

Clone shallow copies into a temporary folder inside the workspace, for example
`temp/repo-research/<owner>-<repo>`. Do not clone into the user's project.

```bash
git clone --depth 1 https://github.com/<owner>/<repo>.git temp/repo-research/<repo>
```

Inspect, read-only:

- `README`, `LICENSE`, `CHANGELOG`, `SECURITY.md`, `CONTRIBUTING`.
- Package manifests (`package.json`, `pyproject.toml`, `go.mod`, `Cargo.toml`)
  and lockfiles for dependency weight.
- Test folders and CI config; do they actually run tests?
- Install scripts and post-install hooks; note anything that downloads or
  executes code.
- Open issues labeled bug/security; closed issues to see response quality.
- Releases: dates, assets for Windows, checksums/signatures.

**Never run the project's install scripts, tests, or binaries without explicit
user approval.** Cloned code is untrusted input. Do not paste secrets or tokens
into the report.

## Step 6 — Score and recommend

Use a simple table; score only the dimensions that matter for this request:

| Candidate | Fit | Activity | Maturity | Docs | License | Risk | Windows |
|---|---|---|---|---|---|---|---|
| owner/repo | high/med/low | last commit | tests/CI | quality | SPDX | low/med/high | yes/no |

Then write:

1. **Recommendation:** the best fit and why, in 2–4 sentences.
2. **Runner-up / alternative:** when it is better (lighter, simpler, more
   permissive, more active).
3. **Tradeoffs and risks:** maintenance, license, platform, security, lock-in.
4. **Evidence:** links, dates, license, last release, issue/PR observations.
5. **Next step:** what to verify or try before adopting.

If nothing fits, say so and name the closest options with the gap.

## Rules

- Never rank by stars alone; stars measure attention, not quality.
- Prefer active, maintained projects over famous but stale ones.
- Quote dates for activity claims ("last release 2026-08-14", not "recently").
- Distinguish fact (file/URL) from inference (your judgment).
- Do not install, execute, or approve a repository on the user's behalf.
- Keep temporary clones under the workspace `temp/` folder and clean them up
  when finished.
