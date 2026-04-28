#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Build Linux release artifacts for MailHog or FileHog.

Usage:
  build-linux-release.sh <mailhog|filehog> [--version VERSION] [--arch amd64|arm64] [--webkit2-41]

Examples:
  ./scripts/release/linux/build-linux-release.sh mailhog
  ./scripts/release/linux/build-linux-release.sh filehog --version 0.1.0
  ./scripts/release/linux/build-linux-release.sh mailhog --arch arm64 --webkit2-41
EOF
}

fail() {
  echo "Error: $*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "'$1' is required"
}

detect_arch() {
  case "$(uname -m)" in
    x86_64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      fail "unsupported Linux architecture: $(uname -m). Use --arch amd64 or --arch arm64."
      ;;
  esac
}

deb_arch_for() {
  case "$1" in
    amd64) echo "amd64" ;;
    arm64) echo "arm64" ;;
    *)
      fail "unsupported Debian architecture mapping for: $1"
      ;;
  esac
}

app_description() {
  case "$1" in
    mailhog) echo "Archival email keyword hunter" ;;
    filehog) echo "Archival non-email keyword hunter" ;;
    *)
      fail "unknown app: $1"
      ;;
  esac
}

app_long_description() {
  case "$1" in
    mailhog)
      cat <<'EOF'
MailHog searches archival email containers and message files for keywords,
including PST, OST, EML, MSG, and MBOX family formats.
EOF
      ;;
    filehog)
      cat <<'EOF'
FileHog searches archival non-email files by filename and, where supported,
by file contents across document and text-oriented formats.
EOF
      ;;
    *)
      fail "unknown app: $1"
      ;;
  esac
}

desktop_name() {
  case "$1" in
    mailhog) echo "MailHog" ;;
    filehog) echo "FileHog" ;;
    *)
      fail "unknown app: $1"
      ;;
  esac
}

APP="${1:-}"
if [[ -z "$APP" ]]; then
  usage
  exit 1
fi
shift

case "$APP" in
  mailhog|filehog) ;;
  *)
    usage
    fail "first argument must be 'mailhog' or 'filehog'"
    ;;
esac

VERSION=""
ARCH=""
EXTRA_TAGS=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      [[ $# -ge 2 ]] || fail "--version requires a value"
      VERSION="$2"
      shift 2
      ;;
    --arch)
      [[ $# -ge 2 ]] || fail "--arch requires a value"
      ARCH="$2"
      shift 2
      ;;
    --webkit2-41)
      EXTRA_TAGS="webkit2_41"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

[[ "$(uname -s)" == "Linux" ]] || fail "Linux release packaging must run on a Linux machine"

require_cmd git
require_cmd wails
require_cmd tar
require_cmd dpkg-deb

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
APP_DIR="$ROOT/$APP"
[[ -d "$APP_DIR" ]] || fail "app directory not found: $APP_DIR"

if [[ -z "$ARCH" ]]; then
  ARCH="$(detect_arch)"
fi

case "$ARCH" in
  amd64|arm64) ;;
  *)
    fail "unsupported --arch value: $ARCH"
    ;;
esac

if [[ -z "$VERSION" ]]; then
  VERSION="$(git -C "$ROOT" describe --tags --always --dirty)"
fi
SAFE_VERSION="${VERSION//\//-}"
DEB_ARCH="$(deb_arch_for "$ARCH")"

DIST_ROOT="$ROOT/releases/linux/dist/$APP/$SAFE_VERSION/$ARCH"
TMP_ROOT="$ROOT/releases/linux/tmp/$APP-$SAFE_VERSION-$ARCH"
TARBALL_ROOT="$TMP_ROOT/${APP}-${SAFE_VERSION}-linux-${ARCH}"
PACKAGE_ROOT="$TMP_ROOT/${APP}_${SAFE_VERSION}_${DEB_ARCH}"
PACKAGE_STAGING="$PACKAGE_ROOT/opt/$APP"
DEBIAN_ROOT="$PACKAGE_ROOT/DEBIAN"
DESKTOP_DIR="$PACKAGE_ROOT/usr/share/applications"
ICON_DIR="$PACKAGE_ROOT/usr/share/pixmaps"
BIN_LINK_DIR="$PACKAGE_ROOT/usr/bin"

rm -rf "$TMP_ROOT"
mkdir -p "$DIST_ROOT" "$TARBALL_ROOT" "$PACKAGE_STAGING" "$DEBIAN_ROOT" "$DESKTOP_DIR" "$ICON_DIR" "$BIN_LINK_DIR"

BUILD_CMD=(wails build --clean --platform "linux/$ARCH" --no-package)
if [[ -n "$EXTRA_TAGS" ]]; then
  BUILD_CMD+=(--tags "$EXTRA_TAGS")
fi

echo "Building $APP for linux/$ARCH..."
(cd "$APP_DIR" && "${BUILD_CMD[@]}")

BINARY_PATH="$APP_DIR/build/bin/$APP"
[[ -f "$BINARY_PATH" ]] || fail "expected binary not found after build: $BINARY_PATH"

cp "$BINARY_PATH" "$TARBALL_ROOT/$APP"
cp "$BINARY_PATH" "$PACKAGE_STAGING/$APP"
cp "$APP_DIR/build/appicon.png" "$TARBALL_ROOT/${APP}.png"
cp "$APP_DIR/build/appicon.png" "$ICON_DIR/${APP}.png"
chmod 0755 "$TARBALL_ROOT/$APP" "$PACKAGE_STAGING/$APP"

cat >"$TARBALL_ROOT/README-linux.txt" <<EOF
$(desktop_name "$APP") Linux release
Version: $SAFE_VERSION
Architecture: $ARCH

This archive contains the native Linux desktop binary for $APP.

Run:
  ./$APP

Notes:
  - Built from this repository using Wails on Linux.
  - Some scan features may require extra helper tools on the target machine.
  - For Debian/Ubuntu deployment, use the matching .deb package instead.
EOF

ln -s "/opt/$APP/$APP" "$BIN_LINK_DIR/$APP"
cp "$ROOT/packaging/linux/$APP/$APP.desktop" "$DESKTOP_DIR/$APP.desktop"

INSTALLED_SIZE="$(du -sk "$PACKAGE_ROOT" | awk '{print $1}')"
LONG_DESC="$(app_long_description "$APP" | sed 's/^/ /')"
cat >"$DEBIAN_ROOT/control" <<EOF
Package: $APP
Version: $SAFE_VERSION
Section: utils
Priority: optional
Architecture: $DEB_ARCH
Maintainer: Bryan Snyder <noreply@example.com>
Depends: libgtk-3-0, libwebkit2gtk-4.0-37 | libwebkit2gtk-4.1-0
Installed-Size: $INSTALLED_SIZE
Description: $(app_description "$APP")
$LONG_DESC
EOF

TARBALL_PATH="$DIST_ROOT/${APP}-${SAFE_VERSION}-linux-${ARCH}.tar.gz"
DEB_PATH="$DIST_ROOT/${APP}_${SAFE_VERSION}_${DEB_ARCH}.deb"

tar -C "$TMP_ROOT" -czf "$TARBALL_PATH" "$(basename "$TARBALL_ROOT")"
dpkg-deb --build "$PACKAGE_ROOT" "$DEB_PATH" >/dev/null

echo
echo "Artifacts ready:"
echo "  $TARBALL_PATH"
echo "  $DEB_PATH"
