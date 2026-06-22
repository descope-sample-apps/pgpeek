#!/usr/bin/env bash
# Fails if any function under internal/ is below 100% statement coverage.
# The main package is intentionally excluded: it is thin bootstrap glue whose
# residual lines are the os.Exit entrypoint and defensive log branches, verified
# by integration tests rather than unit-asserted.
set -euo pipefail

profile="${1:-cover.out}"

if [[ ! -f "$profile" ]]; then
  echo "coverage profile not found: $profile" >&2
  exit 2
fi

gaps="$(go tool cover -func="$profile" | awk '/\/internal\// && $NF != "100.0%" { print }')"

if [[ -n "$gaps" ]]; then
  echo "✗ internal/ coverage below 100%:" >&2
  echo "$gaps" >&2
  exit 1
fi

echo "✓ internal/... is at 100% statement coverage"
