# Shared low-level helpers used across multiple flows.

safe_json() {
  local response="$1" fallback="${2:-[]}"
  if jq -e . <<< "$response" &>/dev/null; then
    printf '%s\n' "$response"
  else
    printf '%s\n' "$fallback"
  fi
}

retry() {
  local max_attempts="${1:-3}"
  shift
  local attempt=1
  while (( attempt <= max_attempts )); do
    if "$@" &>/dev/null; then
      return 0
    fi
    ((attempt++))
    [[ $attempt -le $max_attempts ]] && sleep 1
  done
  return 1
}
