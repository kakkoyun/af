#!/bin/sh
# coverage-check.sh — enforce per-package statement-coverage floors.
#
# Usage: coverage-check.sh <coverage.out>
#
# Floors (percent): pure-logic internal packages must stay at or above 80
# (constitution rule 1); cmd/af is IO-shimmed and relies on testscript
# goldens, so it carries a lower floor. Test-only helper packages are
# exempt. Raise a floor here whenever a package's coverage rises — floors
# only ratchet up.
set -eu

profile=${1:?usage: coverage-check.sh <coverage.out>}
module=github.com/kakkoyun/af

floor_for() {
    case $1 in
    "$module"/cmd/af) echo 65 ;;
    "$module"/internal/testutil) echo 0 ;;  # test-only helpers
    "$module"/internal/doccheck) echo 0 ;;  # test-only doc guard
    "$module"/internal/*) echo 80 ;;
    "$module"/cmd/*) echo 65 ;;
    *) echo 0 ;;
    esac
}

report=$(awk '
    NR == 1 { next } # "mode:" header
    {
        split($1, loc, ":")
        pkg = loc[1]
        sub(/\/[^\/]*$/, "", pkg)
        stmts[pkg] += $2
        if ($3 > 0) covered[pkg] += $2
    }
    END {
        for (pkg in stmts)
            printf "%s %.1f\n", pkg, covered[pkg] * 100 / stmts[pkg]
    }
' "$profile" | sort)

status=0

# Packages with no test files never appear in the profile; without this
# sweep a 0%-covered package silently passes the gate.
for pkg in $(go list ./... 2>/dev/null); do
    case $report in
    *"$pkg "*) ;;
    *)
        floor=$(floor_for "$pkg")
        if [ "$floor" -gt 0 ]; then
            printf 'FAIL %-60s missing from profile (no tests?) < floor %s%%\n' "$pkg" "$floor"
            status=1
        else
            printf 'ok   %-60s absent from profile (floor %s%%)\n' "$pkg" "$floor"
        fi
        ;;
    esac
done

while IFS=' ' read -r pkg pct; do
    floor=$(floor_for "$pkg")
    if awk "BEGIN { exit !($pct < $floor) }"; then
        printf 'FAIL %-60s %6.1f%% < floor %s%%\n' "$pkg" "$pct" "$floor"
        status=1
    else
        printf 'ok   %-60s %6.1f%% (floor %s%%)\n' "$pkg" "$pct" "$floor"
    fi
done <<EOF
$report
EOF

exit $status
