#!/usr/bin/env bash
set -euo pipefail

MAIBOT_REPO="${MAIBOT_REPO:-Mai-with-u/maibot-bootstrap}"
MAIBOT_VERSION="${MAIBOT_VERSION:-latest}"
MAIBOT_INSTALL_DIR="${MAIBOT_INSTALL_DIR:-$HOME/.local/bin}"

log() { printf '[INFO] %s\n' "$*"; }
ok() { printf '[OK] %s\n' "$*"; }
err() { printf '[ERROR] %s\n' "$*" >&2; }

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    err "missing required command: $1"
    exit 1
  fi
}

check_prereqs() {
  local -a required missing
  local cmd
  required=(git uv)
  missing=()

  for cmd in "${required[@]}"; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      missing+=("$cmd")
    fi
  done

  if [ "${#missing[@]}" -ne 0 ]; then
    err "missing required system tools: ${missing[*]}"
    log "installer can try to install them automatically after confirmation"
    if [ ! -t 0 ] || [ ! -t 1 ]; then
      err "non-interactive shell detected; cannot ask for confirmation"
      log "please install missing tools manually and rerun"
      log "macOS (Homebrew): brew install ${missing[*]}"
      log "Debian/Ubuntu: sudo apt-get update && sudo apt-get install -y ${missing[*]}"
      log "Fedora/CentOS: sudo dnf install -y ${missing[*]} || sudo yum install -y ${missing[*]}"
      exit 1
    fi

    printf 'Install missing tools now? [y/N]: '
    read -r answer
    case "${answer}" in
      y|Y|yes|YES)
        install_missing_tools "${missing[@]}"
        ;;
      *)
        err "installation aborted by user"
        exit 1
        ;;
    esac
  fi
}

run_with_privilege() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
    return
  fi
  if ! command -v sudo >/dev/null 2>&1; then
    err "sudo is required to install system packages"
    exit 1
  fi
  sudo "$@"
}

install_missing_tools() {
  local -a tools
  tools=("$@")

  if command -v brew >/dev/null 2>&1; then
    log "installing via Homebrew: ${tools[*]}"
    brew install "${tools[@]}"
    return
  fi

  if command -v apt-get >/dev/null 2>&1; then
    log "installing via apt-get: ${tools[*]}"
    run_with_privilege apt-get update -y
    run_with_privilege apt-get install -y "${tools[@]}"
    return
  fi

  if command -v dnf >/dev/null 2>&1; then
    log "installing via dnf: ${tools[*]}"
    run_with_privilege dnf install -y "${tools[@]}"
    return
  fi

  if command -v yum >/dev/null 2>&1; then
    log "installing via yum: ${tools[*]}"
    run_with_privilege yum install -y "${tools[@]}"
    return
  fi

  err "unsupported package manager; install manually: ${tools[*]}"
  exit 1
}

fetch() {
  local url="$1"
  local out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$url"
  else
    err "curl or wget is required"
    exit 1
  fi
}

detect_os() {
  local uname_s
  uname_s="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$uname_s" in
    linux*) echo "linux" ;;
    darwin*) echo "darwin" ;;
    *) err "unsupported os: $uname_s"; exit 1 ;;
  esac
}

detect_arch() {
  local uname_m
  uname_m="$(uname -m)"
  case "$uname_m" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) err "unsupported arch: $uname_m"; exit 1 ;;
  esac
}

latest_tag() {
  local api
  api="https://api.github.com/repos/${MAIBOT_REPO}/releases/latest"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$api" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1
  else
    wget -qO- "$api" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1
  fi
}

checksum_tool() {
  if command -v sha256sum >/dev/null 2>&1; then
    echo "sha256sum"
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    echo "shasum -a 256"
    return
  fi
  err "sha256sum or shasum is required"
  exit 1
}

file_sha256() {
  local file="$1"
  local tool
  tool="$(checksum_tool)"
  eval "$tool \"$file\"" | awk '{print $1}'
}

manifest_get() {
  local key="$1"
  local file="$2"
  awk -F'=' -v k="$key" '$1==k {sub(/^[^=]*=/, "", $0); print $0}' "$file" | tail -n 1
}

verify_checksum() {
  local file="$1"
  local expected="$2"
  local actual
  actual="$(file_sha256 "$file")"
  if [ "${expected}" != "${actual}" ]; then
    err "checksum mismatch"
    err "expected=${expected}"
    err "actual=${actual}"
    exit 1
  fi
}

main() {
  check_prereqs
  need_cmd mkdir
  need_cmd chmod
  need_cmd mktemp

  local os arch version asset url expected_sha manifest_url tmp_dir out_file manifest_file key_prefix installer_version
  os="$(detect_os)"
  arch="$(detect_arch)"

  if [ "$MAIBOT_VERSION" = "latest" ]; then
    version="$(latest_tag)"
    if [ -z "$version" ]; then
      err "failed to resolve latest version from GitHub"
      exit 1
    fi
  else
    version="$MAIBOT_VERSION"
  fi

  manifest_url="https://github.com/${MAIBOT_REPO}/releases/download/${version}/manifest.txt"

  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' EXIT
  manifest_file="${tmp_dir}/manifest.txt"

  log "downloading manifest (${version})"
  fetch "$manifest_url" "$manifest_file"

  installer_version="$(manifest_get installer_version "$manifest_file")"
  [ -n "$installer_version" ] && log "installer version: ${installer_version}"

  key_prefix="asset.${os}.${arch}.binary"
  asset="$(manifest_get "${key_prefix}.name" "$manifest_file")"
  expected_sha="$(manifest_get "${key_prefix}.sha256" "$manifest_file" | tr '[:upper:]' '[:lower:]')"
  url="$(manifest_get "${key_prefix}.url" "$manifest_file")"

  if [ -z "$asset" ] || [ -z "$expected_sha" ]; then
    err "manifest missing asset metadata for ${os}/${arch}"
    exit 1
  fi
  if [ -z "$url" ]; then
    url="https://github.com/${MAIBOT_REPO}/releases/download/${version}/${asset}"
  fi

  out_file="${tmp_dir}/${asset}"

  log "downloading ${asset} (${version})"
  fetch "$url" "$out_file"

  log "verifying checksum"
  verify_checksum "$out_file" "$expected_sha"

  mkdir -p "$MAIBOT_INSTALL_DIR"
  install_path="${MAIBOT_INSTALL_DIR}/maibot"
  cp "$out_file" "$install_path"
  chmod +x "$install_path"

  ok "installed: ${install_path}"
  if ! command -v maibot >/dev/null 2>&1; then
    log "add to PATH if needed: export PATH=\"${MAIBOT_INSTALL_DIR}:\$PATH\""
  fi
  log "run: maibot version"
}

main "$@"
