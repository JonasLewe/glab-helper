#!/bin/sh

set -eu

: "${HOME:?HOME must be set}"

repo_dir=$(CDPATH= cd "$(dirname "$0")" && pwd)
install_dir=${GLAB_HELPER_INSTALL_DIR:-"$HOME/.local/bin"}
staging=

cleanup() {
    if [ -n "$staging" ]; then
        rm -f "$staging"
    fi
}
trap cleanup 0 HUP INT TERM

printf '%s\n\n' '=== Installing glab-helper ==='

mkdir -p "$install_dir"
install_dir=$(CDPATH= cd "$install_dir" && pwd)
target="$install_dir/glab-helper"

case ":${PATH:-}:" in
    *":$install_dir:"*) ;;
    *)
        printf '%s\n' "$install_dir is not in PATH."
        printf '%s\n\n' 'Add export PATH="$HOME/.local/bin:$PATH" to your shell configuration.'
        ;;
esac

set --
for dependency in go glab fzf; do
    if ! command -v "$dependency" >/dev/null 2>&1; then
        set -- "$@" "$dependency"
    fi
done

if [ "$#" -gt 0 ]; then
    printf 'Installing missing dependencies:'
    printf ' %s' "$@"
    printf '\n'

    case "$(uname -s)" in
        Darwin)
            if ! command -v brew >/dev/null 2>&1; then
                printf '%s\n' 'Homebrew is required to install missing dependencies: https://brew.sh' >&2
                exit 1
            fi
            brew install "$@"
            ;;
        Linux)
            if ! command -v pacman >/dev/null 2>&1; then
                printf '%s\n' 'No supported package manager found. Install Go, glab, and fzf manually.' >&2
                exit 1
            fi
            if [ "$(id -u)" -eq 0 ]; then
                pacman -S --needed "$@"
            elif command -v sudo >/dev/null 2>&1; then
                sudo pacman -S --needed "$@"
            else
                printf '%s\n' 'sudo is required to install missing packages with pacman.' >&2
                exit 1
            fi
            ;;
        *)
            printf '%s\n' 'Unsupported operating system. Install Go, glab, and fzf manually.' >&2
            exit 1
            ;;
    esac
fi

for dependency in go glab fzf; do
    if ! command -v "$dependency" >/dev/null 2>&1; then
        printf 'Required command is still unavailable: %s\n' "$dependency" >&2
        exit 1
    fi
done

staging=$(mktemp "$install_dir/.glab-helper.XXXXXX")
(
    cd "$repo_dir"
    go build -trimpath -o "$staging" ./cmd/glab-helper
)
chmod 0755 "$staging"

if ! installed_version=$("$staging" --version); then
    printf '%s\n' 'The built glab-helper binary failed its version check.' >&2
    exit 1
fi

mv -f "$staging" "$target"
staging=
trap - 0 HUP INT TERM

printf '\nInstalled %s at %s\n' "$installed_version" "$target"
printf '%s\n' "Run glab-helper from a cloned GitLab repository."
