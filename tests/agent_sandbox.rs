//! Integration tests for per-agent OS sandbox mapping (ADR-028).
//!
//! These tests verify the public contract of each agent's `apply_sandbox`
//! function from the outside (via the library's public API). Unit tests in
//! each agent module test the function directly as `pub(crate)`; these tests
//! verify the `AgentSandbox` type is accessible and behaves correctly when
//! combined with each agent's launch command shape.

use af::agent::amp::{AmpProvider, apply_sandbox as amp_apply};
use af::agent::claude::{ClaudeProvider, apply_sandbox as claude_apply};
use af::agent::codex::{CodexProvider, apply_sandbox as codex_apply};
use af::agent::copilot::{CopilotProvider, apply_sandbox as copilot_apply};
use af::agent::gemini::{GeminiProvider, apply_sandbox as gemini_apply};
use af::agent::pi::{PiProvider, apply_sandbox as pi_apply};
use af::agent::{AgentProvider, AgentSandbox, ApprovalMode, LaunchOpts};

fn default_opts() -> LaunchOpts {
    LaunchOpts {
        session_id: "test-session".to_owned(),
        approval_mode: ApprovalMode::Default,
        sandbox: AgentSandbox::None,
    }
}

// ---------------------------------------------------------------------------
// AgentSandbox type
// ---------------------------------------------------------------------------

#[test]
fn agent_sandbox_default_is_none() {
    assert_eq!(AgentSandbox::default(), AgentSandbox::None);
}

#[test]
fn agent_sandbox_variants_are_distinguishable() {
    assert_ne!(AgentSandbox::None, AgentSandbox::Os);
}

#[test]
fn agent_sandbox_is_copy() {
    let s = AgentSandbox::Os;
    let s2 = s; // Copy — no move error
    assert_eq!(s, s2);
}

// ---------------------------------------------------------------------------
// codex: AgentSandbox::Os → appends `-s workspace-write`
// ---------------------------------------------------------------------------

#[test]
fn codex_sandbox_none_leaves_launch_cmd_unchanged() {
    let p = CodexProvider;
    let mut cmd = p.launch_cmd(&default_opts());
    let before = cmd.clone();
    codex_apply(&mut cmd, AgentSandbox::None);
    assert_eq!(cmd, before);
}

#[test]
fn codex_sandbox_os_appends_s_workspace_write() {
    let p = CodexProvider;
    let mut cmd = p.launch_cmd(&default_opts());
    codex_apply(&mut cmd, AgentSandbox::Os);
    assert!(
        cmd.windows(2).any(|w| w == ["-s", "workspace-write"]),
        "codex argv should contain `-s workspace-write` when AgentSandbox::Os: {cmd:?}"
    );
}

#[test]
fn codex_sandbox_os_with_yolo_mode_places_flag_before_subcommand() {
    // With yolo approval mode, verify sandbox flag is after approval flags.
    let opts = LaunchOpts {
        session_id: "s".to_owned(),
        approval_mode: ApprovalMode::Yolo,
        sandbox: AgentSandbox::None,
    };
    let p = CodexProvider;
    let mut cmd = p.launch_cmd(&opts);
    codex_apply(&mut cmd, AgentSandbox::Os);

    // Expected: ["codex", "--full-auto", "--ask-for-approval", "never", "-s", "workspace-write"]
    let s_pos = cmd
        .iter()
        .position(|t| t == "-s")
        .expect("-s must be present");
    let full_auto_pos = cmd
        .iter()
        .position(|t| t == "--full-auto")
        .expect("--full-auto must be present");
    assert!(
        s_pos > full_auto_pos,
        "-s must come after --full-auto in argv: {cmd:?}"
    );
}

#[test]
fn codex_launch_cmd_with_opts_sandbox_os_appends_workspace_write() {
    // End-to-end wiring: the LaunchOpts.sandbox field reaches apply_sandbox
    // through launch_cmd without a separate call.
    let p = CodexProvider;
    let opts = LaunchOpts {
        session_id: "s".to_owned(),
        approval_mode: ApprovalMode::Default,
        sandbox: AgentSandbox::Os,
    };
    assert_eq!(p.launch_cmd(&opts), vec!["codex", "-s", "workspace-write"]);
}

