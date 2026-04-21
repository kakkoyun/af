#!/usr/bin/env sh
# book-gen.sh — generate mdBook command reference from `af <cmd> --help` output.
#
# Writes book/src/commands/<cmd>.md for each top-level subcommand. Pages are
# wrapped in a triple-backtick `text` block so mdBook renders them verbatim.
# Also emits book/src/commands/index.json as a machine-readable manifest.
#
# Prefers a locally built debug binary (fast); falls back to `cargo run` if
# the binary is missing. See ADR-020.

set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="$ROOT/book/src/commands"
BIN="$ROOT/target/debug/af"

COMMANDS="create done list resume agent gc editor diff pr note stats export doctor config completions session-branch version"

mkdir -p "$OUT_DIR"

run_af() {
    if [ -x "$BIN" ]; then
        "$BIN" "$@"
    else
        (cd "$ROOT" && cargo run --quiet -- "$@")
    fi
}

emit_page() {
    cmd="$1"
    out="$OUT_DIR/$cmd.md"
    help_text=$(run_af "$cmd" --help 2>&1 || true)

    {
        printf '# %s\n\n' "$cmd"
        printf '> Generated from `af %s --help`. Do not edit by hand — re-run `just book-gen`.\n\n' "$cmd"
        printf '```text\n'
        printf '%s\n' "$help_text"
        printf '```\n'
    } > "$out"
    printf '  wrote %s\n' "$out"
}

emit_manifest() {
    manifest="$OUT_DIR/index.json"
    {
        printf '{\n'
        printf '  "commands": [\n'
        first=1
        for cmd in $COMMANDS; do
            if [ "$first" -eq 1 ]; then
                first=0
            else
                printf ',\n'
            fi
            printf '    { "name": %s, "page": %s }' "\"$cmd\"" "\"$cmd.md\""
        done
        printf '\n  ]\n'
        printf '}\n'
    } > "$manifest"
    printf '  wrote %s\n' "$manifest"
}

printf 'book-gen: regenerating command reference pages in %s\n' "$OUT_DIR"
for cmd in $COMMANDS; do
    emit_page "$cmd"
done
emit_manifest
printf 'book-gen: done (%s commands)\n' "$(printf '%s\n' $COMMANDS | wc -l | tr -d ' ')"
