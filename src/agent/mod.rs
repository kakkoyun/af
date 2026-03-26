//! AI agent provider abstraction (ADR-001).
//!
//! Defines the [`AgentProvider`] trait that encapsulates agent-specific behaviour
//! (launch commands, session resumption, permission bypass). Built-in providers:
//! Claude Code, pi, Codex, Gemini CLI, Amp.
