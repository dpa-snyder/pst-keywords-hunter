#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FIXTURE_DIR="$ROOT/testing/mailhog/generated/basic-eml"

rm -rf "$FIXTURE_DIR"
mkdir -p "$FIXTURE_DIR/Inbox" "$FIXTURE_DIR/Sent"

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

printf 'Generated MailHog fixtures in %s\n' "$FIXTURE_DIR"
