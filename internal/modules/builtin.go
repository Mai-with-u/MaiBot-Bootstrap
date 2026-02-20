package modules

import "maibot/internal/config"

func BuiltinDefinitions() []config.ModuleDefinition {
	return []config.ModuleDefinition{
		{
			Name:        "napcat",
			Description: "Install NapCat runtime, LinuxQQ, launcher helper into workspace",
			Install: []config.ModuleStep{
				{
					Name:    "prepare workspace directories",
					Command: "bash",
					Args: []string{"-lc", `set -euo pipefail
INSTALL_DIR="$PWD/modules/napcat"
mkdir -p "$INSTALL_DIR"
mkdir -p "$INSTALL_DIR/tmp"
echo "NapCat install dir: $INSTALL_DIR"`},
				},
				{
					Name:        "install dependencies (apt/dnf)",
					Command:     "bash",
					RequireSudo: true,
					Sensitive:   true,
					Prompt:      "Install system dependencies for NapCat?",
					Args: []string{"-lc", `set -euo pipefail
if command -v apt-get >/dev/null 2>&1; then
  DEBIAN_FRONTEND=noninteractive apt-get update -y -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq zip unzip jq curl xvfb screen xauth procps g++
elif command -v dnf >/dev/null 2>&1; then
  dnf install -y epel-release
  dnf install --allowerasing -y zip unzip jq curl xorg-x11-server-Xvfb screen procps-ng gcc-c++
else
  echo "Unsupported package manager. Only apt-get/dnf are supported." >&2
  exit 1
fi`},
				},
				{
					Name:    "download and unpack napcat shell",
					Command: "bash",
					Args: []string{"-lc", `set -euo pipefail
INSTALL_DIR="$PWD/modules/napcat"
cd "$INSTALL_DIR"
ZIP_FILE="NapCat.Shell.zip"
target_proxy="${MAIBOT_PROXY_PREFIX:-}"

if [ -f "$ZIP_FILE" ]; then
  echo "reuse existing $ZIP_FILE"
else
  direct_url="https://github.com/NapNeko/NapCatQQ/releases/latest/download/NapCat.Shell.zip"
  if [ -n "$target_proxy" ]; then
    mirror_url="${target_proxy}/${direct_url#https://}"
    curl -k -L -f "$mirror_url" -o "$ZIP_FILE" || curl -k -L -f "$direct_url" -o "$ZIP_FILE"
  else
    curl -k -L -f "$direct_url" -o "$ZIP_FILE"
  fi
fi

unzip -t "$ZIP_FILE" >/dev/null
unzip -q -o "$ZIP_FILE" -d "$INSTALL_DIR"`},
				},
				{
					Name:        "install linuxqq package",
					Command:     "bash",
					RequireSudo: true,
					Sensitive:   true,
					Prompt:      "Install LinuxQQ package required by NapCat?",
					Args: []string{"-lc", `set -euo pipefail
INSTALL_DIR="$PWD/modules/napcat"
cd "$INSTALL_DIR"

arch=$(arch | sed 's/aarch64/arm64/' | sed 's/x86_64/amd64/')
if [ "$arch" = "amd64" ]; then
  if command -v dnf >/dev/null 2>&1; then
    qq_url="https://dldir1.qq.com/qqfile/qq/QQNT/8015ff90/linuxqq_3.2.21-42086_x86_64.rpm"
    curl -k -L -f "$qq_url" -o QQ.rpm
    dnf localinstall -y ./QQ.rpm
    rm -f QQ.rpm
  elif command -v apt-get >/dev/null 2>&1; then
    qq_url="https://dldir1.qq.com/qqfile/qq/QQNT/8015ff90/linuxqq_3.2.21-42086_amd64.deb"
    curl -k -L -f "$qq_url" -o QQ.deb
    DEBIAN_FRONTEND=noninteractive apt-get install -f -y --allow-downgrades -qq ./QQ.deb
    DEBIAN_FRONTEND=noninteractive apt-get install -y --allow-downgrades -qq libnss3 libgbm1
    DEBIAN_FRONTEND=noninteractive apt-get install -y --allow-downgrades -qq libasound2 || DEBIAN_FRONTEND=noninteractive apt-get install -y --allow-downgrades -qq libasound2t64
    rm -f QQ.deb
  else
    echo "Unsupported package manager. Only apt-get/dnf are supported." >&2
    exit 1
  fi
elif [ "$arch" = "arm64" ]; then
  if command -v dnf >/dev/null 2>&1; then
    qq_url="https://dldir1.qq.com/qqfile/qq/QQNT/8015ff90/linuxqq_3.2.21-42086_aarch64.rpm"
    curl -k -L -f "$qq_url" -o QQ.rpm
    dnf localinstall -y ./QQ.rpm
    rm -f QQ.rpm
  elif command -v apt-get >/dev/null 2>&1; then
    qq_url="https://dldir1.qq.com/qqfile/qq/QQNT/8015ff90/linuxqq_3.2.21-42086_arm64.deb"
    curl -k -L -f "$qq_url" -o QQ.deb
    DEBIAN_FRONTEND=noninteractive apt-get install -f -y --allow-downgrades -qq ./QQ.deb
    DEBIAN_FRONTEND=noninteractive apt-get install -y --allow-downgrades -qq libnss3 libgbm1
    DEBIAN_FRONTEND=noninteractive apt-get install -y --allow-downgrades -qq libasound2 || DEBIAN_FRONTEND=noninteractive apt-get install -y --allow-downgrades -qq libasound2t64
    rm -f QQ.deb
  else
    echo "Unsupported package manager. Only apt-get/dnf are supported." >&2
    exit 1
  fi
else
  echo "Unsupported architecture: $arch" >&2
  exit 1
fi`},
				},
				{
					Name:    "build launcher and write startup script",
					Command: "bash",
					Args: []string{"-lc", `set -euo pipefail
INSTALL_DIR="$PWD/modules/napcat"
cd "$INSTALL_DIR"

cpp_url="https://raw.githubusercontent.com/NapNeko/napcat-linux-launcher/refs/heads/main/launcher.cpp"
download_url="$cpp_url"
if [ -n "${MAIBOT_PROXY_PREFIX:-}" ]; then
  download_url="${MAIBOT_PROXY_PREFIX}/${cpp_url#https://}"
fi

curl -k -L -f "$download_url" -o launcher.cpp || curl -k -L -f "$cpp_url" -o launcher.cpp
g++ -shared -fPIC launcher.cpp -o libnapcat_launcher.so -ldl

cat > launcher.sh <<'EOF'
#!/bin/bash
Xvfb :1 -screen 0 1x1x8 +extension GLX +render > /dev/null 2>&1 &
export DISPLAY=:1
trap "" SIGPIPE
LD_PRELOAD=./libnapcat_launcher.so qq --no-sandbox
EOF

chmod +x launcher.sh
echo "NapCat installed. Start with: cd $INSTALL_DIR && sudo bash ./launcher.sh"`},
				},
			},
		},
		{
			Name:        "adapter-example",
			Description: "Built-in adapter installer example",
			Install: []config.ModuleStep{
				{
					Name:    "install adapter",
					Command: "bash",
					Args:    []string{"-lc", "echo 'TODO: replace with real adapter install command'"},
				},
			},
		},
	}
}
