#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Build and install the MailHog and FileHog launch commands for the current user.

Usage:
  ./scripts/install-commands.sh [--bin-dir DIR]

Examples:
  ./scripts/install-commands.sh
  ./scripts/install-commands.sh --bin-dir "$HOME/bin"
EOF
}

fail() {
  echo "Error: $*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "'$1' is required"
}

require_go_124() {
  local raw_version
  local version
  local major
  local minor

  raw_version="$(go env GOVERSION 2>/dev/null || true)"
  if [[ -z "$raw_version" ]]; then
    raw_version="$(go version 2>/dev/null | awk '{print $3}')"
  fi
  [[ -n "$raw_version" ]] || fail "unable to determine Go version"

  version="${raw_version#go}"
  version="${version%%[^0-9.]*}"
  major="${version%%.*}"
  minor="${version#*.}"
  minor="${minor%%.*}"

  [[ "$major" =~ ^[0-9]+$ ]] || fail "unable to parse Go version '$raw_version'"
  [[ "$minor" =~ ^[0-9]+$ ]] || fail "unable to parse Go version '$raw_version'"

  if (( major < 1 || (major == 1 && minor < 24) )); then
    fail "Go 1.24 or newer is required; found ${raw_version}. Install Go 1.24+ and rerun the installer."
  fi
}

create_linux_desktop_entry() {
  local app_id="$1"
  local app_name="$2"
  local comment="$3"
  local exec_path="$4"
  local icon_source="$5"
  local applications_dir="${HOME}/.local/share/applications"
  local icons_dir="${HOME}/.local/share/icons/hicolor/256x256/apps"
  local desktop_file="${applications_dir}/${app_id}.desktop"
  local icon_target="${icons_dir}/${app_id}.png"

  mkdir -p "$applications_dir" "$icons_dir"
  install -m 0644 "$icon_source" "$icon_target"

  cat >"$desktop_file" <<EOF
[Desktop Entry]
Type=Application
Version=1.0
Name=${app_name}
Comment=${comment}
Exec=${exec_path}
Icon=${icon_target}
Terminal=false
Categories=Utility;Office;
EOF

  chmod 0644 "$desktop_file"
}

has_pkg_config_package() {
  pkg-config --exists "$1"
}

linux_prereq_help() {
  cat <<'EOF'
Install the missing Linux desktop build dependencies, then run the installer again.

Go 1.24 or newer is required.

Install Node plus the Linux desktop build packages:
  sudo apt update
  sudo apt install -y nodejs npm build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev

If your distro package for Go is older than 1.24, install a newer Go release from https://go.dev/dl/

Ubuntu 22.04 / Debian variants using WebKit 4.0 still need:
  sudo apt update
  sudo apt install -y nodejs npm build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.0-dev
EOF
}

detect_linux_build_tags() {
  require_cmd pkg-config

  if ! has_pkg_config_package gtk+-3.0; then
    echo >&2
    echo "Missing required package: gtk+-3.0" >&2
    echo >&2
    linux_prereq_help >&2
    exit 1
  fi

  if has_pkg_config_package webkit2gtk-4.0; then
    echo ""
    return
  fi

  if has_pkg_config_package webkit2gtk-4.1; then
    echo "webkit2_41"
    return
  fi

  echo >&2
  echo "Missing required package: webkit2gtk development files" >&2
  echo >&2
  linux_prereq_help >&2
  exit 1
}

is_on_path() {
  case ":$PATH:" in
    *":$1:"*) return 0 ;;
    *) return 1 ;;
  esac
}

default_bin_dir() {
  if [[ -d "${HOME}/bin" ]] && is_on_path "${HOME}/bin"; then
    echo "${HOME}/bin"
    return
  fi
  if [[ -d "${HOME}/.local/bin" ]] && is_on_path "${HOME}/.local/bin"; then
    echo "${HOME}/.local/bin"
    return
  fi
  echo "${HOME}/.local/bin"
}