// ---------------------------------------------------------------------------
// claude: AgentSandbox::Os is a documented no-op
// ---------------------------------------------------------------------------

#[test]
fn claude_sandbox_none_leaves_launch_cmd_unchanged() {
    let p = ClaudeProvider;
    let mut cmd = p.launch_cmd(&default_opts());
    let before = cmd.clone();
    claude_apply(&mut cmd, AgentSandbox::None);
    assert_eq!(cmd, before);
}

#[test]
fn claude_sandbox_os_is_noop() {
    let p = ClaudeProvider;
    let mut cmd = p.launch_cmd(&default_opts());
    let before = cmd.clone();
    claude_apply(&mut cmd, AgentSandbox::Os);
    assert_eq!(
        cmd, before,
        "claude apply_sandbox(Os) must be a no-op (documented in ADR-028)"
    );
}

// ---------------------------------------------------------------------------
// pi: AgentSandbox::Os degrades to none (argv unchanged)
// ---------------------------------------------------------------------------

#[test]
fn pi_sandbox_none_leaves_launch_cmd_unchanged() {
    let p = PiProvider;
    let mut cmd = p.launch_cmd(&default_opts());
    let before = cmd.clone();
    pi_apply(&mut cmd, AgentSandbox::None);
    assert_eq!(cmd, before);
}

#[test]
fn pi_sandbox_os_does_not_modify_argv() {
    let p = PiProvider;
    let mut cmd = p.launch_cmd(&default_opts());
    let before = cmd.clone();
    pi_apply(&mut cmd, AgentSandbox::Os);
    assert_eq!(cmd, before, "pi degrade-to-none must not modify argv");
}

// ---------------------------------------------------------------------------
// gemini: AgentSandbox::Os degrades to none (argv unchanged)
// ---------------------------------------------------------------------------

#[test]
fn gemini_sandbox_none_leaves_launch_cmd_unchanged() {
    let p = GeminiProvider;
    let mut cmd = p.launch_cmd(&default_opts());
    let before = cmd.clone();
    gemini_apply(&mut cmd, AgentSandbox::None);
    assert_eq!(cmd, before);
}

#[test]
fn gemini_sandbox_os_does_not_modify_argv() {
    let p = GeminiProvider;
    let mut cmd = p.launch_cmd(&default_opts());
    let before = cmd.clone();
    gemini_apply(&mut cmd, AgentSandbox::Os);
    assert_eq!(cmd, before, "gemini degrade-to-none must not modify argv");
}

// ---------------------------------------------------------------------------
// amp: AgentSandbox::Os degrades to none (argv unchanged)
// ---------------------------------------------------------------------------

#[test]
fn amp_sandbox_none_leaves_launch_cmd_unchanged() {
    let p = AmpProvider;
    let mut cmd = p.launch_cmd(&default_opts());
    let before = cmd.clone();
    amp_apply(&mut cmd, AgentSandbox::None);
    assert_eq!(cmd, before);
}

#[test]
fn amp_sandbox_os_does_not_modify_argv() {
    let p = AmpProvider;
    let mut cmd = p.launch_cmd(&default_opts());
    let before = cmd.clone();
    amp_apply(&mut cmd, AgentSandbox::Os);
    assert_eq!(cmd, before, "amp degrade-to-none must not modify argv");
}

// ---------------------------------------------------------------------------
// copilot: AgentSandbox::Os degrades to none (argv unchanged)
// ---------------------------------------------------------------------------

#[test]
fn copilot_sandbox_none_leaves_launch_cmd_unchanged() {
    let p = CopilotProvider;
    let mut cmd = p.launch_cmd(&default_opts());
    let before = cmd.clone();
    copilot_apply(&mut cmd, AgentSandbox::None);
    assert_eq!(cmd, before);
}

#[test]
fn copilot_sandbox_os_does_not_modify_argv() {
    let p = CopilotProvider;
    let mut cmd = p.launch_cmd(&default_opts());
    let before = cmd.clone();
    copilot_apply(&mut cmd, AgentSandbox::Os);
    assert_eq!(cmd, before, "copilot degrade-to-none must not modify argv");
}
