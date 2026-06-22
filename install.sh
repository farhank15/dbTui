#!/bin/sh
# dbTui automated installer script
set -e

REPOS="farhank15/dbTui"

# Print colored messages
info() {
  printf "\033[36m[dbTui]\033[0m %s\n" "$*"
}

warn() {
  printf "\033[33m[warn]\033[0m %s\n" "$*"
}

error() {
  printf "\033[31m[error]\033[0m %s\n" "$*"
  exit 1
}

# 1. Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux*)   OS="linux" ;;
  darwin*)  OS="darwin" ;;
  msys*|mingw*|cygwin*|windows*) OS="windows" ;;
  *) error "Unsupported operating system: $OS" ;;
esac

# 2. Detect Architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="x86_64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) error "Unsupported architecture: $ARCH" ;;
esac

# 3. Retrieve Latest Version
info "Fetching latest version from GitHub..."
LATEST_RELEASE=$(curl -s "https://api.github.com/repos/${REPOS}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST_RELEASE" ]; then
  LATEST_RELEASE="v1.0.0"
  warn "Failed to fetch latest tag via GitHub API, falling back to ${LATEST_RELEASE}."
fi

FORMAT="tar.gz"
if [ "$OS" = "windows" ]; then
  FORMAT="zip"
fi

FILE_NAME="dbTui_${OS}_${ARCH}.${FORMAT}"
URL="https://github.com/real-farhank15/dbTui/releases/download/${LATEST_RELEASE}/${FILE_NAME}" # Update to match organization/repo if needed

# 4. Create temp workspace
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TEMP_DIR}"' EXIT

info "Downloading dbTui ${LATEST_RELEASE} ($OS/$ARCH)..."
curl -sSL -o "${TEMP_DIR}/${FILE_NAME}" "https://github.com/${REPOS}/releases/download/${LATEST_RELEASE}/${FILE_NAME}"

info "Extracting binary..."
if [ "$FORMAT" = "zip" ]; then
  unzip -q "${TEMP_DIR}/${FILE_NAME}" -d "${TEMP_DIR}"
else
  tar -xzf "${TEMP_DIR}/${FILE_NAME}" -C "${TEMP_DIR}"
fi

BINARY_NAME="dbTui"
if [ "$OS" = "windows" ]; then
  BINARY_NAME="dbTui.exe"
fi

# 5. Check binary
if [ ! -f "${TEMP_DIR}/${BINARY_NAME}" ]; then
  error "Failed to locate binary after extraction."
fi

# 6. Install path selection
INSTALL_DIR="/usr/local/bin"

if [ ! -w "$INSTALL_DIR" ]; then
  warn "Write permission denied for ${INSTALL_DIR}."
  INSTALL_DIR="${HOME}/.local/bin"
  info "Installing to user-local directory instead: ${INSTALL_DIR}"
  mkdir -p "${INSTALL_DIR}"
else
  info "Installing to ${INSTALL_DIR}..."
fi

mv "${TEMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

info "Successfully installed dbTui to ${INSTALL_DIR}/${BINARY_NAME}! 🎉"
if [ "$INSTALL_DIR" = "${HOME}/.local/bin" ]; then
  warn "Make sure '${HOME}/.local/bin' is included in your PATH variable."
fi
