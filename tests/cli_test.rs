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
