#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-dev}"
OUT_DIR="${OUT_DIR:-dist/${VERSION}}"
MAIBOT_REPO="${MAIBOT_REPO:-Mai-with-u/maibot-bootstrap}"
BASE_URL="${BASE_URL:-https://github.com/${MAIBOT_REPO}/releases/download/${VERSION}}"

INSTALLER_VERSION="$(sed -n 's/^const InstallerVersion = "\([^"]*\)"/\1/p' internal/version/version.go)"
if [ -z "$INSTALLER_VERSION" ]; then
  echo "Failed to read installer version from internal/version/version.go" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

build_target() {
  local goos="$1"
  local goarch="$2"
  local out_name="maibot_${goos}_${goarch}"
  if [ "$goos" = "windows" ]; then
    out_name+=".exe"
  fi

  echo "Building ${out_name}"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -ldflags="-s -w" -o "${OUT_DIR}/${out_name}" ./cmd/maibot
}

build_target linux amd64
build_target linux arm64
build_target darwin amd64
build_target darwin arm64
build_target windows amd64
build_target windows arm64

(
  cd "$OUT_DIR"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum maibot_* > checksums.txt
  else
    shasum -a 256 maibot_* > checksums.txt
  fi
)

manifest_path="${OUT_DIR}/manifest.txt"
{
  echo "version=${VERSION}"
  echo "installer_version=${INSTALLER_VERSION}"

  while IFS= read -r line; do
    [ -z "$line" ] && continue
    checksum="$(printf '%s' "$line" | awk '{print $1}')"
    filename="$(printf '%s' "$line" | awk '{print $2}')"

    os=""
    arch=""
    case "$filename" in
      maibot_linux_amd64)
        os="linux"; arch="amd64" ;;
      maibot_linux_arm64)
        os="linux"; arch="arm64" ;;
      maibot_darwin_amd64)
        os="darwin"; arch="amd64" ;;
      maibot_darwin_arm64)
        os="darwin"; arch="arm64" ;;
      maibot_windows_amd64.exe)
        os="windows"; arch="amd64" ;;
      maibot_windows_arm64.exe)
        os="windows"; arch="arm64" ;;
      *)
        continue ;;
    esac

    key="asset.${os}.${arch}.binary"
    echo "${key}.name=${filename}"
    echo "${key}.url=${BASE_URL}/${filename}"
    echo "${key}.sha256=${checksum}"
  done < "${OUT_DIR}/checksums.txt"
} > "$manifest_path"

echo "Artifacts created in ${OUT_DIR}"
