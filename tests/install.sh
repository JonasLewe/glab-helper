#!/bin/sh

set -eu

root_dir=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
temporary_directory=$(mktemp -d)
trap 'rm -rf "$temporary_directory"' 0 HUP INT TERM

stub_directory="$temporary_directory/bin"
install_directory="$temporary_directory/install"
command_log="$temporary_directory/go-command"
mkdir -p "$stub_directory" "$install_directory"

for command_name in glab fzf; do
    printf '#!/bin/sh\nexit 0\n' >"$stub_directory/$command_name"
    chmod 0755 "$stub_directory/$command_name"
done

cat >"$stub_directory/go" <<'EOF'
#!/bin/sh
set -eu

printf '%s\n' "$PWD|$*" >"$INSTALL_TEST_COMMAND_LOG"
if [ "${INSTALL_TEST_BUILD_FAIL:-}" = true ]; then
    exit 42
fi

output=
while [ "$#" -gt 0 ]; do
    if [ "$1" = -o ]; then
        output=$2
        shift 2
        continue
    fi
    shift
done

[ -n "$output" ]
cat >"$output" <<'BINARY'
#!/bin/sh
printf '%s\n' 'glab-helper 0.2.0'
BINARY
chmod 0755 "$output"
EOF
chmod 0755 "$stub_directory/go"

legacy_binary="$temporary_directory/legacy-glab-helper"
printf '#!/bin/sh\nprintf "%%s\\n" legacy\n' >"$legacy_binary"
chmod 0755 "$legacy_binary"
ln -s "$legacy_binary" "$install_directory/glab-helper"

test_path="$stub_directory:/usr/bin:/bin"
output=$(HOME="$temporary_directory/home" \
    PATH="$test_path" \
    GLAB_HELPER_INSTALL_DIR="$install_directory" \
    INSTALL_TEST_COMMAND_LOG="$command_log" \
    "$root_dir/install.sh")

case "$output" in
    *"Installed glab-helper 0.2.0 at $install_directory/glab-helper"*) ;;
    *)
        printf 'unexpected installer output:\n%s\n' "$output" >&2
        exit 1
        ;;
esac

if [ "$("$install_directory/glab-helper" --version)" != 'glab-helper 0.2.0' ]; then
    printf '%s\n' 'installed binary did not pass the version check' >&2
    exit 1
fi
if [ -L "$install_directory/glab-helper" ]; then
    printf '%s\n' 'installer did not replace the legacy symlink with the Go binary' >&2
    exit 1
fi

expected_command="$root_dir|build -trimpath -o"
if ! grep -F "$expected_command" "$command_log" >/dev/null; then
    printf 'unexpected Go build command: %s\n' "$(cat "$command_log")" >&2
    exit 1
fi

printf '#!/bin/sh\nprintf "%%s\\n" preserved\n' >"$install_directory/glab-helper"
chmod 0755 "$install_directory/glab-helper"

if HOME="$temporary_directory/home" \
    PATH="$test_path" \
    GLAB_HELPER_INSTALL_DIR="$install_directory" \
    INSTALL_TEST_COMMAND_LOG="$command_log" \
    INSTALL_TEST_BUILD_FAIL=true \
    "$root_dir/install.sh" >/dev/null 2>&1; then
    printf '%s\n' 'installer unexpectedly succeeded after a failed build' >&2
    exit 1
fi

if [ "$("$install_directory/glab-helper")" != preserved ]; then
    printf '%s\n' 'failed build replaced the existing installation' >&2
    exit 1
fi

if find "$install_directory" -maxdepth 1 -name '.glab-helper.*' | grep . >/dev/null; then
    printf '%s\n' 'installer left a staging file behind' >&2
    exit 1
fi

printf '%s\n' 'installer checks passed'
