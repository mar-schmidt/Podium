#!/usr/bin/env bash
set -euo pipefail

VERSION="latest"
INSTALL_DIR="${HOME}/.local/bin"
PODIUM_HOME_VALUE="${PODIUM_HOME:-}"
AUTOSTART="ask"
RUN_ONBOARD="yes"
SOURCE_FALLBACK="no"
DRY_RUN="no"
RELEASE_BASE="${PODIUM_RELEASE_BASE:-https://github.com/mar-schmidt/Podium/releases}"
REPO_URL="${PODIUM_REPO_URL:-https://github.com/mar-schmidt/Podium.git}"

usage() {
  cat <<'USAGE'
Install Podium.

Usage:
  curl -fsSL https://podium.ai/install.sh | bash
  bash install.sh [--version VERSION] [--install-dir DIR] [--podium-home DIR]
                  [--autostart ask|yes|no] [--no-onboard]
                  [--source-fallback] [--dry-run]
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) VERSION="${2:?missing version}"; shift 2 ;;
    --install-dir) INSTALL_DIR="${2:?missing dir}"; shift 2 ;;
    --podium-home) PODIUM_HOME_VALUE="${2:?missing dir}"; shift 2 ;;
    --autostart) AUTOSTART="${2:?missing value}"; shift 2 ;;
    --no-onboard) RUN_ONBOARD="no"; shift ;;
    --source-fallback) SOURCE_FALLBACK="yes"; shift ;;
    --dry-run) DRY_RUN="yes"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

case "$AUTOSTART" in ask|yes|no) ;; *) echo "--autostart must be ask, yes, or no" >&2; exit 2 ;; esac

say() { printf '%s\n' "$*"; }
run() {
  if [ "$DRY_RUN" = "yes" ]; then
    printf '[dry-run] %s\n' "$*"
  else
    "$@"
  fi
}

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  darwin) os="darwin" ;;
  linux) os="linux" ;;
  *) echo "Unsupported OS: $os" >&2; exit 1 ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

tmp="$(mktemp -d)"
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT

if [ "$VERSION" = "latest" ]; then
  release_url="${RELEASE_BASE}/latest/download"
  archive="podium_${os}_${arch}.tar.gz"
else
  release_url="${RELEASE_BASE}/download/${VERSION}"
  archive="podium_${VERSION}_${os}_${arch}.tar.gz"
fi
url="${release_url}/${archive}"
sum_url="${release_url}/SHA256SUMS"

download_release() {
  say "Downloading ${url}"
  if [ "$DRY_RUN" = "yes" ]; then
    say "[dry-run] curl -fL $url -o $tmp/$archive"
    say "[dry-run] curl -fL $sum_url -o $tmp/SHA256SUMS"
    run mkdir -p "$INSTALL_DIR"
    say "[dry-run] verify checksum and unpack $archive"
    say "[dry-run] install podium and podiumd into $INSTALL_DIR"
    return 0
  fi
  curl -fL "$url" -o "$tmp/$archive" || return 1
  curl -fL "$sum_url" -o "$tmp/SHA256SUMS" || return 1
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$tmp" && grep " ${archive}$" SHA256SUMS | sha256sum -c -) || return 1
  else
    expected="$(grep " ${archive}$" "$tmp/SHA256SUMS" | awk '{print $1}')"
    actual="$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')"
    [ "$expected" = "$actual" ] || return 1
  fi
  mkdir -p "$tmp/unpack"
  tar -xzf "$tmp/$archive" -C "$tmp/unpack" || return 1
  run mkdir -p "$INSTALL_DIR"
  run install -m 0755 "$tmp/unpack/podium" "$INSTALL_DIR/podium"
  run install -m 0755 "$tmp/unpack/podiumd" "$INSTALL_DIR/podiumd"
}

build_from_source() {
  say "Building Podium from source fallback."
  work="$tmp/src"
  if [ -f "go.mod" ] && [ -d "cmd/podium" ]; then
    work="$(pwd)"
  else
    git clone --depth 1 "$REPO_URL" "$work"
  fi
  (cd "$work" && make build)
  run mkdir -p "$INSTALL_DIR"
  run install -m 0755 "$work/bin/podium" "$INSTALL_DIR/podium"
  run install -m 0755 "$work/bin/podiumd" "$INSTALL_DIR/podiumd"
}

if ! download_release; then
  if [ "$SOURCE_FALLBACK" = "yes" ]; then
    build_from_source
  else
    echo "Release download failed. Re-run with --source-fallback to build locally." >&2
    exit 1
  fi
fi

if [ -n "$PODIUM_HOME_VALUE" ]; then
  export PODIUM_HOME="$PODIUM_HOME_VALUE"
fi

install_autostart() {
  podiumd_path="$INSTALL_DIR/podiumd"
  if [ "$os" = "darwin" ]; then
    plist="$HOME/Library/LaunchAgents/com.podium.podiumd.plist"
    run mkdir -p "$(dirname "$plist")"
    if [ "$DRY_RUN" = "yes" ]; then
      say "[dry-run] write $plist"
    else
      cat > "$plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>com.podium.podiumd</string>
  <key>ProgramArguments</key><array><string>${podiumd_path}</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
$(if [ -n "${PODIUM_HOME_VALUE}" ]; then printf '  <key>EnvironmentVariables</key><dict><key>PODIUM_HOME</key><string>%s</string></dict>\n' "$PODIUM_HOME_VALUE"; fi)
  <key>StandardOutPath</key><string>${HOME}/Library/Logs/podiumd.log</string>
  <key>StandardErrorPath</key><string>${HOME}/Library/Logs/podiumd.err.log</string>
</dict></plist>
PLIST
      launchctl unload "$plist" >/dev/null 2>&1 || true
      launchctl load "$plist"
    fi
  elif command -v systemctl >/dev/null 2>&1 && systemctl --user show-environment >/dev/null 2>&1; then
    unit="$HOME/.config/systemd/user/podium.service"
    run mkdir -p "$(dirname "$unit")"
    if [ "$DRY_RUN" = "yes" ]; then
      say "[dry-run] write $unit"
    else
      {
        echo "[Unit]"
        echo "Description=Podium daemon"
        echo
        echo "[Service]"
        echo "ExecStart=${podiumd_path}"
        [ -n "$PODIUM_HOME_VALUE" ] && echo "Environment=PODIUM_HOME=${PODIUM_HOME_VALUE}"
        echo "Restart=always"
        echo
        echo "[Install]"
        echo "WantedBy=default.target"
      } > "$unit"
      systemctl --user daemon-reload
      systemctl --user enable --now podium.service
    fi
  else
    say "systemd --user is not available; skipping autostart."
  fi
}

if [ "$AUTOSTART" = "ask" ]; then
  printf 'Start Podium automatically when you log in? [Y/n] '
  read -r reply || reply=""
  case "${reply:-Y}" in y|Y|yes|YES|"") AUTOSTART="yes" ;; *) AUTOSTART="no" ;; esac
fi
[ "$AUTOSTART" = "yes" ] && install_autostart

say "Podium installed to ${INSTALL_DIR}."
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) say "Add ${INSTALL_DIR} to PATH if your shell cannot find podium." ;;
esac

if [ "$RUN_ONBOARD" = "yes" ]; then
  PATH="$INSTALL_DIR:$PATH" run "$INSTALL_DIR/podium" onboard
fi
