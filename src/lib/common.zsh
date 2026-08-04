# Shared low-level helpers used across multiple flows.

sanitize_error() {
  local message="$1"
  if [[ -n "${JIRA_TOKEN:-}" ]]; then
    message="${message//$JIRA_TOKEN/<redacted>}"
  fi
  printf '%s\n' "$message"
}

summarize_error() {
  local message summary
  message="$(sanitize_error "$1")"
  summary="$(tail -n 1 <<< "$message")"
  if [[ ${#summary} -gt 240 ]]; then
    summary="${summary:0:237}..."
  fi
  printf '%s\n' "${summary:-request failed without an error message}"
}

api_error() {
  local resource="$1" operation="$2" cause="$3"
  printf '  %s%s%s %s %s failed: %s\n' \
    "${RED:-}" "${ICON_WARN:-!}" "${RESET:-}" \
    "$resource" "$operation" "$(summarize_error "$cause")" >&2
}

retry_safe() {
  local max_attempts="${1:-3}"
  shift
  local attempt=1 delay="${GLAB_HELPER_RETRY_BASE_DELAY:-1}"

  while (( attempt <= max_attempts )); do
    if "$@"; then
      return 0
    fi
    ((attempt++))
    if (( attempt <= max_attempts )); then
      sleep "$delay"
      delay=$((delay * 2))
      (( delay > 8 )) && delay=8
    fi
  done
  return 1
}

retry_read_json() {
  local resource="$1" schema="$2"
  shift 2
  local attempt=1 max_attempts=3 delay="${GLAB_HELPER_RETRY_BASE_DELAY:-1}"
  local output="" cause="" retry_after=""

  while (( attempt <= max_attempts )); do
    if output=$("$@" 2>&1); then
      if jq -e "$schema" <<< "$output" &>/dev/null; then
        printf '%s\n' "$output"
        return 0
      fi
      cause="response did not match the expected JSON schema"
    else
      cause="${output:-request exited with a non-zero status}"
    fi

    ((attempt++))
    if (( attempt <= max_attempts )); then
      retry_after=$(sed -nE \
        's/.*[Rr]etry-[Aa]fter:[[:space:]]*([0-9]+).*/\1/p; s/.*[Rr]etry after[[:space:]]+([0-9]+).*/\1/p' \
        <<< "$cause" | head -n 1)
      if [[ "$retry_after" == <-> ]]; then
        (( retry_after > 60 )) && retry_after=60
        sleep "$retry_after"
      else
        sleep "$delay"
      fi
      delay=$((delay * 2))
      (( delay > 8 )) && delay=8
    fi
  done

  api_error "$resource" "read" "$cause"
  return 1
}

retry_idempotent() {
  retry_safe 3 "$@"
}

require_writes_allowed() {
  local operation="${1:-mutation}"
  if [[ "${DRY_RUN_MODE:-false}" == "true" ]]; then
    api_error "$operation" "write" "blocked by --dry-run"
    return 1
  fi
  return 0
}

csv_to_lines() {
  local csv="$1"
  [[ -z "$csv" ]] && return 0
  tr ',' '\n' <<< "$csv" | sed '/^$/d'
}

csv_sets_equal() {
  local left_sorted right_sorted
  left_sorted="$(csv_to_lines "$1" | sort -u | paste -sd, -)"
  right_sorted="$(csv_to_lines "$2" | sort -u | paste -sd, -)"
  [[ "$left_sorted" == "$right_sorted" ]]
}

remove_status_labels_csv() {
  local cleaned
  cleaned="$(csv_to_lines "$1" | grep -v '^status::' | paste -sd, -)"
  printf '%s\n' "$cleaned"
}

csv_sets_equal_ignoring_status() {
  local left_csv right_csv left_sorted right_sorted
  left_csv="$(remove_status_labels_csv "$1")"
  right_csv="$(remove_status_labels_csv "$2")"
  left_sorted="$(csv_to_lines "$left_csv" | sort -u | paste -sd, -)"
  right_sorted="$(csv_to_lines "$right_csv" | sort -u | paste -sd, -)"
  [[ "$left_sorted" == "$right_sorted" ]]
}

build_story_sync_description() {
  local story_desc="$1"
  local subtasks_json="$2"
  local description st_count st st_key st_summary

  description=$(jira_to_markdown "$story_desc")
  st_count=$(jq 'length' <<< "$subtasks_json")
  if [[ "$st_count" -gt 0 ]]; then
    description="${description}"$'\n\n'"## Subtasks"
    while IFS= read -r st; do
      st_key=$(jq -r '.key' <<< "$st")
      st_summary=$(jq -r '.fields.summary' <<< "$st")
      description="${description}"$'\n'"- [ ] ${st_key}: ${st_summary}"
    done < <(jq -c '.[]' <<< "$subtasks_json")
  fi

  printf '%s\n' "$description"
}

trim_whitespace() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s\n' "$value"
}

is_suspicious_jira_epic_title() {
  local normalized
  normalized="$(trim_whitespace "$1")"
  [[ -z "$normalized" ]] && return 0
  [[ "$normalized" == *[[:alnum:]]* ]] && return 1
  return 0
}

normalize_status_token() {
  local normalized
  normalized="$(trim_whitespace "$1" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//; s/-+/-/g')"
  printf '%s\n' "$normalized"
}

jira_status_rank() {
  local status_name status_category
  status_name="$(normalize_status_token "$1")"
  status_category="$(normalize_status_token "$2")"

  case "$status_name" in
    in-review|review|code-review|peer-review|qa|testing)
      printf '2\n'
      return 0
      ;;
    in-progress|progress|doing|development|implementing|implementation)
      printf '1\n'
      return 0
      ;;
    to-do|todo|open|backlog|selected-for-development|ready)
      printf '0\n'
      return 0
      ;;
    done|closed|resolved)
      printf '3\n'
      return 0
      ;;
  esac

  case "$status_category" in
    done)
      printf '3\n'
      ;;
    new)
      printf '0\n'
      ;;
    *)
      printf '%s\n' '-1'
      ;;
  esac
}

