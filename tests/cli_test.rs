//! Integration tests for the `af` CLI binary.

use assert_cmd::Command;
use predicates::prelude::*;

fn cmd() -> Command {
    Command::cargo_bin("af").expect("binary exists")
}

#[test]
fn test_version_subcommand() {
    cmd()
        .arg("version")
        .assert()
        .success()
        .stdout(predicate::str::starts_with("af "));
}

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

#[test]
fn test_version_flag() {
    cmd()
        .arg("--version")
        .assert()
        .success()
        .stdout(predicate::str::starts_with("af "));
}
