// Package sessiondata implements ADR-066 — agent and harness session
// data export from slicer VMs.
//
// The package harvests transcripts and session metadata from a slicer
// VM's home directory through an allowlist, stages them under
// ~/.local/share/af/v1/session-import/<af-session>/<vm>/<timestamp>/,
// and merges them into the corresponding host-side agent directories
// using a hash-aware append-only strategy. Files whose host destination
// already exists with different content are routed to a sibling
// conflicts/ tree rather than overwritten.
//
// Two host-facing operations are exposed: Pull (copy + merge) and List
// (inventory only). Both consult the same Slicer interface which the
// CLI binds to slicer vm exec / slicer vm cp; tests substitute the
// FakeSlicer backed by a real on-disk directory.
//
// Host continuation (rewriting transcript metadata so claude/codex/pi
// can resume on the host) is gated behind a future implementation step
// — see the ADR-066 §"Host continuation mode" comment in pull.go.
package sessiondata
