#!/usr/bin/env bash
# ==============================================================================
# glab-helper installer — macOS & Arch Linux
# ==============================================================================
#
# Usage:
#   git clone <repo> ~/glab-helper
#   cd ~/glab-helper
#   ./install.sh
#
# What it does:
#   1. Installs dependencies (glab, fzf, jq) via brew/pacman
#   2. Symlinks src/glab-helper → ~/.local/bin/glab-helper

set -e

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OS="$(uname)"
BIN_DIR="$HOME/.local/bin"

echo "=== Installing glab-helper ==="
echo

# ─── Ensure ~/.local/bin exists and is on PATH ───
mkdir -p "$BIN_DIR"

if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
    echo "$BIN_DIR is not in your PATH."
    echo "   Add this to your shell config:"
    echo "     export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo
fi

# ─── Install dependencies ───
deps=(glab fzf jq)
missing=()

for cmd in "${deps[@]}"; do
    if ! command -v "$cmd" &>/dev/null; then
        missing+=("$cmd")
    fi
done

if [[ ${#missing[@]} -eq 0 ]]; then
    echo "All dependencies installed (${deps[*]})"
else
    echo "Installing missing dependencies: ${missing[*]}"

    if [[ "$OS" == "Darwin" ]]; then
        if ! command -v brew &>/dev/null; then
            echo "Homebrew not found. Install it first: https://brew.sh"
            exit 1
        fi
        brew install "${missing[@]}"
    elif [[ "$OS" == "Linux" ]]; then
        if command -v pacman &>/dev/null; then
            sudo pacman -S --noconfirm "${missing[@]}"
        else
            echo "No supported package manager found (brew/pacman)."
            echo "   Please install manually: ${missing[*]}"
            echo "   See: https://gitlab.com/gitlab-org/cli#installation"
            exit 1
        fi
    fi
fi

echo

# ─── Symlink glab-helper to ~/.local/bin ───
src="$REPO_DIR/src/glab-helper"
dst="$BIN_DIR/glab-helper"

if [[ ! -f "$src" ]]; then
    echo "Source script not found: $src"
    exit 1
fi

chmod +x "$src"

if [[ -e "$dst" ]] || [[ -L "$dst" ]]; then
    if [[ "$(readlink "$dst" 2>/dev/null)" == "$src" ]]; then
        echo "glab-helper already linked"
    else
        read -p "$dst already exists. Overwrite? (y/n) " -r || REPLY="n"
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            rm -f "$dst"
            ln -s "$src" "$dst"
            echo "glab-helper → $src"
        else
            echo "Skipping"
        fi
    fi
else
    ln -s "$src" "$dst"
    echo "glab-helper → $src"
fi

echo
echo "Installation complete!"
echo "   Run 'glab-helper' from any GitLab repo to get started."
echo
