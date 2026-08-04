work_on_existing_issue() {
  if ! require_writes_allowed "work on GitLab issue"; then
    return 1
  fi

  echo ""
  echo -n "  ${DIM}Fetching issues and branches...${RESET}"

  # Fetch open issues
  if ! issues_json=$(fetch_all_issues "opened"); then
    printf "\r                                        \r"
    echo "  ${RED}${ICON_WARN}${RESET} Could not read GitLab issues completely."
    echo ""
    return 1
  fi
  issue_count=$(jq 'length' <<< "$issues_json")

  # Prune deleted remote-tracking refs before checking branch associations.
  remote_branches=""
  if require_writes_allowed "fetch Git branches" \
    && git fetch --prune origin --quiet 2>/dev/null; then
    remote_branches=$(git branch -r 2>/dev/null | grep -v 'HEAD' | sed 's|^ *origin/||' | sed 's|^ *||')
  else
    printf "\r                                        \r"
    echo "  ${YELLOW}${ICON_WARN}${RESET} Remote branches could not be refreshed; showing local branch associations only."
    echo -n "  ${DIM}Fetching issues and branches...${RESET}"
  fi
  local_branches=$(git branch 2>/dev/null | sed 's|^[* ] ||')
  all_branches=$(printf '%s\n%s' "$remote_branches" "$local_branches" | sort -u)

  printf "\r                                        \r"

  if [[ "$issue_count" -eq 0 ]]; then
    echo ""
    echo "  ${DIM}No open issues found.${RESET}"
    echo ""
    return 0
  fi

  # Build fzf list: check each issue for existing branch
  issue_list=""
  while IFS= read -r line; do
    iid=$(jq -r '.iid' <<< "$line")
    ititle=$(jq -r '.title' <<< "$line")

    # Check if any branch starts with <iid>-
    existing_branch=$(grep -E "^${iid}-" <<< "$all_branches" | head -1)

    if [[ -n "$existing_branch" ]]; then
      issue_list+="  #${iid}  ${ititle}  ${DIM}[${ICON_BRANCH} ${existing_branch}]${RESET}"$'\n'
    else
      issue_list+="  #${iid}  ${ititle}"$'\n'
    fi
  done < <(jq -c '.[]' <<< "$issues_json")

  # Remove trailing newline
  issue_list="${issue_list%$'\n'}"

  echo ""
  echo "  ${MAGENTA}${ICON_TITLE}${RESET} ${BOLD}Open Issues${RESET} ${DIM}(${issue_count} issues)${RESET}"
  echo ""

  selected_issue=$(fzf \
    --prompt="  Issue > " \
    --header="  ENTER=select  ESC=cancel" \
    --height=~40 \
    --reverse \
    --border=rounded \
    --border-label=" open issues " \
    --color="border:magenta,header:dim,prompt:magenta" \
    --ansi \
    <<< "$issue_list" \
    || echo "")

  if [[ -z "$selected_issue" ]]; then
    echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted."
    exit 0
  fi

  # Parse issue number from selection
  selected_iid=$(grep -oE '#[0-9]+' <<< "$selected_issue" | head -1 | tr -d '#')

  if [[ -z "$selected_iid" ]]; then
    echo "  ${RED}${ICON_WARN}${RESET} Could not parse issue number from selection."
    exit 1
  fi

  selected_title=$(jq -r --argjson iid "$selected_iid" '.[] | select(.iid == $iid) | .title' <<< "$issues_json")
  selected_issue_json=$(jq --argjson iid "$selected_iid" '.[] | select(.iid == $iid)' <<< "$issues_json")

  echo ""
  echo "  ${GREEN}${ICON_OK}${RESET} ${BOLD}#${selected_iid}${RESET} ${selected_title}"

  # Check if branch exists for context
  existing_branch=$(grep -E "^${selected_iid}-" <<< "$all_branches" | head -1)
  if [[ -n "$existing_branch" ]]; then
    echo "  ${DIM}${ICON_BRANCH} ${existing_branch}${RESET}"
  fi

  # ─── Action menu (loop) ───
  while true; do
    echo ""
    work_options="${ICON_BRANCH} Branch (checkout / create)
${ICON_DESC} Edit description
${ICON_LABEL} Edit labels
${ICON_USER} Edit assignee
${ICON_MILE} Edit milestone
${ICON_WARN} Close issue"

    work_action=$(fzf \
      --prompt="  Action > " \
      --header="  ENTER=select  ESC=done" \
      --height=~40 \
      --reverse \
      --border=rounded \
      --border-label=" #${selected_iid} " \
      --color="border:magenta,header:dim,prompt:magenta" \
      <<< "$work_options" \
      || echo "")

    if [[ -z "$work_action" ]]; then
      echo "  ${DIM}Done.${RESET}"
      break
    fi

    # ─── Branch ───
    if [[ "$work_action" == *"Branch"* ]]; then
      if [[ -n "$existing_branch" ]]; then
        echo ""
        echo "  ${YELLOW}${ICON_WARN}${RESET} Branch ${BOLD}${existing_branch}${RESET} already exists for this issue."
        echo ""
        echo -n "  ${BOLD}Check out existing branch?${RESET} ${DIM}(y/n)${RESET} "
        read -r do_checkout

        if [[ "$do_checkout" == "y" ]]; then
          if ! require_writes_allowed "check out Git branch"; then
            return 1
          fi
          checkout_err=$(git checkout "$existing_branch" 2>&1)
          checkout_rc=$?
          if [[ $checkout_rc -ne 0 ]]; then
            checkout_err=$(git checkout -b "$existing_branch" "origin/$existing_branch" 2>&1)
            checkout_rc=$?
          fi
          if [[ $checkout_rc -eq 0 ]]; then
            echo "  ${GREEN}${ICON_OK}${RESET} Switched to ${BOLD}${existing_branch}${RESET}"
          else
            echo "  ${RED}${ICON_WARN}${RESET} Failed to check out branch"
            echo "  ${DIM}${checkout_err}${RESET}"
          fi
        fi
        echo ""
      else
        offer_branch_creation "$selected_iid" "$selected_title"
      fi
      break

    # ─── Edit description ───
    elif [[ "$work_action" == *"description"* ]]; then
      echo ""
      echo -n "  ${DIM}Fetching current description...${RESET}"
      if ! current_issue=$(retry_read_json \
        "GitLab issue #${selected_iid}" \
        'type == "object" and ((.description // "") | type == "string")' \
        glab issue view "$selected_iid" --output json); then
        printf "\r                                        \r"
        echo "  ${RED}${ICON_WARN}${RESET} Could not read the current issue description."
        continue
      fi
      current_desc=$(jq -r '.description // ""' <<< "$current_issue")
      printf "\r                                        \r"

      tmpfile=$(mktemp "${TMPDIR:-/tmp}/gl-issue-XXXXXX")
      _tmpfiles+=("$tmpfile")
      cat > "$tmpfile" <<< "$current_desc"

      echo "  ${DIM}Opening editor... (save & quit to update, :cq to cancel)${RESET}"
      echo ""

      if ! open_editor "$tmpfile"; then
        echo "  ${YELLOW}${ICON_WARN}${RESET} Cancelled."
      else
        new_desc=$(cat "$tmpfile")
        if [[ "$new_desc" == "$current_desc" ]]; then
          echo "  ${DIM}No changes made.${RESET}"
        else
          echo -n "  ${DIM}Updating description...${RESET}"
          if require_writes_allowed "update GitLab issue description" \
            && glab issue update "$selected_iid" -d "$new_desc" &>/dev/null; then
            printf "\r                                \r"
            echo "  ${GREEN}${ICON_OK}${RESET} Description updated"
          else
            printf "\r                                \r"
            echo "  ${RED}${ICON_WARN}${RESET} Failed to update description"
          fi
        fi
      fi

    # ─── Edit labels ───
    elif [[ "$work_action" == *"labels"* ]]; then
      echo ""
      echo -n "  ${DIM}Fetching labels...${RESET}"
      current_labels=$(jq -r '.labels // [] | join(",")' <<< "$selected_issue_json")
      if ! labels_json=$(fetch_all_labels); then
        printf "\r                                \r"
        echo "  ${RED}${ICON_WARN}${RESET} Could not read GitLab labels."
        continue
      fi
      all_labels=$(jq -r 'sort_by(.name) | .[].name' <<< "$labels_json")
      printf "\r                                \r"

      if [[ -n "$current_labels" ]]; then
        echo "  ${DIM}Current:${RESET} ${current_labels}"
        echo ""
      fi

      selected_labels=$(fzf \
        --multi \
        --prompt="  Labels > " \
        --header="  TAB=multi-select  ENTER=confirm  ESC=keep current" \
        --height=~40 \
        --reverse \
        --border=rounded \
        --border-label=" labels " \
        --color="border:magenta,header:dim,prompt:magenta" \
        <<< "$all_labels" \
        || echo "")

      if [[ -n "$selected_labels" ]]; then
        new_labels=""
        while IFS= read -r lbl; do
          [[ -z "$lbl" ]] && continue
          if [[ -n "$new_labels" ]]; then
            new_labels="${new_labels},${lbl}"
          else
            new_labels="$lbl"
          fi
        done <<< "$selected_labels"

        if require_writes_allowed "update GitLab issue labels" \
          && glab issue update "$selected_iid" -l "$new_labels" &>/dev/null; then
          echo "  ${GREEN}${ICON_OK}${RESET} Labels updated to ${DIM}${new_labels}${RESET}"
        else
          echo "  ${RED}${ICON_WARN}${RESET} Failed to update labels"
        fi
      else
        echo "  ${DIM}No changes made.${RESET}"
      fi

    # ─── Edit assignee ───
    elif [[ "$work_action" == *"assignee"* ]]; then
      echo ""
      echo -n "  ${DIM}Fetching members...${RESET}"
      current_assignees=$(jq -r '.assignees // [] | map(.username) | join(", ")' <<< "$selected_issue_json")
      if ! members_json=$(fetch_all_members); then
        printf "\r                                \r"
        echo "  ${RED}${ICON_WARN}${RESET} Could not read GitLab project members."
        continue
      fi
      members=$(jq -r 'sort_by(.username) | .[] | "\(.username)  (\(.name))"' <<< "$members_json")
      printf "\r                                \r"

      if [[ -n "$current_assignees" ]]; then
        echo "  ${DIM}Current:${RESET} ${current_assignees}"
        echo ""
      fi

      if [[ -n "$members" ]]; then
        member_options="  Unassign
${members}"

        selected_member=$(fzf \
          --prompt="  Assignee > " \
          --header="  ENTER=select  ESC=keep current" \
          --height=~40 \
          --reverse \
          --border=rounded \
          --border-label=" assignee " \
          --color="border:magenta,header:dim,prompt:magenta" \
          <<< "$member_options" \
          || echo "")

        if [[ -z "$selected_member" ]]; then
          echo "  ${DIM}No changes made.${RESET}"
        elif [[ "$selected_member" == *"Unassign"* ]]; then
          if require_writes_allowed "remove GitLab issue assignee" \
            && glab issue update "$selected_iid" -a "" &>/dev/null; then
            echo "  ${GREEN}${ICON_OK}${RESET} Assignee removed"
          else
            echo "  ${RED}${ICON_WARN}${RESET} Failed to update assignee"
          fi
        else
          new_assignee=$(awk '{print $1}' <<< "$selected_member")
          if require_writes_allowed "update GitLab issue assignee" \
            && glab issue update "$selected_iid" -a "$new_assignee" &>/dev/null; then
            echo "  ${GREEN}${ICON_OK}${RESET} Assigned to ${DIM}${new_assignee}${RESET}"
          else
            echo "  ${RED}${ICON_WARN}${RESET} Failed to update assignee"
          fi
        fi
      else
        echo -n "  ${ICON_NEW} Username (leave empty to skip): "
        read -r new_assignee
        if [[ -n "$new_assignee" ]]; then
          if require_writes_allowed "update GitLab issue assignee" \
            && glab issue update "$selected_iid" -a "$new_assignee" &>/dev/null; then
            echo "  ${GREEN}${ICON_OK}${RESET} Assigned to ${DIM}${new_assignee}${RESET}"
          else
            echo "  ${RED}${ICON_WARN}${RESET} Failed to update assignee"
          fi
        fi
      fi

    # ─── Edit milestone ───
    elif [[ "$work_action" == *"milestone"* ]]; then
      echo ""
      echo -n "  ${DIM}Fetching milestones...${RESET}"
      current_ms=$(jq -r '.milestone.title // "none"' <<< "$selected_issue_json")
      if ! ms_json=$(fetch_all_milestones); then
        printf "\r                                \r"
        echo "  ${RED}${ICON_WARN}${RESET} Could not read GitLab milestones."
        continue
      fi
      milestones=$(jq -r '.[].title // empty' <<< "$ms_json")
      printf "\r                                \r"

      echo "  ${DIM}Current:${RESET} ${current_ms}"
      echo ""

      ms_options="  Remove milestone
${milestones}"

      selected_ms=$(fzf \
        --prompt="  Milestone > " \
        --header="  ENTER=select  ESC=keep current" \
        --height=~40 \
        --reverse \
        --border=rounded \
        --border-label=" milestone " \
        --color="border:magenta,header:dim,prompt:magenta" \
        <<< "$ms_options" \
        || echo "")

      if [[ -z "$selected_ms" ]]; then
        echo "  ${DIM}No changes made.${RESET}"
      elif [[ "$selected_ms" == *"Remove"* ]]; then
        if require_writes_allowed "remove GitLab issue milestone" \
          && glab issue update "$selected_iid" -m "" &>/dev/null; then
          echo "  ${GREEN}${ICON_OK}${RESET} Milestone removed"
        else
          echo "  ${RED}${ICON_WARN}${RESET} Failed to update milestone"
        fi
      else
        if require_writes_allowed "update GitLab issue milestone" \
          && glab issue update "$selected_iid" -m "$selected_ms" &>/dev/null; then
          echo "  ${GREEN}${ICON_OK}${RESET} Milestone set to ${DIM}${selected_ms}${RESET}"
        else
          echo "  ${RED}${ICON_WARN}${RESET} Failed to update milestone"
        fi
      fi

    # ─── Close issue ───
    elif [[ "$work_action" == *"Close"* ]]; then
      echo ""
      echo -n "  ${BOLD}Close issue #${selected_iid}?${RESET} ${DIM}(y/n)${RESET} "
      read -r confirm_close
      if [[ "$confirm_close" == "y" ]]; then
        if require_writes_allowed "close GitLab issue" \
          && glab issue close "$selected_iid" &>/dev/null; then
          echo "  ${GREEN}${ICON_OK}${RESET} Issue #${selected_iid} closed"
        else
          echo "  ${RED}${ICON_WARN}${RESET} Failed to close issue"
        fi
      else
        echo "  ${DIM}Cancelled.${RESET}"
      fi
      break
    fi
  done

  exit 0
}