choose_bin_dir() {
  local default_dir="$1"
  local input

  echo "Install directory for the 'mailhog' and 'filehog' commands?"
  echo "Press enter to use the default, or type a custom directory."
  echo "Default: $default_dir"
  printf "> "
  read -r input
  if [[ -n "$input" ]]; then
    BIN_DIR="$input"
  fi
}

BIN_DIR="${HOME}/.local/bin"
BIN_DIR_EXPLICIT=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bin-dir)
      [[ $# -ge 2 ]] || fail "--bin-dir requires a value"
      BIN_DIR="$2"
      BIN_DIR_EXPLICIT=1
      shift 2
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

if [[ $BIN_DIR_EXPLICIT -eq 0 ]]; then
  BIN_DIR="$(default_bin_dir)"
  if [[ -t 0 ]]; then
    choose_bin_dir "$BIN_DIR"
  fi
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MAILHOG_FRONTEND="$ROOT/mailhog/frontend"
FILEHOG_FRONTEND="$ROOT/filehog/frontend"
TMP_BIN_DIR="$ROOT/.build/bin"
BUILD_TAGS="production"

require_cmd go
require_cmd npm
require_go_124

if [[ "$(uname -s)" == "Linux" ]]; then
  EXTRA_TAGS="$(detect_linux_build_tags)"
  if [[ -n "$EXTRA_TAGS" ]]; then
    BUILD_TAGS="${BUILD_TAGS} ${EXTRA_TAGS}"
  fi
fi

mkdir -p "$BIN_DIR" "$TMP_BIN_DIR"

echo "Building MailHog frontend..."
(cd "$MAILHOG_FRONTEND" && npm install && npm run build)

echo "Building FileHog frontend..."
(cd "$FILEHOG_FRONTEND" && npm install && npm run build)

echo "Building desktop binaries..."
if [[ "$(uname -s)" == "Darwin" ]]; then
  (
    cd "$ROOT"
    CGO_LDFLAGS='-framework UniformTypeIdentifiers' go build -tags "$BUILD_TAGS" -o "$TMP_BIN_DIR/mailhog" ./mailhog
    CGO_LDFLAGS='-framework UniformTypeIdentifiers' go build -tags "$BUILD_TAGS" -o "$TMP_BIN_DIR/filehog" ./filehog
  )
else
  (
    cd "$ROOT"
    go build -tags "$BUILD_TAGS" -o "$TMP_BIN_DIR/mailhog" ./mailhog
    go build -tags "$BUILD_TAGS" -o "$TMP_BIN_DIR/filehog" ./filehog
  )
fi

install -m 0755 "$TMP_BIN_DIR/mailhog" "$BIN_DIR/mailhog"
install -m 0755 "$TMP_BIN_DIR/filehog" "$BIN_DIR/filehog"

if [[ "$(uname -s)" == "Linux" ]]; then
  create_linux_desktop_entry \
    "mailhog" \
    "MailHog" \
    "Archival email keyword hunter" \
    "${BIN_DIR}/mailhog" \
    "$ROOT/mailhog/build/appicon.png"
  create_linux_desktop_entry \
    "filehog" \
    "FileHog" \
    "Archival non-email keyword hunter" \
    "${BIN_DIR}/filehog" \
    "$ROOT/filehog/build/appicon.png"
fi

echo
echo "Installed commands:"
echo "  $BIN_DIR/mailhog"
echo "  $BIN_DIR/filehog"
if [[ "$(uname -s)" == "Linux" ]]; then
  echo
  echo "Installed desktop launchers:"
  echo "  ${HOME}/.local/share/applications/mailhog.desktop"
  echo "  ${HOME}/.local/share/applications/filehog.desktop"
fi
echo
if is_on_path "$BIN_DIR"; then
  echo "'$BIN_DIR' is already on your PATH."
else
  echo "If '$BIN_DIR' is not already on your PATH, add this line to your shell profile:"
  echo "  export PATH=\"$BIN_DIR:\$PATH\""
  echo
  echo "Then open a new shell or run:"
  echo "  export PATH=\"$BIN_DIR:\$PATH\""
fi
echo
echo "You can then launch:"
echo "  mailhog"
echo "  filehog"
