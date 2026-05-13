#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FIXTURE_DIR="$ROOT/testing/mailhog/generated/basic-eml"
ARCHIVE_FIXTURE_DIR="$ROOT/testing/mailhog/generated/archive-placeholders"

rm -rf "$FIXTURE_DIR"
rm -rf "$ARCHIVE_FIXTURE_DIR"
mkdir -p "$FIXTURE_DIR/Inbox" "$FIXTURE_DIR/Sent"
mkdir -p "$ARCHIVE_FIXTURE_DIR"

cat >"$FIXTURE_DIR/Inbox/hit.eml" <<'EOF'
From: archive@example.com
To: review@example.com
Date: Tue, 02 Jan 2024 10:00:00 -0500
Subject: Harbor follow-up

Harbor appears in the body.
EOF

cat >"$FIXTURE_DIR/Sent/miss.eml" <<'EOF'
From: archive@example.com
To: review@example.com
Date: Tue, 02 Jan 2024 11:00:00 -0500
Subject: Routine note

No tracked keyword appears here.
EOF

cat >"$ARCHIVE_FIXTURE_DIR/sample.pst" <<'EOF'
Synthetic PST placeholder for stub-based scanner tests.
EOF

cat >"$ARCHIVE_FIXTURE_DIR/sample.ost" <<'EOF'
Synthetic OST placeholder for stub-based scanner tests.
EOF

cat >"$ARCHIVE_FIXTURE_DIR/sample.msg" <<'EOF'
Synthetic MSG placeholder for stub-based scanner tests.
EOF

printf 'Generated MailHog fixtures in %s and %s\n' "$FIXTURE_DIR" "$ARCHIVE_FIXTURE_DIR"
