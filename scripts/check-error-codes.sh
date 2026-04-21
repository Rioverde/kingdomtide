#!/usr/bin/env bash
# check-error-codes.sh — fail if any sendError call in server code still
# passes a raw string literal as the code argument. Use pb.ErrCodeXxx
# constants instead so the client catalog and the wire protocol stay in sync.
#
# Usage: scripts/check-error-codes.sh
# Exit 0 = all clear. Exit 1 = hardcoded codes found.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SEARCH_DIR="$ROOT/internal/server"

# Match sendError( ... , "literal") — three-argument form where the third
# argument is a double-quoted string. Exclude test files and lines that
# already reference pb.ErrCode or an errcode. identifier.
matches=$(grep -rn 'sendError([^,]*,[^,]*, "' "$SEARCH_DIR" 2>/dev/null \
    | grep -v '_test\.go' \
    | grep -v 'pb\.ErrCode' \
    | grep -v 'errcode\.' \
    || true)

if [ -z "$matches" ]; then
    exit 0
fi

while IFS= read -r line; do
    file=$(echo "$line" | cut -d: -f1)
    lineno=$(echo "$line" | cut -d: -f2)
    echo "Hardcoded error code in $file:$lineno. Use pb.ErrCodeXxx constant instead."
done <<< "$matches"

exit 1
