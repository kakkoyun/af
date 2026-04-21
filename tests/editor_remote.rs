//! Integration tests for `af editor` remote-session URL building (ADR-019).
//!
//! These tests exercise the pure URL-builder functions exported from
//! `af::cmd::editor`. They touch no filesystem state and spawn no processes.
//! The functions tested here are the seam Lane L-REMOTE will wire into
//! `run()` once `ExecutionInfo` carries SSH-target metadata.

use af::cmd::editor::{EditorKind, build_remote_open_args, build_workspaces_connect_args};

// ── EditorKind parsing (config.editor.visual -> EditorKind) ──────────────────

#[test]
fn test_editor_kind_from_str_code() {
    assert_eq!("code".parse::<EditorKind>().unwrap(), EditorKind::VSCode);
    assert_eq!("vscode".parse::<EditorKind>().unwrap(), EditorKind::VSCode);
}

#[test]
fn test_editor_kind_from_str_cursor() {
    assert_eq!("cursor".parse::<EditorKind>().unwrap(), EditorKind::Cursor);
}

#[test]
fn test_editor_kind_from_str_zed() {
    assert_eq!("zed".parse::<EditorKind>().unwrap(), EditorKind::Zed);
}

#[test]
fn test_editor_kind_from_str_is_case_insensitive() {
    assert_eq!("CODE".parse::<EditorKind>().unwrap(), EditorKind::VSCode);
    assert_eq!("Cursor".parse::<EditorKind>().unwrap(), EditorKind::Cursor);
    assert_eq!("ZED".parse::<EditorKind>().unwrap(), EditorKind::Zed);
}

#[test]
fn test_editor_kind_from_str_unknown_errors() {
    assert!("emacs".parse::<EditorKind>().is_err());
    assert!("vim".parse::<EditorKind>().is_err());
    assert!("".parse::<EditorKind>().is_err());
}

// ── VSCode remote URL ────────────────────────────────────────────────────────

#[test]
fn test_build_vscode_remote_args() {
    let args = build_remote_open_args(&EditorKind::VSCode, "my-host", "/home/user/project");
    // VSCode: `code --folder-uri vscode-remote://ssh-remote+<host>/<path>`
    assert_eq!(args.binary, "code");
    assert_eq!(
        args.argv,
        vec![
            String::from("--folder-uri"),
            String::from("vscode-remote://ssh-remote+my-host/home/user/project"),
        ]
    );
}

#[test]
fn test_build_vscode_remote_url_preserves_dots_in_host() {
    let args = build_remote_open_args(&EditorKind::VSCode, "vm-123.exe.dev", "/workspace/repo");
    let uri = &args.argv[1];
    assert!(
        uri.starts_with("vscode-remote://ssh-remote+vm-123.exe.dev/"),
        "unexpected uri: {uri}"
    );
    assert!(uri.ends_with("/workspace/repo"), "unexpected uri: {uri}");
}

#[test]
fn test_build_vscode_remote_handles_path_with_or_without_leading_slash() {
    // Path may arrive with or without a leading slash; URL must contain
    // exactly one between "ssh-remote+host" and the rest.
    let with = build_remote_open_args(&EditorKind::VSCode, "host", "/a/b");
    let without = build_remote_open_args(&EditorKind::VSCode, "host", "a/b");
    assert_eq!(with.argv[1], "vscode-remote://ssh-remote+host/a/b");
    assert_eq!(without.argv[1], "vscode-remote://ssh-remote+host/a/b");
}

// ── Cursor remote URL ────────────────────────────────────────────────────────

#[test]
fn test_build_cursor_remote_args() {
    let args = build_remote_open_args(&EditorKind::Cursor, "my-host", "/home/user/project");
    // Cursor: `cursor --folder-uri cursor://vscode-remote/ssh-remote+<host>/<path>`
    assert_eq!(args.binary, "cursor");
    assert_eq!(
        args.argv,
        vec![
            String::from("--folder-uri"),
            String::from("cursor://vscode-remote/ssh-remote+my-host/home/user/project"),
        ]
    );
}

#[test]
fn test_build_cursor_remote_url_has_correct_scheme() {
    let args = build_remote_open_args(&EditorKind::Cursor, "dev-vm", "/code/myrepo");
    let uri = &args.argv[1];
    assert!(
        uri.starts_with("cursor://vscode-remote/ssh-remote+"),
        "unexpected uri: {uri}"
    );
}

// ── Zed remote URI ───────────────────────────────────────────────────────────

#[test]
fn test_build_zed_remote_args() {
    let args = build_remote_open_args(&EditorKind::Zed, "my-host", "/home/user/project");
    // Zed: `zed ssh://my-host/home/user/project`
    assert_eq!(args.binary, "zed");
    assert_eq!(
        args.argv,
        vec![String::from("ssh://my-host/home/user/project")]
    );
}

#[test]
fn test_build_zed_remote_uri_format() {
    let args = build_remote_open_args(&EditorKind::Zed, "dev-vm.internal", "/srv/code");
    let uri = &args.argv[0];
    assert!(uri.starts_with("ssh://"), "unexpected arg: {uri}");
    assert!(uri.contains("dev-vm.internal"), "host missing: {uri}");
    assert!(uri.ends_with("/srv/code"), "path missing: {uri}");
}

// ── Workspaces provider: separate opener ────────────────────────────────────

#[test]
fn test_build_workspaces_connect_args() {
    // Workspaces uses `workspaces connect <name>` — not a URI scheme.
    let args = build_workspaces_connect_args("my-workspace");
    assert_eq!(args.binary, "workspaces");
    assert_eq!(
        args.argv,
        vec![String::from("connect"), String::from("my-workspace")]
    );
}

#[test]
fn test_workspaces_connect_with_various_names() {
    let a = build_workspaces_connect_args("session-42");
    assert_eq!(
        a.argv,
        vec![String::from("connect"), String::from("session-42")]
    );

    let b = build_workspaces_connect_args("kakkoyun--issue-99");
    assert_eq!(
        b.argv,
        vec![String::from("connect"), String::from("kakkoyun--issue-99")]
    );
}
