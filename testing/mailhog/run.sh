#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

"$ROOT/testing/mailhog/generate-fixtures.sh" >/dev/null
(cd "$ROOT" && go test ./scanner)
