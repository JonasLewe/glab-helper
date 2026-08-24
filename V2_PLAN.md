# glab-helper v2

`v2` replaces the Zsh implementation with one small Go binary. `main` remains
the stable Zsh release until the final cutover.

## Working rules

- Every step starts from `v2`, has one purpose, and returns to `v2` only after
  it builds and its affected behavior is checked.
- Existing GitLab workflows are migrated while the retiring Jira source is
  replaced by the concrete YouTrack setup used by the team.
- Target GitLab Community Edition 19.2.1. Represent the team's concrete
  two-level source hierarchy as milestone -> issue, while retaining the
  configurable milestone -> issue -> task model without relying on paid epics
  or configurable work item types. Allow source levels to be ignored while
  preserving validation of the complete YouTrack parent chain.
- Prefer the Go standard library. Keep `glab` and `fzf` until replacing either
  one produces a clear user benefit.
- YouTrack remains a strictly read-only source.
- Introduce the source interface where it is consumed, only when YouTrack is
  connected to the sync core. Do not add a plugin system or speculative
  capabilities.
- Validate data once at its boundary; downstream code trusts those validated
  invariants instead of repeating the same checks.
- Add one focused test for risky behavior. Remove the corresponding Zsh code
  and tests as soon as a Go path replaces them.

## Steps

- [x] 01 — Minimal Go command with offline help and version.
- [x] 02 — Preserve the existing CLI flag contract.
- [x] 03 — Read and validate the current GitLab project without mutations.
- [x] 04 — Read all GitLab issues or fail without a partial result.
- [x] 05 — Read all GitLab milestones or fail without a partial result.
- [x] 06 — Read all GitLab labels or fail without a partial result.
- [x] 07 — Read current remote branches; keep pruning and checkout disabled.
- [x] 08 — Load and validate the YouTrack configuration read-only.
- [x] 09 — Read and validate a configurable YouTrack hierarchy into a minimal
  provider-neutral source snapshot.
- [x] 10 — Preview the combined milestone/issue/task synchronization without
  writes.
- [x] 11 — Confirm and apply the sync plan; keep YouTrack read-only.
- [x] 11a — Simplify the sync core without changing its supported behavior.
- [x] 12 — List and select an existing issue or task without mutations.
- [x] 13 — Migrate branch pruning, creation, and checkout for issues and tasks.
- [x] 13a — Provision missing issue-board lists from synchronized status labels.
- [x] 14 — Migrate the remaining issue-edit actions.
- [x] 15 — Migrate the remaining developer actions.
- [ ] 16 — Migrate maintenance reset last because it is destructive.
- [ ] 16a — Polish the interactive terminal UX: show a compact glab-helper
  startup logo after clearing the screen and add optional color highlights only
  when the terminal supports them, with a no-error plain-text fallback.
- [ ] 17 — Switch installation to Go and remove Zsh-only runtime dependencies.

Before step 17, tag the final Zsh release. Merge `v2` into `main` only when the
Go binary covers the current supported workflows and the installer selects it.
