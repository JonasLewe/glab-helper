# Shared UI and editor helpers.

hr() {
  echo "  ${DIM}──────────────────────────────────────────────${RESET}"
}

box_line() {
  local text="$1"
  local visible_len=${#text}
  local pad=$((BOX_WIDTH - visible_len - 2))
  if [[ $pad -lt 0 ]]; then
    pad=0
  fi
  local spaces=""
  if [[ $pad -gt 0 ]]; then
    spaces=$(printf '%*s' "$pad" '')
  fi
  echo "  ${BOX}║${RESET}  ${text}${spaces}${BOX}║${RESET}"
}

offer_branch_creation() {
  local issue_nr="$1"
  local issue_title="$2"
  local slug branch_name branch_name_input want_branch
  local do_checkout checkout_err checkout_rc branch_err branch_rc
  local default_branch branch_list selected_base

  slug=$(slugify "$issue_title")
  branch_name="${issue_nr}-${slug}"

  if [[ ${#branch_name} -gt 60 ]]; then
    branch_name="${branch_name:0:60}"
    local truncated="${branch_name%-*}"
    branch_name="${truncated:-$branch_name}"
  fi

  hr
  echo ""
  echo "  ${MAGENTA}${ICON_BRANCH}${RESET} ${BOLD}Branch${RESET}"
  echo ""
  echo -n "  ${BOLD}Create a branch for this issue?${RESET} ${DIM}(y/n)${RESET} "
  read -r want_branch
  if [[ "$want_branch" != "y" ]]; then
    echo ""
    return
  fi

  echo ""
  echo -n "  ${BOLD}Branch name${RESET} ${DIM}[${branch_name}]${RESET}: "
  read -r branch_name_input
  branch_name="${branch_name_input:-$branch_name}"

  if git show-ref --verify --quiet "refs/heads/$branch_name" 2>/dev/null; then
    echo ""
    echo "  ${YELLOW}${ICON_WARN}${RESET} Branch ${BOLD}${branch_name}${RESET} already exists locally."
    echo ""
    echo -n "  ${BOLD}Check out existing branch?${RESET} ${DIM}(y/n)${RESET} "
    read -r do_checkout
    if [[ "$do_checkout" == "y" ]]; then
      if ! require_writes_allowed "check out Git branch"; then
        return 1
      fi
      checkout_err=$(git checkout "$branch_name" 2>&1)
      checkout_rc=$?
      if [[ $checkout_rc -eq 0 ]]; then
        echo "  ${GREEN}${ICON_OK}${RESET} Switched to ${BOLD}${branch_name}${RESET}"
      else
        echo "  ${RED}${ICON_WARN}${RESET} Failed to check out branch"
        echo "  ${DIM}${checkout_err}${RESET}"
      fi
    fi
    echo ""
    return
  fi

  if ! require_writes_allowed "fetch Git branches"; then
    return 1
  fi
  git fetch origin --quiet 2>/dev/null
  if ! default_branch=$(get_default_branch); then
    echo "  ${RED}${ICON_WARN}${RESET} Could not determine the default branch."
    return 1
  fi

  branch_list=$(git for-each-ref --sort=-committerdate --format='%(refname:short)' refs/remotes/origin/ \
    | sed 's|^origin/||' \
    | grep -v '^HEAD$')

  if [[ -z "$branch_list" ]]; then
    branch_list=$(git for-each-ref --sort=-committerdate --format='%(refname:short)' refs/heads/)
  fi

  if [[ -z "$branch_list" ]]; then
    echo ""
    echo "  ${YELLOW}${ICON_WARN}${RESET} No branches found. Create an initial commit first."
    echo ""
    return
  fi

  if [[ -n "$default_branch" ]]; then
    branch_list=$(echo "$branch_list" | grep -v "^${default_branch}$")
    branch_list="${default_branch} (default)
${branch_list}"
  fi

  echo ""
  selected_base=$(fzf \
    --prompt="  Base branch > " \
    --header="  ENTER=select  ESC=cancel" \
    --height=~40 \
    --reverse \
    --border=rounded \
    --border-label=" base branch " \
    --color="border:magenta,header:dim,prompt:magenta" \
    <<< "$branch_list" \
    || echo "")

  if [[ -z "$selected_base" ]]; then
    echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted."
    echo ""
    return
  fi

  selected_base="${selected_base% \(default\)}"

  local start_point="origin/$selected_base"
  if ! git show-ref --verify --quiet "refs/remotes/$start_point" 2>/dev/null; then
    if git show-ref --verify --quiet "refs/heads/$selected_base" 2>/dev/null; then
      start_point="$selected_base"
    else
      echo ""
      echo "  ${RED}${ICON_WARN}${RESET} Branch ${BOLD}${selected_base}${RESET} not found (neither remote nor local)."
      echo ""
      return
    fi
  fi

  if ! require_writes_allowed "create Git branch"; then
    return 1
  fi
  branch_err=$(git branch "$branch_name" "$start_point" 2>&1)
  branch_rc=$?
  if [[ $branch_rc -eq 0 ]]; then
    echo ""
    echo "  ${GREEN}${ICON_OK}${RESET} Branch ${BOLD}${branch_name}${RESET} created from ${DIM}${selected_base}${RESET}"

    echo ""
    echo -n "  ${BOLD}Check out branch now?${RESET} ${DIM}(y/n)${RESET} "
    read -r do_checkout

    if [[ "$do_checkout" == "y" ]]; then
      if ! require_writes_allowed "check out Git branch"; then
        return 1
      fi
      checkout_err=$(git checkout "$branch_name" 2>&1)
      checkout_rc=$?
      if [[ $checkout_rc -eq 0 ]]; then
        echo "  ${GREEN}${ICON_OK}${RESET} Switched to ${BOLD}${branch_name}${RESET}"
      else
        echo "  ${RED}${ICON_WARN}${RESET} Failed to check out branch"
        echo "  ${DIM}${checkout_err}${RESET}"
      fi
    fi
  else
    echo ""
    echo "  ${RED}${ICON_WARN}${RESET} Failed to create branch"
    echo "  ${DIM}${branch_err}${RESET}"
  fi

  echo ""
}

open_editor() {
  if command -v nvim &>/dev/null; then
    nvim --clean \
      -c "set noswapfile nobackup nowritebackup" \
      -c "syntax on | set filetype=markdown number cursorline" \
      "$1"
  elif command -v vim &>/dev/null; then
    vim -u NONE --noplugin \
      -c "set noswapfile nobackup nowritebackup" \
      -c "syntax on | set filetype=markdown number" \
      "$1"
  elif [[ -n "$VISUAL" ]]; then
    ${=VISUAL} "$1"
  elif [[ -n "$EDITOR" ]]; then
    ${=EDITOR} "$1"
  else
    vi "$1"
  fi
}