gitlab_issue_status_rank() {
  local state="$1"
  local labels_csv="$2"
  local lbl

  [[ "$state" == "closed" ]] && { printf '3\n'; return 0; }

  while IFS= read -r lbl; do
    [[ "$lbl" == "status::review" ]] && { printf '2\n'; return 0; }
  done < <(csv_to_lines "$labels_csv")

  while IFS= read -r lbl; do
    [[ "$lbl" == "status::in-progress" ]] && { printf '1\n'; return 0; }
  done < <(csv_to_lines "$labels_csv")

  printf '0\n'
}

gitlab_status_label_for_rank() {
  case "$1" in
    1) printf 'status::in-progress\n' ;;
    2) printf 'status::review\n' ;;
    *) printf '\n' ;;
  esac
}

status_rank_display() {
  case "$1" in
    0) printf 'open\n' ;;
    1) printf 'in-progress\n' ;;
    2) printf 'review\n' ;;
    3) printf 'done\n' ;;
    *) printf 'unknown\n' ;;
  esac
}

normalize_status_labels_csv() {
  local labels_without_status target_status
  labels_without_status="$(remove_status_labels_csv "$1")"
  target_status="$2"

  if [[ -n "$target_status" ]]; then
    if [[ -n "$labels_without_status" ]]; then
      printf '%s,%s\n' "$labels_without_status" "$target_status"
    else
      printf '%s\n' "$target_status"
    fi
  else
    printf '%s\n' "$labels_without_status"
  fi
}

build_story_sync_labels_csv() {
  local labels_json="$1"
  local priority="$2"
  local labels_csv

  labels_csv=$(jq -r 'join(",")' <<< "$labels_json")
  if [[ -n "$priority" ]]; then
    if [[ -n "$labels_csv" ]]; then
      labels_csv="${labels_csv},prio::${priority}"
    else
      labels_csv="prio::${priority}"
    fi
  fi

  printf '%s\n' "$labels_csv"
}

preview_display_value() {
  if [[ -n "$1" ]]; then
    printf '%s\n' "$1"
  else
    printf 'none\n'
  fi
}

print_story_label_preview_details() {
  local old_csv="$1"
  local new_csv="$2"
  local old_prio="" new_prio="" lbl
  local -a old_non_prio=()
  local -a new_non_prio=()
  local -a added=()
  local -a removed=()

  while IFS= read -r lbl; do
    [[ -z "$lbl" ]] && continue
    if [[ "$lbl" == prio::* ]]; then
      old_prio="$lbl"
    elif [[ "$lbl" == status::* ]]; then
      continue
    else
      old_non_prio+=("$lbl")
    fi
  done < <(csv_to_lines "$old_csv")

  while IFS= read -r lbl; do
    [[ -z "$lbl" ]] && continue
    if [[ "$lbl" == prio::* ]]; then
      new_prio="$lbl"
    elif [[ "$lbl" == status::* ]]; then
      continue
    else
      new_non_prio+=("$lbl")
    fi
  done < <(csv_to_lines "$new_csv")

  if [[ "$old_prio" != "$new_prio" ]]; then
    echo "      priority: $(preview_display_value "$old_prio") -> $(preview_display_value "$new_prio")"
  fi

  for lbl in "${new_non_prio[@]}"; do
    if ! printf '%s\n' "${old_non_prio[@]}" | grep -qxF "$lbl"; then
      added+=("$lbl")
    fi
  done

  for lbl in "${old_non_prio[@]}"; do
    if ! printf '%s\n' "${new_non_prio[@]}" | grep -qxF "$lbl"; then
      removed+=("$lbl")
    fi
  done

  if [[ ${#added[@]} -gt 0 ]]; then
    echo "      add: ${(j:, :)added}"
  fi
  if [[ ${#removed[@]} -gt 0 ]]; then
    echo "      remove: ${(j:, :)removed}"
  fi
}
