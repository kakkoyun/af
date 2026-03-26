//! Layered configuration system (ADR-003).
//!
//! Configuration is loaded from multiple sources with increasing precedence:
//! compiled defaults → user config → project config → env vars → CLI flags.
