//! Deterministic UUID v5 generation for session IDs.
//!
//! Sessions get a stable UUID derived from `"<repo>/<branch>"` using the DNS namespace.
//! This ensures that resuming a session uses the same agent session ID.
