#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FIXTURE_DIR="$ROOT/testing/filehog/generated/basic-files"
EXTRACTION_FIXTURE_DIR="$ROOT/testing/filehog/generated/extraction-placeholders"

rm -rf "$FIXTURE_DIR"
rm -rf "$EXTRACTION_FIXTURE_DIR"
mkdir -p "$FIXTURE_DIR/docs" "$FIXTURE_DIR/misc"
mkdir -p "$EXTRACTION_FIXTURE_DIR/docs"

cat >"$FIXTURE_DIR/docs/hit.txt" <<'EOF'
This file contains harbor in the body.
EOF

cat >"$FIXTURE_DIR/misc/no-hit.txt" <<'EOF'
This file should not match the current keyword set.
EOF

touch "$FIXTURE_DIR/docs/harbor-notes.bin"

cat >"$EXTRACTION_FIXTURE_DIR/docs/sample.pdf" <<'EOF'
%PDF-1.4 synthetic placeholder for stub-based pdftotext tests.
EOF

cat >"$EXTRACTION_FIXTURE_DIR/docs/legacy.doc" <<'EOF'
Synthetic legacy Office placeholder for stub-based soffice tests.
EOF

printf 'Generated FileHog fixtures in %s and %s\n' "$FIXTURE_DIR" "$EXTRACTION_FIXTURE_DIR"
