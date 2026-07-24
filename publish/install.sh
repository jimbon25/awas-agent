#!/usr/bin/env bash

set -euo pipefail

OWNER="jimbon25"
REPO="awas-agent"

OS_RAW="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "${OS_RAW}" in
    linux*)   OS="linux" ;;
    darwin*)  OS="darwin" ;;
    msys*|mingw*|cygwin*) OS="windows" ;;
    *)
        echo "error: Unsupported Operating System: ${OS_RAW}" >&2
        exit 1
        ;;
esac

ARCH_RAW="$(uname -m)"
case "${ARCH_RAW}" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)
        echo "error: Unsupported CPU Architecture: ${ARCH_RAW}" >&2
        exit 1
        ;;
esac

echo "info: Detected platform: ${OS} (${ARCH})"

echo "info: Checking latest version on GitHub..."
LATEST_TAG=$(curl -s "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "${LATEST_TAG}" ]; then
    echo "error: Could not retrieve latest release tag." >&2
    exit 1
fi

VERSION="${LATEST_TAG#v}"
echo "info: Latest release version: v${VERSION}"

FILE_EXT="tar.gz"
if [ "${OS}" = "windows" ]; then
    FILE_EXT="zip"
fi

FILE_NAME="awas_${VERSION}_${OS}_${ARCH}.${FILE_EXT}"
DOWNLOAD_URL="https://github.com/${OWNER}/${REPO}/releases/download/${LATEST_TAG}/${FILE_NAME}"

TEMP_DIR=$(mktemp -d)
clean_up() {
    rm -rf "${TEMP_DIR}"
}
trap clean_up EXIT

echo "info: Downloading release archive..."
curl -L -o "${TEMP_DIR}/${FILE_NAME}" "${DOWNLOAD_URL}"

echo "info: Extracting package..."
cd "${TEMP_DIR}"
if [ "${FILE_EXT}" = "zip" ]; then
    unzip -q "${FILE_NAME}"
else
    tar -xzf "${FILE_NAME}"
fi

chmod +x awas

INSTALL_DIR="/usr/local/bin"
if [ ! -w "${INSTALL_DIR}" ]; then
    echo "warning: Write permission denied for ${INSTALL_DIR}. Attempting to install to ~/.local/bin..."
    INSTALL_DIR="${HOME}/.local/bin"
    mkdir -p "${INSTALL_DIR}"
    
    if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        echo "warning: ${INSTALL_DIR} is not in your PATH. Please add it to your profile configuration."
    fi
fi

echo "info: Installing binary to ${INSTALL_DIR}/awas..."
cp awas "${INSTALL_DIR}/awas"

echo "success: AWAS CLI version v${VERSION} has been successfully installed."
echo "info: Run 'awas --help' to get started."
