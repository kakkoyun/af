// Package gh wraps the parts of the gh CLI that af consumes — PR
// metadata and diff retrieval per ADR-073 §4 + §5. The wrappers
// invoke gh through a sandbox.Runner so tests can substitute a fake
// implementation.
package gh
