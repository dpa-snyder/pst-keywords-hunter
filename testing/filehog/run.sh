#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

"$ROOT/testing/filehog/generate-fixtures.sh" >/dev/null
(cd "$ROOT" && go test ./filescanner)
