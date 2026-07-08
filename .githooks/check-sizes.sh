#!/bin/bash
set -euo pipefail

MAX_FILE_LINES=500
FAIL=0

echo "→ Checking file sizes..."
for f in $(find . -name '*.go' -not -path '*/vendor/*' -not -name '*_templ.go'); do
    lines=$(wc -l < "$f")
    if [ "$lines" -gt "$MAX_FILE_LINES" ]; then
        echo "  ❌ $f: $lines lines (max $MAX_FILE_LINES)"
        FAIL=1
    fi
done

if [ "$FAIL" -eq 0 ]; then
    echo "  ✅ All files within size limits"
fi

echo "→ Running deadcode scan..."
if which deadcode >/dev/null 2>&1; then
    # Scan application packages only. The internal/llm and internal/datastar
    # packages are intentionally-provided library APIs (wire them when a
    # feature needs them); deadcode would flag their not-yet-wired exports
    # as noise, so they are excluded here.
    output=$(deadcode -test ./cmd/... ./features/... ./router/... ./internal/nats/... ./internal/queue/... ./internal/workflow/... 2>&1) || true
    if [ -n "$output" ]; then
        echo "  ⚠️  Dead code found:"
        echo "$output" | head -20
    else
        echo "  ✅ No dead code detected"
    fi
else
    echo "  ⚡ deadcode not installed (run: go install golang.org/x/tools/cmd/deadcode@latest)"
fi

exit $FAIL
