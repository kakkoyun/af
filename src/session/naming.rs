//! Session name sanitization and branch prefix logic.
//!
//! tmux session names cannot contain `/`, `.`, or `:`. These characters are
//! replaced with `--`. Branch names may be prefixed with a username when
//! working on fork repositories.
