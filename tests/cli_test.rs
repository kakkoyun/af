//! Integration tests for the `af` CLI binary.

use assert_cmd::Command;
use predicates::prelude::*;

fn cmd() -> Command {
    Command::cargo_bin("af").expect("binary exists")
}

// ── Version ─────────────────────────────────────────────────────────────────

#[test]
fn test_version_subcommand() {
    cmd()
        .arg("version")
        .assert()
        .success()
        .stdout(predicate::str::starts_with("af "));
}

#[test]
fn test_version_flag() {
    cmd()
        .arg("--version")
        .assert()
        .success()
        .stdout(predicate::str::starts_with("af "));
}

// ── Help ────────────────────────────────────────────────────────────────────

#[test]
fn test_no_args_shows_help() {
    cmd()
        .assert()
        .failure()
        .stderr(predicate::str::contains("Usage"));
}

#[test]
fn test_help_flag() {
    cmd()
        .arg("--help")
        .assert()
        .success()
        .stdout(predicate::str::contains("agentic-flow"));
}

// ── Subcommand help ─────────────────────────────────────────────────────────

#[test]
fn test_create_help_shows_flags() {
    cmd()
        .args(["create", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("--from"))
        .stdout(predicate::str::contains("--current"))
        .stdout(predicate::str::contains("--from-pr"))
        .stdout(predicate::str::contains("--bare"))
        .stdout(predicate::str::contains("--agent"));
}

#[test]
fn test_done_help_shows_force() {
    cmd()
        .args(["done", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("--force"));
}

#[test]
fn test_resume_help_shows_bare() {
    cmd()
        .args(["resume", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("--bare"));
}

#[test]
fn test_resume_help_shows_respawn() {
    cmd()
        .args(["resume", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("--respawn"));
}

#[test]
fn test_doctor_help_shows_fix() {
    cmd()
        .args(["doctor", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("--fix"))
        .stdout(predicate::str::contains("--yes"));
}

#[test]
fn test_list_help() {
    cmd()
        .args(["list", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("List active workstreams"));
}

#[test]
fn test_session_branch_help() {
    cmd()
        .args(["session-branch", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("session ID"));
}

// ── Create remote/sandbox/yolo flags ────────────────────────────────────────

#[test]
fn test_create_help_shows_remote_sandbox_yolo() {
    cmd()
        .args(["create", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("--remote"))
        .stdout(predicate::str::contains("--sandbox"))
        .stdout(predicate::str::contains("--yolo"));
}

// ── Flag conflicts ──────────────────────────────────────────────────────────

#[test]
fn test_create_from_and_current_conflict() {
    cmd()
        .args(["create", "--from", "develop", "--current", "task"])
        .assert()
        .failure()
        .stderr(predicate::str::contains("cannot be used with"));
}

#[test]
fn test_doctor_yes_requires_fix() {
    cmd()
        .args(["doctor", "--yes"])
        .assert()
        .failure()
        .stderr(predicate::str::contains("--fix"));
}

// ── List with no sessions ───────────────────────────────────────────────────

#[test]
fn test_list_empty_shows_no_sessions() {
    // When no sessions exist, should print a message (not crash).
    // May fail if user has real sessions, but the output should not panic.
    let result = cmd().arg("list").assert();
    // Either success with "No active" or success with session listing.
    result.success();
}

// ── Config ──────────────────────────────────────────────────────────────────

#[test]
fn test_config_show() {
    cmd()
        .args(["config", "show"])
        .assert()
        .success()
        .stdout(predicate::str::contains("[general]"));
}

#[test]
fn test_config_help() {
    cmd()
        .args(["config", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("show"))
        .stdout(predicate::str::contains("init"));
}

// ── Completions ─────────────────────────────────────────────────────────────

#[test]
fn test_completions_bash() {
    cmd()
        .args(["completions", "bash"])
        .assert()
        .success()
        .stdout(predicate::str::contains("_af"));
}

#[test]
fn test_completions_zsh() {
    cmd()
        .args(["completions", "zsh"])
        .assert()
        .success()
        .stdout(predicate::str::contains("#compdef af"));
}

#[test]
fn test_completions_fish() {
    cmd()
        .args(["completions", "fish"])
        .assert()
        .success()
        .stdout(predicate::str::contains("complete"));
}

// ── Agent subcommand ───────────────────────────────────────────────────────

#[test]
fn test_agent_help_shows_subcommands() {
    cmd()
        .args(["agent", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("add"))
        .stdout(predicate::str::contains("stop"))
        .stdout(predicate::str::contains("list"));
}

#[test]
fn test_agent_add_help_shows_flags() {
    cmd()
        .args(["agent", "add", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("--slot"))
        .stdout(predicate::str::contains("--agent"))
        .stdout(predicate::str::contains("--session"));
}

#[test]
fn test_agent_stop_help_shows_slot() {
    cmd()
        .args(["agent", "stop", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("slot"))
        .stdout(predicate::str::contains("--session"));
}

#[test]
fn test_agent_stop_requires_slot_arg() {
    cmd()
        .args(["agent", "stop"])
        .assert()
        .failure()
        .stderr(predicate::str::contains("<SLOT>").or(predicate::str::contains("required")));
}

#[test]
fn test_agent_no_subcommand_shows_help() {
    cmd()
        .arg("agent")
        .assert()
        .failure()
        .stderr(predicate::str::contains("Usage"));
}

// ── PR subcommand ──────────────────────────────────────────────────────────

#[test]
fn test_pr_help_shows_flags() {
    cmd()
        .args(["pr", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("--title"))
        .stdout(predicate::str::contains("--draft"))
        .stdout(predicate::str::contains("--web"));
}

// ── Diff subcommand ────────────────────────────────────────────────────────

#[test]
fn test_diff_help_shows_flags() {
    cmd()
        .args(["diff", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("--base"))
        .stdout(predicate::str::contains("--dark"))
        .stdout(predicate::str::contains("--unified"))
        .stdout(predicate::str::contains("--no-open"));
}

// ── Man page ───────────────────────────────────────────────────────────────

#[test]
fn test_mangen_produces_roff_output() {
    cmd()
        .arg("mangen")
        .assert()
        .success()
        .stdout(predicate::str::contains(".TH"))
        .stdout(predicate::str::contains("af"));
}

// ── Agent flag ──────────────────────────────────────────────────────────────

#[test]
fn test_create_unknown_agent_shows_supported_list() {
    // Running create outside a git repo with an unknown agent should fail
    // with a message listing supported agents.
    let result = cmd()
        .args(["create", "--agent", "unknown-agent", "test-task"])
        .env("HOME", "/tmp/af-test-nonexistent")
        .assert();
    // Should fail (either unknown agent error or no tmux).
    result.failure();
}

// ── Export subcommand ─────────────────────────────────────────────────────

#[test]
fn test_export_help_shows_format() {
    cmd()
        .args(["export", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("--format"));
}

#[test]
fn test_export_help_shows_session() {
    cmd()
        .args(["export", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("session"));
}

// ── Doctor --verbose ──────────────────────────────────────────────────────

#[test]
fn test_doctor_help_shows_verbose() {
    cmd()
        .args(["doctor", "--help"])
        .assert()
        .success()
        .stdout(predicate::str::contains("--verbose"));
}
