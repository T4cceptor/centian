#!/usr/bin/env bash
# Copyright 2025 Centian Contributors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

# Centian Installation Script
# Downloads and installs the latest release from GitHub

# Configuration
GITHUB_REPO="T4cceptor/centian"
BINARY_NAME="centian"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)     echo "linux";;
        Darwin*)    echo "darwin";;
        MINGW*|MSYS*|CYGWIN*) echo "windows";;
        *)          error "Unsupported operating system: $(uname -s)";;
    esac
}

# Detect architecture
detect_arch() {
    local arch
    arch="$(uname -m)"

    case "$arch" in
        x86_64|amd64)   echo "amd64";;
        aarch64|arm64)  echo "arm64";;
        armv7l|armv6l)  echo "arm";;
        *)              error "Unsupported architecture: $arch";;
    esac
}

# Check if required tools are available
check_dependencies() {
    local missing_deps=()

    for cmd in curl tar; do
        if ! command -v "$cmd" &> /dev/null; then
            missing_deps+=("$cmd")
        fi
    done

    if [ ${#missing_deps[@]} -ne 0 ]; then
        error "Missing required dependencies: ${missing_deps[*]}"
    fi
}

# Get latest release version from GitHub
get_latest_version() {
    local api_url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"

    info "Fetching latest release information..."

    local version
    version=$(curl -sL "$api_url" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$version" ]; then
        error "Failed to fetch latest version from GitHub"
    fi

    echo "$version"
}

# Download and extract release
download_release() {
    local version="$1"
    local os="$2"
    local arch="$3"

    # Construct download URL
    # Expected format: centian_v1.0.0_darwin_amd64.tar.gz
    local filename="${BINARY_NAME}_${version}_${os}_${arch}.tar.gz"
    local download_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/${filename}"

    info "Downloading ${filename}..."

    local tmp_dir
    tmp_dir=$(mktemp -d)
    local tar_file="${tmp_dir}/${filename}"

    if ! curl -sL -o "$tar_file" "$download_url"; then
        rm -rf "$tmp_dir"
        error "Failed to download release from $download_url"
    fi

    info "Extracting archive..."

    if ! tar -xzf "$tar_file" -C "$tmp_dir"; then
        rm -rf "$tmp_dir"
        error "Failed to extract archive"
    fi

    echo "$tmp_dir"
}

# Install binary
install_binary() {
    local tmp_dir="$1"
    local binary_path="${tmp_dir}/${BINARY_NAME}"

    if [ ! -f "$binary_path" ]; then
        error "Binary not found in extracted archive: $binary_path"
    fi

    info "Installing ${BINARY_NAME} to ${INSTALL_DIR}..."

    # Check if we need sudo
    if [ -w "$INSTALL_DIR" ]; then
        cp "$binary_path" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    else
        info "Elevated privileges required for installation to ${INSTALL_DIR}"
        sudo cp "$binary_path" "${INSTALL_DIR}/${BINARY_NAME}"
        sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    # Cleanup
    rm -rf "$tmp_dir"
}

# Verify installation
verify_installation() {
    if ! command -v "$BINARY_NAME" &> /dev/null; then
        warn "${BINARY_NAME} is installed but not in PATH"
        warn "Make sure ${INSTALL_DIR} is in your PATH"
        echo ""
        echo "Add this to your shell configuration (~/.bashrc, ~/.zshrc, etc.):"
        echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        return 1
    fi

    local installed_version
    installed_version=$("$BINARY_NAME" --version 2>/dev/null || echo "unknown")

    success "${BINARY_NAME} installed successfully!"
    info "Version: $installed_version"
    info "Location: $(command -v $BINARY_NAME)"

    return 0
}

# Main installation flow
main() {
    echo ""
    info "Centian Installation Script"
    echo ""

    # Check dependencies
    check_dependencies

    # Detect system
    local os arch
    os=$(detect_os)
    arch=$(detect_arch)

    info "Detected OS: $os"
    info "Detected Architecture: $arch"

    # Get latest version
    local version
    version=$(get_latest_version)
    info "Latest version: $version"

    # Download release
    local tmp_dir
    tmp_dir=$(download_release "$version" "$os" "$arch")

    # Install binary
    install_binary "$tmp_dir"

    # Verify installation
    echo ""
    verify_installation

    echo ""
    info "To get started, run: ${BINARY_NAME} --help"
    echo ""
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --install-dir)
            INSTALL_DIR="$2"
            shift 2
            ;;
        --help|-h)
            echo "Centian Installation Script"
            echo ""
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --install-dir DIR    Install to custom directory (default: /usr/local/bin)"
            echo "  --help, -h           Show this help message"
            echo ""
            echo "Environment Variables:"
            echo "  INSTALL_DIR          Custom installation directory"
            echo ""
            echo "Examples:"
            echo "  $0                              # Install to /usr/local/bin"
            echo "  $0 --install-dir ~/.local/bin   # Install to user directory"
            echo "  INSTALL_DIR=~/bin $0            # Install using env variable"
            exit 0
            ;;
        *)
            error "Unknown option: $1. Use --help for usage information."
            ;;
    esac
done

# Run main installation
main