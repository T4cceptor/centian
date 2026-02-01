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

set -euo pipefail

# Centian Installation Script
# Downloads and installs the latest release from GitHub

# Configuration
GITHUB_REPO="T4cceptor/centian"
BINARY_NAME="centian"
INSTALL_VERSION="${INSTALL_VERSION:-}"

# Set default install dir based on OS (can be overridden by INSTALL_DIR env var)
# Prefers user-local directories to avoid requiring sudo
get_default_install_dir() {
    case "$(uname -s)" in
        MINGW*|MSYS*|CYGWIN*) echo "${LOCALAPPDATA:-$HOME/AppData/Local}/Programs/centian";;
        *)                     echo "${HOME}/.local/bin";;
    esac
}
INSTALL_DIR="${INSTALL_DIR:-$(get_default_install_dir)}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions (output to stderr so they don't interfere with command substitution)
info() {
    echo -e "${BLUE}[INFO]${NC} $1" >&2
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" >&2
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" >&2
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
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
    local os="$1"
    local missing_deps=()

    # curl is always required
    if ! command -v "curl" &> /dev/null; then
        missing_deps+=("curl")
    fi

    # Windows needs unzip, others need tar
    if [ "$os" = "windows" ]; then
        if ! command -v "unzip" &> /dev/null; then
            missing_deps+=("unzip")
        fi
    else
        if ! command -v "tar" &> /dev/null; then
            missing_deps+=("tar")
        fi
    fi

    if [ ${#missing_deps[@]} -ne 0 ]; then
        error "Missing required dependencies: ${missing_deps[*]}"
    fi
}

# Get latest release version from GitHub
get_latest_version() {
    local api_url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
    local version
    version=$(curl -fsSL "$api_url" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
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
    # Expected format: centian_v1.0.0_darwin_amd64.tar.gz (or .zip for Windows)
    local ext="tar.gz"
    if [ "$os" = "windows" ]; then
        ext="zip"
    fi
    local filename="${BINARY_NAME}_${version}_${os}_${arch}.${ext}"
    local download_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/${filename}"

    info "Downloading ${filename}..."
    info "URL: ${download_url}"

    local tmp_dir
    tmp_dir=$(mktemp -d)
    local archive_file="${tmp_dir}/${filename}"

    if ! curl -fsSL -o "$archive_file" "$download_url"; then
        rm -rf "$tmp_dir"
        error "Failed to download release asset from $download_url. Ensure the release exists and includes ${filename}."
    fi

    if [ ! -s "$archive_file" ]; then
        rm -rf "$tmp_dir"
        error "Downloaded archive is empty: $download_url"
    fi

    info "Extracting archive..."

    if [ "$os" = "windows" ]; then
        if ! unzip -q "$archive_file" -d "$tmp_dir"; then
            rm -rf "$tmp_dir"
            error "Failed to extract archive"
        fi
    else
        if ! tar -xzf "$archive_file" -C "$tmp_dir"; then
            rm -rf "$tmp_dir"
            error "Failed to extract archive"
        fi
    fi

    echo "$tmp_dir"
}

# Install binary
install_binary() {
    local tmp_dir="$1"
    local os="$2"

    # Determine binary name (with .exe on Windows)
    local src_binary="${BINARY_NAME}"
    local dest_binary="${BINARY_NAME}"
    if [ "$os" = "windows" ]; then
        src_binary="${BINARY_NAME}.exe"
        dest_binary="${BINARY_NAME}.exe"
    fi

    local binary_path="${tmp_dir}/${src_binary}"

    if [ ! -f "$binary_path" ]; then
        error "Binary not found in extracted archive: $binary_path"
    fi

    info "Installing ${dest_binary} to ${INSTALL_DIR}..."

    # Ensure install dir exists
    if [ ! -d "$INSTALL_DIR" ]; then
        if [ "$os" = "windows" ]; then
            mkdir -p "$INSTALL_DIR"
        elif [ -w "$(dirname "$INSTALL_DIR")" ]; then
            mkdir -p "$INSTALL_DIR"
        else
            info "Elevated privileges required to create ${INSTALL_DIR}"
            sudo mkdir -p "$INSTALL_DIR"
        fi
    fi

    # Check if we need sudo (not on Windows)
    if [ "$os" = "windows" ] || [ -w "$INSTALL_DIR" ]; then
        cp "$binary_path" "${INSTALL_DIR}/${dest_binary}"
        if [ "$os" != "windows" ]; then
            chmod +x "${INSTALL_DIR}/${dest_binary}"
        fi
    else
        info "Elevated privileges required for installation to ${INSTALL_DIR}"
        sudo cp "$binary_path" "${INSTALL_DIR}/${dest_binary}"
        sudo chmod +x "${INSTALL_DIR}/${dest_binary}"
    fi

    # Cleanup
    rm -rf "$tmp_dir"
}

# Verify installation
verify_installation() {
    local os="$1"
    local binary_cmd="${BINARY_NAME}"
    if [ "$os" = "windows" ]; then
        binary_cmd="${BINARY_NAME}.exe"
    fi

    if ! command -v "$binary_cmd" &> /dev/null; then
        warn "${binary_cmd} is installed but not in PATH"
        warn "Make sure ${INSTALL_DIR} is in your PATH"
        echo ""
        if [ "$os" = "windows" ]; then
            echo "Add ${INSTALL_DIR} to your PATH environment variable"
        else
            echo "Add this to your shell configuration (~/.bashrc, ~/.zshrc, etc.):"
            echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        fi
        return 1
    fi

    local installed_version
    installed_version=$("$binary_cmd" --version 2>/dev/null || echo "unknown")

    success "${binary_cmd} installed successfully!"
    info "Version: $installed_version"
    info "Location: $(command -v $binary_cmd)"

    return 0
}

# Verify checksum
verify_checksum() {
    local tmp_dir="$1"
    local version="$2"
    local os="$3"
    local arch="$4"

    local checksums_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/checksums.txt"
    local checksums_file="${tmp_dir}/checksums.txt"

    info "Verifying checksum..."

    if ! curl -fsSL -o "$checksums_file" "$checksums_url" 2>/dev/null; then
        warn "Checksums file not available, skipping verification"
        return 0
    fi

    # Determine the archive filename
    local ext="tar.gz"
    if [ "$os" = "windows" ]; then
        ext="zip"
    fi
    local filename="${BINARY_NAME}_${version}_${os}_${arch}.${ext}"
    local archive_file="${tmp_dir}/${filename}"

    # Get expected checksum from checksums file
    local expected_checksum
    expected_checksum=$(grep "${filename}" "$checksums_file" | awk '{print $1}')

    if [ -z "$expected_checksum" ]; then
        warn "Checksum for ${filename} not found in checksums.txt, skipping verification"
        return 0
    fi

    # Calculate actual checksum
    local actual_checksum
    if command -v sha256sum &> /dev/null; then
        actual_checksum=$(sha256sum "$archive_file" | awk '{print $1}')
    elif command -v shasum &> /dev/null; then
        actual_checksum=$(shasum -a 256 "$archive_file" | awk '{print $1}')
    else
        warn "Neither sha256sum nor shasum available, skipping checksum verification"
        return 0
    fi

    if [ "$expected_checksum" != "$actual_checksum" ]; then
        error "Checksum verification failed! Expected: ${expected_checksum}, Got: ${actual_checksum}"
    fi

    success "Checksum verified"
}

# Main installation flow
main() {
    echo ""
    info "Centian Installation Script"
    echo ""

    # Detect system first (needed for dependency check)
    local os arch
    os=$(detect_os)
    arch=$(detect_arch)

    # Check dependencies (pass os for platform-specific checks)
    check_dependencies "$os"

    info "Detected OS: $os"
    info "Detected Architecture: $arch"

    # Get version
    local version
    if [ -n "$INSTALL_VERSION" ]; then
        version="$INSTALL_VERSION"
        info "Using specified version: $version"
    else
        info "Fetching latest release information..."
        version=$(get_latest_version)
        info "Latest version: $version"
    fi

    # Download release
    local tmp_dir
    tmp_dir=$(download_release "$version" "$os" "$arch")

    # Verify checksum
    verify_checksum "$tmp_dir" "$version" "$os" "$arch"

    # Install binary
    install_binary "$tmp_dir" "$os"

    # Verify installation
    echo ""
    verify_installation "$os"

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
        --version)
            INSTALL_VERSION="$2"
            shift 2
            ;;
        --help|-h)
            echo "Centian Installation Script"
            echo ""
            echo "Usage: install.sh [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --install-dir DIR    Install to custom directory (default: ~/.local/bin)"
            echo "  --version VERSION    Install a specific version tag (e.g., v0.1.0)"
            echo "  --help, -h           Show this help message"
            echo ""
            echo "Environment Variables:"
            echo "  INSTALL_DIR          Custom installation directory"
            echo "  INSTALL_VERSION      Install a specific version tag"
            echo ""
            echo "Examples:"
            echo "  curl -fsSL https://raw.githubusercontent.com/T4cceptor/centian/main/scripts/install.sh | bash"
            echo "  curl -fsSL ... | INSTALL_DIR=~/.local/bin bash"
            echo "  curl -fsSL ... | INSTALL_VERSION=v0.1.0 bash"
            exit 0
            ;;
        *)
            error "Unknown option: $1. Use --help for usage information."
            ;;
    esac
done

# Run main installation
main
