#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FIXTURE_DIR="$ROOT/testing/filehog/generated/basic-files"

rm -rf "$FIXTURE_DIR"
mkdir -p "$FIXTURE_DIR/docs" "$FIXTURE_DIR/misc"

cat >"$FIXTURE_DIR/docs/hit.txt" <<'EOF'
This file contains harbor in the body.
EOF

cat >"$FIXTURE_DIR/misc/no-hit.txt" <<'EOF'
This file should not match the current keyword set.
EOF

touch "$FIXTURE_DIR/docs/harbor-notes.bin"

printf 'Generated FileHog fixtures in %s\n' "$FIXTURE_DIR"
