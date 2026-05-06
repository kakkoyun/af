# ADR-021: Release Discipline & CHANGELOG-Driven Notes

**Status:** Accepted
**Date:** 2026-04-21

## Context

The release workflow (`release.yml`) currently sets `generate_release_notes: true`
in `softprops/action-gh-release`, which auto-assembles release notes from commit
messages via GitHub's "automatically generated release notes" feature. This creates
two problems:

1. **Authoritative narrative lives in two places.** `CHANGELOG.md` is curated;
   GitHub's auto-notes are generated. Readers see different summaries depending
   on where they look.
2. **CHANGELOG drift goes undetected.** If commits ship without a matching
   CHANGELOG entry, the auto-notes paper over the gap rather than surfacing it.

Additionally, no project-level ritual exists for verifying the release workflow
before tagging. Discovering a matrix-build failure after pushing a tag is painful
because the tag is already public.

## Decision

### CHANGELOG-driven release notes

Replace `generate_release_notes: true` with an `awk` extraction step that reads
the section matching the release tag from `CHANGELOG.md` and passes it to the
`body` field of `softprops/action-gh-release`.

The extraction command:

```bash
version="${tag#v}"
notes=$(awk "/^## \[${version}\]/{flag=1; next} /^## \[/{flag=0} flag{print}" CHANGELOG.md)
```

This is idiomatic portable `awk` with no external tooling. It sets a flag on the
heading matching the version, clears it on the next heading, and prints all lines
in between. If the CHANGELOG has no entry for the tag, `notes` will be empty and
the release body will clearly indicate the gap.

### Mandatory dry-run before tag

Before pushing any `vX.Y.Z` tag, run the release workflow via `workflow_dispatch`
against a throwaway tag (e.g., `v0.0.0-dry`) to verify all 6 matrix-build targets
compile and artifacts upload. Delete the resulting draft release. Only then push the
real tag.

The `just release-dry-run` recipe encapsulates this:

```
release-dry-run:
    gh workflow run release.yml -f tag=v0.0.0-dry
    @echo "Monitor: gh run list --workflow=release.yml"
    @echo "Cleanup: gh release delete v0.0.0-dry --yes && git push origin :refs/tags/v0.0.0-dry"
```

### When to tag

The user (project owner) retains the sole authority to call for a tag. The sprint
completion criterion ("all initial topics covered + patterns established") is a
necessary precondition but not sufficient — the user must explicitly approve.

### CHANGELOG maintenance rule

- Every user-facing commit must add a bullet under `## [Unreleased]`.
- On release: rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD` and add the
  version link at the bottom of the file.
- CHANGELOG entries follow Keep-a-Changelog groupings (`Added`, `Changed`, `Fixed`,
  `Removed`, `Deprecated`, `Security`). The existing grouped-by-phase convention
  used in `0.1.0` is acceptable for the first release; subsequent releases use the
  standard groups.

## Consequences

- The GitHub release page body is always identical to the CHANGELOG section.
  One canonical source of truth.
- CHANGELOG gaps become visible as empty release bodies rather than being
  silently filled with auto-generated content.
- The dry-run ritual adds ~5 minutes of verification overhead before any tag.
  This is acceptable given that tags are rare and one-way.
- `just release-dry-run` encodes the verification steps so they are discoverable
  and repeatable without consulting this ADR.
