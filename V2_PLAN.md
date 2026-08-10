# glab-helper v2

`v2` replaces the Zsh implementation with one small Go binary. `main` remains
the stable Zsh release until the final cutover.

## Working rules

- Every step starts from `v2`, has one purpose, and returns to `v2` only after
  it builds and its affected behavior is checked.
- Existing GitLab workflows are migrated while the retiring Jira source is
  replaced by the concrete YouTrack setup used by the team.
- Prefer the Go standard library. Keep `glab` and `fzf` until replacing either
  one produces a clear user benefit.
- YouTrack remains a strictly read-only source.
- Introduce the source interface where it is consumed, only when YouTrack is
  connected to the sync core. Do not add a plugin system or speculative
  capabilities.
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
- [ ] 09 — Read YouTrack into a minimal provider-neutral source snapshot.
- [ ] 10 — Produce the combined YouTrack-to-GitLab sync preview without writes.
- [ ] 11 — Confirm and apply the sync plan; keep YouTrack read-only.
- [ ] 12 — List and select an existing issue without mutations.
- [ ] 13 — Migrate branch pruning, creation, and checkout.
- [ ] 14 — Migrate the remaining issue-edit actions.
- [ ] 15 — Migrate the remaining developer actions.
- [ ] 16 — Migrate maintenance reset last because it is destructive.
- [ ] 17 — Switch installation to Go and remove Zsh-only runtime dependencies.

Before step 17, tag the final Zsh release. Merge `v2` into `main` only when the
Go binary covers the current supported workflows and the installer selects it.
