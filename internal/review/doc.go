// Package review implements ADR-073 — the read-only af review
// command. It assembles an immutable system prompt with optional
// per-repo and CLI append layers, invokes the configured agent's
// BodyCmd, and writes the response into a .af/reviews/<UTC>-pr<n>.md
// report file.
package review
