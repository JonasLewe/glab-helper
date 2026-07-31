wrap_text_for_issue_summary() {
  fold -s -w "$2" <<< "$1"
}

create_issue() {
  # Sub-menu: From Jira or Manual?
  create_mode="manual"
  if $JIRA_AVAILABLE; then
    create_options="${ICON_TITLE} From Jira
${ICON_NEW} Manual"

    create_action=$(fzf \
      --prompt="  Create issue > " \
      --header="  ENTER=select  ESC=cancel" \
      --height=~40 \
      --reverse \
      --border=rounded \
      --border-label=" create issue " \
      --color="border:cyan,header:dim,prompt:cyan" \
      <<< "$create_options" \
      || echo "")

    if [[ -z "$create_action" ]]; then
      echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted."
      exit 0
    fi

    if [[ "$create_action" == *"From Jira"* ]]; then
      create_mode="jira"
    fi
  fi

  # ─────────────────────────────────────────
  # FROM JIRA
  # ─────────────────────────────────────────
  if [[ "$create_mode" == "jira" ]]; then

    echo ""
    echo -n "  ${DIM}Fetching Jira stories...${RESET}"

    if ! fetch_jira_stories_paginated; then
      printf "\r                                      \r"
      echo ""
      echo "  ${RED}${ICON_WARN}${RESET} Could not connect to Jira."
      echo "  ${DIM}Check VPN/Wireguard connection.${RESET}"
      echo ""
      echo -n "  ${BOLD}Create a local issue instead?${RESET} ${DIM}(y/n)${RESET} "
      read -r fallback_choice
      if [[ "$fallback_choice" == "y" ]]; then
        create_mode="manual"
      else
        exit 0
      fi
    fi
  fi

  if [[ "$create_mode" == "jira" ]]; then

    printf "\r                                      \r"

    # Fetch existing GitLab issues (paginated) to find already-synced stories
    echo -n "  ${DIM}Checking synced issues...${RESET}"
    if ! gl_issues_json=$(fetch_all_issues "opened"); then
      printf "\r                                      \r"
      echo "  ${RED}${ICON_WARN}${RESET} Issue creation aborted because GitLab issues could not be read completely."
      echo ""
      exit 1
    fi
    synced_keys=$(jq -r '.[] | .title' <<< "$gl_issues_json" | grep -oE '\[[A-Z]+-[0-9]+\]' | tr -d '[]')
    printf "\r                                      \r"

    # Filter out already-synced stories
    unsynced_json=$(jq --arg synced "$synced_keys" '
      ($synced | split("\n") | map(select(length > 0))) as $keys |
      map(select(.key as $k | ($keys | index($k)) | not))
    ' <<< "$JIRA_STORIES_JSON")
    unsynced_count=$(jq 'length' <<< "$unsynced_json")

    if [[ "$unsynced_count" -eq 0 ]]; then
      echo ""
      echo "  ${DIM}All Jira stories are already synced to GitLab.${RESET}"
      echo ""
      exit 0
    fi

    # Build fzf list
    story_list=""
    while IFS= read -r line; do
      skey=$(jq -r '.key' <<< "$line")
      stitle=$(jq -r '.fields.summary' <<< "$line")
      sstatus=$(jq -r '.fields.status.name' <<< "$line")
      sprio=$(jq -r '.fields.priority.name' <<< "$line")
      story_list+="  ${skey}  ${stitle}  ${DIM}(${sstatus} | ${sprio})${RESET}"$'\n'
    done < <(jq -c '.[]' <<< "$unsynced_json")
    story_list="${story_list%$'\n'}"

    echo ""
    echo "  ${MAGENTA}${ICON_TITLE}${RESET} ${BOLD}Unsynced Jira Stories${RESET} ${DIM}(${unsynced_count} stories)${RESET}"
    echo ""

    selected_story=$(fzf \
      --prompt="  Story > " \
      --header="  ENTER=select  ESC=cancel" \
      --height=~40 \
      --reverse \
      --border=rounded \
      --border-label=" jira stories " \
      --color="border:magenta,header:dim,prompt:magenta" \
      --ansi \
      <<< "$story_list" \
      || echo "")

    if [[ -z "$selected_story" ]]; then
      echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted."
      exit 0
    fi

    # Parse selected Jira key
    selected_jira_key=$(grep -oE '[A-Z]+-[0-9]+' <<< "$selected_story" | head -1)
    selected_story_json=$(jq --arg key "$selected_jira_key" '.[] | select(.key == $key)' <<< "$unsynced_json")

    # Extract fields
    jira_summary=$(jq -r '.fields.summary' <<< "$selected_story_json")
    jira_description=$(jq -r '.fields.description // ""' <<< "$selected_story_json")
    jira_priority=$(jq -r '.fields.priority.name // ""' <<< "$selected_story_json")
    jira_labels_json=$(jq -r '.fields.labels // []' <<< "$selected_story_json")
    jira_epic_key=$(jq -r '.fields.customfield_10000 // ""' <<< "$selected_story_json")
    jira_subtasks_json=$(jq '.fields.subtasks // []' <<< "$selected_story_json")

    # Build title
    title="[${selected_jira_key}] ${jira_summary}"

    description=$(jira_to_markdown "$jira_description")

    # Append subtasks as checkboxes
    subtask_count=$(jq 'length' <<< "$jira_subtasks_json")
    if [[ "$subtask_count" -gt 0 ]]; then
      description="${description}

## Subtasks"
      while IFS= read -r st; do
        st_key=$(jq -r '.key' <<< "$st")
        st_summary=$(jq -r '.fields.summary' <<< "$st")
        description="${description}
- [ ] ${st_key}: ${st_summary}"
      done < <(jq -c '.[]' <<< "$jira_subtasks_json")
    fi

    # ─── Open editor for description review ───
    echo ""
    hr
    echo "  ${MAGENTA}${ICON_DESC}${RESET} ${BOLD}Description${RESET} ${DIM}(from Jira, review and edit)${RESET}"
    echo ""

    tmpfile=$(mktemp "${TMPDIR:-/tmp}/gl-issue-XXXXXX")
    _tmpfiles+=("$tmpfile")
    cat > "$tmpfile" <<< "$description"

    echo "  ${DIM}Opening editor... (save & quit to continue, :cq to abort)${RESET}"
    echo ""

    open_editor "$tmpfile"
    editor_status=$?

    if [[ $editor_status -ne 0 ]]; then
      echo ""
      echo "  ${YELLOW}${ICON_WARN}${RESET} Editor exited with error. Aborting."
      exit 1
    fi

    if [[ ! -f "$tmpfile" ]]; then
      echo ""
      echo "  ${RED}${ICON_WARN}${RESET} Description file was lost. Aborting."
      exit 1
    fi

    description=$(cat "$tmpfile")
    echo "  ${GREEN}${ICON_OK}${RESET} ${DIM}Description updated${RESET}"

    # ─── Labels (Jira labels pre-selected + option to add more) ───
    echo ""
    hr
    echo "  ${MAGENTA}${ICON_LABEL}${RESET} ${BOLD}Labels${RESET}"
    echo ""

    # Build labels CSV from Jira
    labels_csv=$(jq -r 'join(",")' <<< "$jira_labels_json")
    if [[ -n "$jira_priority" ]]; then
      if [[ -n "$labels_csv" ]]; then
        labels_csv="${labels_csv},prio::${jira_priority}"
      else
        labels_csv="prio::${jira_priority}"
      fi
    fi

    echo "  ${DIM}From Jira:${RESET} ${labels_csv}"
    echo ""

    # Plan missing Jira labels. They are created only after final confirmation.
    echo -n "  ${DIM}Checking labels...${RESET}"
    if ! labels_json=$(fetch_all_labels); then
      printf "\r                                      \r"
      echo "  ${RED}${ICON_WARN}${RESET} Issue creation aborted because GitLab labels could not be read completely."
      echo ""
      exit 1
    fi
    existing_labels=$(jq -r '.[].name' <<< "$labels_json")
    missing_labels=()
    IFS=',' read -rA label_array <<< "$labels_csv"
    for lbl in "${label_array[@]}"; do
      [[ -z "$lbl" ]] && continue
      if ! grep -qxF "$lbl" <<< "$existing_labels"; then
        missing_labels+=("$lbl")
      fi
    done
    printf "\r                                      \r"

    # Offer to add more labels
    all_labels=$(jq -r 'sort_by(.name) | .[].name' <<< "$labels_json" 2>/dev/null || echo "")
    # Remove already-selected labels from the list
    available_labels=""
    while IFS= read -r lbl; do
      [[ -z "$lbl" ]] && continue
      if ! grep -qxF "$lbl" <<< "$(tr ',' '\n' <<< "$labels_csv")"; then
        available_labels+="${lbl}"$'\n'
      fi
    done <<< "$all_labels"
    available_labels="${available_labels%$'\n'}"

    if [[ -n "$available_labels" ]]; then
      echo -n "  ${BOLD}Add more labels?${RESET} ${DIM}(y/n)${RESET} "
      read -r want_more_labels
      if [[ "$want_more_labels" == "y" ]]; then
        echo ""
        extra_labels=$(fzf \
          --multi \
          --prompt="  Labels > " \
          --header="  TAB=multi-select  ENTER=confirm  ESC=none" \
          --height=~40 \
          --reverse \
          --border=rounded \
          --border-label=" additional labels " \
          --color="border:magenta,header:dim,prompt:magenta" \
          <<< "$available_labels" \
          || echo "")

        if [[ -n "$extra_labels" ]]; then
          while IFS= read -r lbl; do
            [[ -z "$lbl" ]] && continue
            labels_csv="${labels_csv},${lbl}"
          done <<< "$extra_labels"
        fi
      fi
    fi

    echo "  ${GREEN}${ICON_OK}${RESET} ${DIM}${labels_csv}${RESET}"

    # ─── Assignee ───
    echo ""
    hr
    echo "  ${MAGENTA}${ICON_USER}${RESET} ${BOLD}Assignee${RESET} ${DIM}(ENTER=confirm, ESC=abort)${RESET}"
    echo ""

    assignee=""
    if ! members_json=$(fetch_all_members); then
      echo "  ${RED}${ICON_WARN}${RESET} Issue creation aborted because GitLab members could not be read completely."
      echo ""
      exit 1
    fi
    members=$(jq -r 'sort_by(.username) | .[] | "\(.username)  (\(.name))"' <<< "$members_json")

    if [[ -n "$members" ]]; then
      member_options="  Skip (no assignee)
${members}"

      selected_member=$(fzf \
        --prompt="  Assignee > " \
        --header="  ENTER=select  ESC=abort" \
        --height=~40 \
        --reverse \
        --border=rounded \
        --border-label=" members " \
        --color="border:magenta,header:dim,prompt:magenta" \
        <<< "$member_options" \
        || echo "")

      if [[ -z "$selected_member" ]]; then
        echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted."
        exit 0
      fi

      if grep -q "Skip (no assignee)" <<< "$selected_member"; then
        assignee=""
      elif [[ -n "$selected_member" ]]; then
        assignee=$(awk '{print $1}' <<< "$selected_member")
      fi
    else
      echo -n "  ${ICON_NEW} Username (leave empty to skip): "
      read -r assignee
    fi

    if [[ -n "$assignee" ]]; then
      echo "  ${GREEN}${ICON_OK}${RESET} ${DIM}${assignee}${RESET}"
    else
      echo "  ${DIM}  none${RESET}"
    fi

    # ─── Milestone from Epic ───
    echo ""
    hr
    echo "  ${MAGENTA}${ICON_MILE}${RESET} ${BOLD}Milestone${RESET}"
    echo ""

    milestone=""
    milestone_action="none"
    existing_ms_id=""
    ms_desc_full=""
    if [[ -n "$jira_epic_key" && "$jira_epic_key" != "null" ]]; then
      echo -n "  ${DIM}Fetching epic from Jira...${RESET}"
      if ! epic_response=$(retry_read_json \
        "Jira epic ${jira_epic_key}" \
        'type == "object" and (.fields | type == "object") and ((.fields.summary // "") | type == "string") and ((.fields.description // "") | type == "string")' \
        curl -fsS --retry 2 --retry-all-errors --retry-delay 0 --retry-max-time 30 \
        -H "Authorization: Bearer ${JIRA_TOKEN}" \
        "${JIRA_URL}/rest/api/2/issue/${jira_epic_key}?fields=summary,description"); then
        printf "\r                                      \r"
        echo "  ${YELLOW}${ICON_WARN}${RESET} Could not fetch epic ${DIM}${jira_epic_key}${RESET}"
      else
        epic_title=$(jq -r '.fields.summary // ""' <<< "$epic_response")
        epic_desc=$(jira_to_markdown "$(jq -r '.fields.description // ""' <<< "$epic_response")")
        ms_desc_full="${epic_desc:+$epic_desc

}<!-- jira:${jira_epic_key} -->"
        printf "\r                                      \r"

        if [[ -n "$epic_title" ]]; then
          if ! ms_json=$(fetch_all_milestones); then
            echo "  ${RED}${ICON_WARN}${RESET} Issue creation aborted because GitLab milestones could not be read completely."
            echo ""
            exit 1
          fi
          # Match by Jira key in description first, then fall back to title
          existing_ms_id=$(jq -r --arg k "$jira_epic_key" '[.[] | select(.description // "" | contains("<!-- jira:" + $k + " -->"))][0] | .id // empty' <<< "$ms_json" 2>/dev/null)
          if [[ -z "$existing_ms_id" ]]; then
            existing_ms_id=$(jq -r --arg t "$epic_title" '[.[] | select(.title == $t)][0] | .id // empty' <<< "$ms_json" 2>/dev/null)
          fi

          if [[ -z "$existing_ms_id" ]]; then
            milestone_action="create"
            echo "  ${DIM}Milestone ${BOLD}${epic_title}${RESET} will be created after confirmation.${RESET}"
          else
            old_ms_title=$(jq -r --argjson id "$existing_ms_id" '.[] | select(.id == $id) | .title // ""' <<< "$ms_json")
            old_ms_desc=$(jq -r --argjson id "$existing_ms_id" '.[] | select(.id == $id) | .description // ""' <<< "$ms_json")
            if [[ "$(trim_whitespace "$old_ms_title")" != "$(trim_whitespace "$epic_title")" || "$old_ms_desc" != "$ms_desc_full" ]]; then
              milestone_action="update"
              echo "  ${DIM}Milestone ${BOLD}${epic_title}${RESET} will be updated after confirmation.${RESET}"
            else
              echo "  ${GREEN}${ICON_OK}${RESET} Milestone ${BOLD}${epic_title}${RESET} ${DIM}(up-to-date)${RESET}"
            fi
          fi
          milestone="$epic_title"
        fi
      fi
    else
      echo "  ${DIM}  no epic linked${RESET}"
    fi

    # ─── Summary & Confirm ───
    echo ""
    hr
    echo ""
    echo "  ${BOLD}Summary${RESET}"
    echo ""
    echo "  ${ICON_TITLE} Title:     ${BOLD}${title}${RESET}"
    echo "  ${ICON_LABEL} Labels:    ${DIM}${labels_csv}${RESET}"
    echo "  ${ICON_USER} Assignee:  ${DIM}${assignee:-none}${RESET}"
    echo "  ${ICON_MILE} Milestone: ${DIM}${milestone:-none}${RESET}"
    if [[ "$subtask_count" -gt 0 ]]; then
      echo "  ${ICON_DESC} Subtasks:  ${DIM}${subtask_count}${RESET}"
    fi
    echo ""

    echo -n "  ${BOLD}Create this issue?${RESET} ${DIM}(y/n)${RESET} "
    read -r confirm
    if [[ "$confirm" != "y" ]]; then
      echo ""
      echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted."
      exit 0
    fi

    if ! require_writes_allowed "create issue from Jira"; then
      exit 1
    fi

    # Apply dependencies only after confirmation: milestone, labels, then issue.
    if [[ "$milestone_action" == "create" ]]; then
      ms_cmd=(glab api "projects/$project_id/milestones" -X POST -f "title=$epic_title" -f "description=$ms_desc_full")
      if ! require_writes_allowed "create GitLab milestone" || ! "${ms_cmd[@]}" &>/dev/null; then
        echo "  ${RED}${ICON_WARN}${RESET} Required milestone could not be created; issue was not created."
        exit 1
      fi
    elif [[ "$milestone_action" == "update" ]]; then
      ms_cmd=(glab api "projects/$project_id/milestones/$existing_ms_id" -X PUT -f "title=$epic_title" -f "description=$ms_desc_full")
      if ! require_writes_allowed "update GitLab milestone" || ! retry_idempotent "${ms_cmd[@]}" &>/dev/null; then
        echo "  ${RED}${ICON_WARN}${RESET} Required milestone could not be updated; issue was not created."
        exit 1
      fi
    fi

    if [[ "$milestone_action" != "none" ]]; then
      if ! ms_json=$(fetch_all_milestones) \
        || ! jq -e --arg key "$jira_epic_key" --arg title "$epic_title" \
          'any(.[]; .title == $title and ((.description // "") | contains("<!-- jira:" + $key + " -->")))' \
          <<< "$ms_json" &>/dev/null; then
        echo "  ${RED}${ICON_WARN}${RESET} Required milestone could not be verified; issue was not created."
        exit 1
      fi
    fi

    for lbl in "${missing_labels[@]}"; do
      [[ -z "$lbl" ]] && continue
      if ! require_writes_allowed "create GitLab label" || ! glab label create -n "$lbl" &>/dev/null; then
        echo "  ${RED}${ICON_WARN}${RESET} Required label '${lbl}' could not be created; issue was not created."
        exit 1
      fi
    done
    if [[ ${#missing_labels[@]} -gt 0 ]]; then
      if ! labels_json=$(fetch_all_labels); then
        echo "  ${RED}${ICON_WARN}${RESET} Required labels could not be verified; issue was not created."
        exit 1
      fi
      existing_labels=$(jq -r '.[].name' <<< "$labels_json")
      for lbl in "${label_array[@]}"; do
        if ! grep -qxF "$lbl" <<< "$existing_labels"; then
          echo "  ${RED}${ICON_WARN}${RESET} Required label '${lbl}' is unavailable; issue was not created."
          exit 1
        fi
      done
    fi

    # ─── Create the issue ───
    cmd=(glab issue create -t "$title" -d "$description")
    if [[ -n "$labels_csv" ]]; then
      cmd+=(-l "$labels_csv")
    fi
    if [[ -n "$assignee" ]]; then
      cmd+=(-a "$assignee")
    fi
    if [[ -n "$milestone" ]]; then
      cmd+=(-m "$milestone")
    fi

    echo ""
    echo -n "  ${DIM}Creating issue...${RESET}"
    if require_writes_allowed "create GitLab issue"; then
      output=$("${cmd[@]}" 2>&1)
      create_status=$?
    else
      output="write blocked"
      create_status=1
    fi
    printf "\r                       \r"

    if [[ $create_status -eq 0 ]]; then
      echo ""
      echo "  ${GREEN}${ICON_OK} Issue created successfully${RESET}"
      echo "  ${DIM}${output}${RESET}"
      echo ""

      # Offer branch creation
      issue_number=$(grep -oE '/issues/[0-9]+' <<< "$output" | grep -oE '[0-9]+' | tail -1)
      if [[ -n "$issue_number" ]]; then
        offer_branch_creation "$issue_number" "$jira_summary"
      fi
    else
      echo ""
      echo "  ${RED}${ICON_WARN} Failed to create issue${RESET}"
      echo "  ${DIM}${output}${RESET}"
      echo ""
      exit 1
    fi

    exit 0
  fi

  # ─────────────────────────────────────────
  # MANUAL
  # ─────────────────────────────────────────

  echo ""
  echo -n "  ${DIM}Fetching project data...${RESET}"

  # ─── Fetch labels ───
  if ! labels_json=$(fetch_all_labels); then
    echo "  ${RED}${ICON_WARN}${RESET} Manual issue creation aborted because GitLab labels could not be read completely."
    exit 1
  fi
  all_labels=$(jq -r 'sort_by(.name) | .[].name' <<< "$labels_json")
  label_count=$(awk 'NF{c++} END{print c+0}' <<< "$all_labels")

  # ─── Fetch members ───
  if ! members_json=$(fetch_all_members); then
    echo "  ${RED}${ICON_WARN}${RESET} Manual issue creation aborted because GitLab members could not be read completely."
    exit 1
  fi
  members=$(jq -r 'sort_by(.username) | .[] | "\(.username)  (\(.name))"' <<< "$members_json")
  member_count=$(awk 'NF{c++} END{print c+0}' <<< "$members")

  # ─── Fetch milestones ───
  if ! ms_json=$(fetch_all_milestones); then
    echo "  ${RED}${ICON_WARN}${RESET} Manual issue creation aborted because GitLab milestones could not be read completely."
    exit 1
  fi
  milestones=$(jq -r '.[].title // empty' <<< "$ms_json")
  ms_count=$(awk 'NF{c++} END{print c+0}' <<< "$milestones")

  printf "\r                                    \r"

  # Show stats
  echo "  ${DIM}${label_count} labels  ·  ${member_count} members  ·  ${ms_count} milestones${RESET}"
  echo ""

  # ═══════════════════════════════════════════
  # TITLE
  # ═══════════════════════════════════════════
  echo -n "  ${YELLOW}${ICON_TITLE}${RESET} ${BOLD}Title:${RESET} "
  read -r title
  if [[ -z "$title" ]]; then
    echo "  ${RED}${ICON_WARN} Aborted: title is required.${RESET}"
    exit 1
  fi

  # ═══════════════════════════════════════════
  # LABELS
  # ═══════════════════════════════════════════
  echo ""
  hr
  echo "  ${MAGENTA}${ICON_LABEL}${RESET} ${BOLD}Labels${RESET} ${DIM}(TAB=select, ENTER=confirm, ESC=abort)${RESET}"
  echo ""

  # Suggest example labels if none exist
  if [[ -z "$all_labels" ]]; then
    echo "  ${DIM}No labels found. Here are some suggestions:${RESET}"
    echo ""
    echo "    ${DIM}prio::1-blocker    prio::2-important   prio::3-roadmap${RESET}"
    echo "    ${DIM}type::feature      type::bug           type::research${RESET}"
    echo "    ${DIM}type::docs         epic::backend       epic::frontend${RESET}"
    echo ""
  fi

  labels_csv=""
  planned_label_names=()
  planned_label_colors=()
  label_loop=true
  while $label_loop; do
    label_options="  Skip (no labels)
${ICON_NEW} Create new label..."
    if [[ -n "$all_labels" ]]; then
      label_options="  Skip (no labels)
${ICON_NEW} Create new label...
${all_labels}"
    fi

    selected_labels=$(fzf \
      --multi \
      --prompt="  Labels > " \
      --header="  TAB=multi-select  ENTER=confirm  ESC=abort" \
      --height=~40 \
      --reverse \
      --border=rounded \
      --border-label=" labels " \
      --color="border:magenta,header:dim,prompt:magenta" \
      <<< "$label_options" \
      || echo "")

    if [[ -z "$selected_labels" ]]; then
      echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted."
      exit 0
    fi

    if grep -q "Create new label" <<< "$selected_labels"; then
      echo ""
      echo -n "  ${ICON_NEW} Label name (e.g. ${DIM}scope::frontend${RESET}): "
      read -r new_label_name
      if [[ -n "$new_label_name" ]]; then
        echo -n "  ${ICON_NEW} Color hex (e.g. ${DIM}#E44D2E${RESET}, empty=default): "
        read -r new_label_color
        planned_label_names+=("$new_label_name")
        planned_label_colors+=("$new_label_color")
        echo "  ${DIM}Label ${BOLD}${new_label_name}${RESET} will be created after confirmation.${RESET}"
        all_labels=$(printf "%s\n%s" "$all_labels" "$new_label_name" | sort | grep -v '^$')
        echo ""
        echo "  ${DIM}Reopening label selector...${RESET}"
        echo ""
      fi
    else
      # Remove Skip and Create entries from selection
      selected_labels=$(grep -v -e "Create new label" -e "Skip (no labels)" <<< "$selected_labels" || echo "")
      if [[ -n "$selected_labels" ]]; then
        labels_csv=""
        while IFS= read -r lbl; do
          [[ -z "$lbl" ]] && continue
          if [[ -n "$labels_csv" ]]; then
            labels_csv="${labels_csv},${lbl}"
          else
            labels_csv="$lbl"
          fi
        done <<< "$selected_labels"
      fi
      label_loop=false
    fi
  done

  if [[ -n "$labels_csv" ]]; then
    echo "  ${GREEN}${ICON_OK}${RESET} ${DIM}${labels_csv}${RESET}"
  else
    echo "  ${DIM}  none${RESET}"
  fi

  # ═══════════════════════════════════════════
  # ASSIGNEE
  # ═══════════════════════════════════════════
  echo ""
  hr
  echo "  ${MAGENTA}${ICON_USER}${RESET} ${BOLD}Assignee${RESET} ${DIM}(ENTER=confirm, ESC=abort)${RESET}"
  echo ""

  assignee=""
  if [[ -n "$members" ]]; then
    member_options="  Skip (no assignee)
${ICON_NEW} Enter manually...
${members}"

    selected_member=$(fzf \
      --prompt="  Assignee > " \
      --header="  ENTER=select  ESC=abort" \
      --height=~40 \
      --reverse \
      --border=rounded \
      --border-label=" members " \
      --color="border:magenta,header:dim,prompt:magenta" \
      <<< "$member_options" \
      || echo "")

    if [[ -z "$selected_member" ]]; then
      echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted."
      exit 0
    fi

    if grep -q "Enter manually" <<< "$selected_member"; then
      echo -n "  ${ICON_NEW} Username: "
      read -r assignee
    elif grep -q "Skip (no assignee)" <<< "$selected_member"; then
      assignee=""
    elif [[ -n "$selected_member" ]]; then
      assignee=$(awk '{print $1}' <<< "$selected_member")
    fi
  else
    echo -n "  ${ICON_NEW} Username (leave empty to skip): "
    read -r assignee
  fi

  if [[ -n "$assignee" ]]; then
    echo "  ${GREEN}${ICON_OK}${RESET} ${DIM}${assignee}${RESET}"
  else
    echo "  ${DIM}  none${RESET}"
  fi

  # ═══════════════════════════════════════════
  # MILESTONE
  # ═══════════════════════════════════════════
  echo ""
  hr
  echo "  ${MAGENTA}${ICON_MILE}${RESET} ${BOLD}Milestone${RESET} ${DIM}(ENTER=confirm, ESC=abort)${RESET}"
  echo ""

  milestone=""
  planned_ms_title=""
  planned_ms_due=""
  ms_options="  Skip (no milestone)
${ICON_NEW} Create new milestone..."
  if [[ -n "$milestones" ]]; then
    ms_options="  Skip (no milestone)
${ICON_NEW} Create new milestone...
${milestones}"
  fi

  selected_ms=$(fzf \
    --prompt="  Milestone > " \
    --header="  ENTER=select  ESC=abort" \
    --height=~40 \
    --reverse \
    --border=rounded \
    --border-label=" milestones " \
    --color="border:magenta,header:dim,prompt:magenta" \
    <<< "$ms_options" \
    || echo "")

  if [[ -z "$selected_ms" ]]; then
    echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted."
    exit 0
  fi

  if grep -q "Create new milestone" <<< "$selected_ms"; then
    echo ""
    echo -n "  ${ICON_NEW} Milestone title: "
    read -r ms_title
    if [[ -n "$ms_title" ]]; then
      echo -n "  ${ICON_NEW} Due date (${DIM}YYYY-MM-DD${RESET}, empty=none): "
      read -r ms_due
      planned_ms_title="$ms_title"
      planned_ms_due="$ms_due"
      milestone="$ms_title"
      echo "  ${DIM}Milestone ${BOLD}${ms_title}${RESET} will be created after confirmation.${RESET}"
    fi
  elif grep -q "Skip" <<< "$selected_ms"; then
    milestone=""
  elif [[ -n "$selected_ms" ]]; then
    milestone="$selected_ms"
  fi

  if [[ -n "$milestone" ]]; then
    echo "  ${GREEN}${ICON_OK}${RESET} ${DIM}${milestone}${RESET}"
  else
    echo "  ${DIM}  none${RESET}"
  fi

  # ═══════════════════════════════════════════
  # DESCRIPTION
  # ═══════════════════════════════════════════
  echo ""
  hr
  echo "  ${MAGENTA}${ICON_DESC}${RESET} ${BOLD}Description${RESET}"
  echo ""

  tmpfile=$(mktemp "${TMPDIR:-/tmp}/gl-issue-XXXXXX")
  _tmpfiles+=("$tmpfile")

  DESCRIPTION_TEMPLATE='## Context


## Acceptance Criteria
- [ ] ...

## Dependencies
- Blocked by: #
- Enables: #'

  cat > "$tmpfile" <<< "$DESCRIPTION_TEMPLATE"

  echo "  ${DIM}Opening editor... (save & quit to continue, :cq to abort)${RESET}"
  echo ""

  while true; do
    open_editor "$tmpfile"
    editor_status=$?

    # :cq in vim/nvim exits with status 1 — treat as explicit abort
    if [[ $editor_status -ne 0 ]]; then
      echo ""
      echo "  ${YELLOW}${ICON_WARN}${RESET} Editor exited with error (status $editor_status). Aborting."
      exit 1
    fi

    # Guard against deleted temp file (e.g. from signal during editing)
    if [[ ! -f "$tmpfile" ]]; then
      echo ""
      echo "  ${RED}${ICON_WARN}${RESET} Description file was lost. Aborting."
      exit 1
    fi

    description=$(cat "$tmpfile")

    # If description is empty/whitespace-only or unchanged, offer to retry
    desc_stripped="${description//[$' \t\n\r']/}"
    desc_trimmed_check=$(awk '{$1=$1};1' <<< "$description" | tr -s '\n')
    template_trimmed_check=$(awk '{$1=$1};1' <<< "$DESCRIPTION_TEMPLATE" | tr -s '\n')

    if [[ -z "$desc_stripped" ]]; then
      echo "  ${YELLOW}${ICON_WARN}${RESET} Description is empty."
      echo -n "  ${BOLD}(r)etry / (c)ontinue / (a)bort?${RESET} "
      read -r empty_choice
      case "$empty_choice" in
        r|R) cat > "$tmpfile" <<< "$DESCRIPTION_TEMPLATE"; continue ;;
        a|A) echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted."; exit 0 ;;
        *)   break ;;
      esac
    elif [[ "$desc_trimmed_check" == "$template_trimmed_check" ]]; then
      echo "  ${YELLOW}${ICON_WARN}${RESET} Description was not modified from template."
      echo -n "  ${BOLD}(r)etry / (c)ontinue / (a)bort?${RESET} "
      read -r unchanged_choice
      case "$unchanged_choice" in
        r|R) continue ;;
        a|A) echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted."; exit 0 ;;
        *)   break ;;
      esac
    else
      break
    fi
  done


  # ═══════════════════════════════════════════
  # SUMMARY
  # ═══════════════════════════════════════════

  # Collect all summary lines (plain text for width calculation)
  summary_lines=()
  summary_lines+=("Summary")
  summary_lines+=("---")  # separator marker
  summary_lines+=("${ICON_TITLE} Title:     ${title}")
  summary_lines+=("${ICON_LABEL} Labels:    ${labels_csv:-none}")
  summary_lines+=("${ICON_USER} Assignee:  ${assignee:-none}")
  summary_lines+=("${ICON_MILE} Milestone: ${milestone:-none}")
  summary_lines+=("---")  # separator marker
  summary_lines+=("${ICON_DESC} Description:")

  # Wrap description lines at 70 chars and collect them
  desc_wrapped=()
  while IFS= read -r line; do
    if [[ ${#line} -gt 70 ]]; then
      while IFS= read -r wrapped; do
        desc_wrapped+=("$wrapped")
      done < <(wrap_text_for_issue_summary "$line" 70)
    else
      desc_wrapped+=("$line")
    fi
  done <<< "$description"

  for dline in "${desc_wrapped[@]}"; do
    summary_lines+=("   ${dline}")
  done

  # Find the widest line
  max_w=0
  for sline in "${summary_lines[@]}"; do
    if [[ "$sline" == "---" ]]; then continue; fi
    local_len=${#sline}
    if [[ $local_len -gt $max_w ]]; then
      max_w=$local_len
    fi
  done

  # Add padding (2 left + 2 right)
  sum_box_w=$((max_w + 4))
  if [[ $sum_box_w -lt 30 ]]; then sum_box_w=30; fi

  sum_border=$(printf '─%.0s' $(seq 1 $sum_box_w))

  # Print summary box
  echo ""
  echo "  ${BOX}┌${sum_border}┐${RESET}"

  for sline in "${summary_lines[@]}"; do
    if [[ "$sline" == "---" ]]; then
      echo "  ${BOX}├${sum_border}┤${RESET}"
      continue
    fi
    local_len=${#sline}
    pad=$((sum_box_w - local_len - 2))
    if [[ $pad -lt 0 ]]; then pad=0; fi
    spaces=""
    if [[ $pad -gt 0 ]]; then
      spaces=$(printf '%*s' "$pad" '')
    fi
    echo "  ${BOX_V} ${sline}${spaces} ${BOX}│${RESET}"
  done

  echo "  ${BOX}└${sum_border}┘${RESET}"
  echo ""

  echo -n "  ${BOLD}Create this issue?${RESET} ${DIM}(y/n)${RESET} "
  read -r confirm

  if [[ "$confirm" != "y" ]]; then
    echo ""
    echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted."
    exit 0
  fi

  if ! require_writes_allowed "create manual GitLab issue"; then
    exit 1
  fi

  if [[ -n "$planned_ms_title" ]]; then
    ms_cmd=(glab api "projects/$project_id/milestones" -X POST -f "title=$planned_ms_title")
    [[ -n "$planned_ms_due" ]] && ms_cmd+=(-f "due_date=$planned_ms_due")
    if ! require_writes_allowed "create GitLab milestone" || ! "${ms_cmd[@]}" &>/dev/null; then
      echo "  ${RED}${ICON_WARN}${RESET} Required milestone could not be created; issue was not created."
      exit 1
    fi
    if ! ms_json=$(fetch_all_milestones) \
      || ! jq -e --arg title "$planned_ms_title" 'any(.[]; .title == $title)' \
        <<< "$ms_json" &>/dev/null; then
      echo "  ${RED}${ICON_WARN}${RESET} Required milestone could not be verified; issue was not created."
      exit 1
    fi
  fi

  created_planned_labels=false
  for (( i=1; i<=${#planned_label_names[@]}; i++ )); do
    new_label_name="${planned_label_names[$i]}"
    if ! grep -qxF "$new_label_name" <<< "$(csv_to_lines "$labels_csv")"; then
      continue
    fi
    create_cmd=(glab label create -n "$new_label_name")
    [[ -n "${planned_label_colors[$i]}" ]] && create_cmd+=(-c "${planned_label_colors[$i]}")
    if ! require_writes_allowed "create GitLab label" || ! "${create_cmd[@]}" &>/dev/null; then
      echo "  ${RED}${ICON_WARN}${RESET} Required label '${new_label_name}' could not be created; issue was not created."
      exit 1
    fi
    created_planned_labels=true
  done
  if $created_planned_labels; then
    if ! labels_json=$(fetch_all_labels); then
      echo "  ${RED}${ICON_WARN}${RESET} Required labels could not be verified; issue was not created."
      exit 1
    fi
    existing_labels=$(jq -r '.[].name' <<< "$labels_json")
    while IFS= read -r lbl; do
      [[ -z "$lbl" ]] && continue
      if ! grep -qxF "$lbl" <<< "$existing_labels"; then
        echo "  ${RED}${ICON_WARN}${RESET} Required label '${lbl}' is unavailable; issue was not created."
        exit 1
      fi
    done < <(csv_to_lines "$labels_csv")
  fi

  # ─── Build & execute command ───
  cmd=(glab issue create -t "$title")

  if [[ -n "$labels_csv" ]]; then
    cmd+=(-l "$labels_csv")
  fi

  if [[ -n "$assignee" ]]; then
    cmd+=(-a "$assignee")
  fi

  if [[ -n "$milestone" ]]; then
    cmd+=(-m "$milestone")
  fi

  cmd+=(-d "$description")

  echo ""
  echo -n "  ${DIM}Creating issue...${RESET}"
  if require_writes_allowed "create GitLab issue"; then
    output=$("${cmd[@]}" 2>&1)
    create_status=$?
  else
    output="write blocked"
    create_status=1
  fi
  printf "\r                       \r"

  if [[ $create_status -eq 0 ]]; then
    echo ""
    echo "  ${GREEN}${ICON_OK} Issue created successfully${RESET}"
    echo "  ${DIM}${output}${RESET}"
    echo ""
  else
    echo ""
    echo "  ${RED}${ICON_WARN} Failed to create issue${RESET}"
    echo "  ${DIM}${output}${RESET}"
    echo ""
    exit 1
  fi

  # ═══════════════════════════════════════════
  # BRANCH CREATION (after issue create)
  # ═══════════════════════════════════════════

  # Extract issue number from glab output URL (e.g. .../issues/42)
  issue_number=$(grep -oE '/issues/[0-9]+' <<< "$output" | grep -oE '[0-9]+' | tail -1)

  if [[ -n "$issue_number" ]]; then
    offer_branch_creation "$issue_number" "$title"
  fi
}
