#!/usr/bin/env bash
# check-locale-keys.sh — fail if any locale.Tr call still passes a string
# literal as the messageID argument. Use locale.KeyXxx constants instead.
#
# Usage: scripts/check-locale-keys.sh
# Exit 0 = all clear. Exit 1 = hardcoded keys found.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SEARCH_DIR="$ROOT/internal"

# Match: locale.Tr( followed by the lang argument (an identifier or field
# expression), then a comma, optional space, and a double-quoted string
# literal as the messageID. Template key/value pairs that appear later in
# the argument list are not caught because they follow additional commas.
matches=$(grep -rn 'locale\.Tr([^,]*,\s*"' "$SEARCH_DIR" 2>/dev/null || true)

if [ -z "$matches" ]; then
    exit 0
fi

while IFS= read -r line; do
    file=$(echo "$line" | cut -d: -f1)
    lineno=$(echo "$line" | cut -d: -f2)
    echo "Hardcoded locale key in $file:$lineno. Use locale.KeyXxx constant instead."
done <<< "$matches"

exit 1
