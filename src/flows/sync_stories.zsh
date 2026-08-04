sync_stories() {
  local sync_mode="${1:-apply}"
  local dry_run=false
  local gl_issues_json synced_keys unsynced_json synced_json
  local unsynced_count synced_count preview_uptodate=0 gl_issue_count=0 existing_milestone_count=0
  local ms_json labels_json existing_labels plan_labels_csv
  local ms_match ms_id old_ms_title old_ms_desc
  local gl_issue gl_iid gl_title gl_description gl_labels_csv gl_milestone gl_state
  local new_title new_description merged_labels final_labels new_milestone change_summary
  local issue_plan milestone_plan labels_csv lbl
  local title_changed description_changed labels_changed labels_visible_changed milestone_changed status_changed
  local s_key s_summary s_desc s_priority s_labels_json s_epic_key s_subtasks_json s_status_name s_status_category
  local desired_status_rank current_status_rank target_status_label status_state_event status_from status_to unknown_status_key
  local created=0 failed=0 issue_updated=0 issue_uptodate=0 labels_created=0 label_failed=0
  local ms_created=0 ms_updated=0 ms_failed=0
  local cmd output created_iid issue_create_status_rank dependency_failed
  local ms_title ms_desc ignored_title story_uses_ignored_epic=false
  local -a new_ms_plan=()
  local -a update_ms_plan=()
  local -a issue_create_plan=()
  local -a issue_update_plan=()
  local -a missing_project_labels=()
  local -a ignored_epic_plan=()
  local -a unresolved_epic_plan=()
  local -a unknown_status_plan=()
  local -a changed_fields=()
  typeset -A epic_titles
  typeset -A epic_descs
  typeset -A ignored_epic_titles
  typeset -A unresolved_epic_seen
  typeset -A missing_label_seen
  typeset -A unknown_status_seen

  [[ "$sync_mode" == "dry-run" || "${DRY_RUN_MODE:-false}" == "true" ]] && dry_run=true

  echo ""
  echo "  ${MAGENTA}${ICON_SYNC}${RESET} ${BOLD}Sync Stories from Jira${RESET}"
  echo ""

  echo -n "  ${DIM}Fetching Jira stories...${RESET}"
  if ! fetch_jira_stories_paginated; then
    printf "\r                                      \r"
    echo "  ${RED}${ICON_WARN}${RESET} Could not connect to Jira."
    echo "  ${DIM}Check VPN/Wireguard connection.${RESET}"
    echo ""
    return 1
  fi
  printf "\r                                      \r"
  echo "  ${GREEN}${ICON_OK}${RESET} ${DIM}${JIRA_STORIES_COUNT} stories fetched from Jira${RESET}"

  echo -n "  ${DIM}Checking synced issues...${RESET}"
  if ! gl_issues_json=$(fetch_all_issues "all"); then
    printf "\r                                      \r"
    echo "  ${RED}${ICON_WARN}${RESET} Story sync aborted because GitLab issues could not be read completely."
    echo ""
    return 1
  fi
  synced_keys=$(jq -r '.[].title | capture("^\\[(?<key>[A-Z]+-[0-9]+)\\](\\s|$)")?.key // empty' <<< "$gl_issues_json")
  gl_issue_count=$(jq 'length' <<< "$gl_issues_json")
  printf "\r                                      \r"

  unsynced_json=$(jq --arg synced "$synced_keys" '
    ($synced | split("\n") | map(select(length > 0))) as $keys |
    map(select(.key as $k | ($keys | index($k)) | not))
  ' <<< "$JIRA_STORIES_JSON")
  synced_json=$(jq --arg synced "$synced_keys" '
    ($synced | split("\n") | map(select(length > 0))) as $keys |
    map(select(.key as $k | ($keys | index($k)) | not | not))
  ' <<< "$JIRA_STORIES_JSON")
  unsynced_count=$(jq 'length' <<< "$unsynced_json")
  synced_count=$(jq 'length' <<< "$synced_json")

  if [[ "$unsynced_count" -eq 0 && "$synced_count" -eq 0 ]]; then
    echo "  ${DIM}No stories found matching board labels.${RESET}"
    echo ""
    return 0
  fi

  echo -n "  ${DIM}Fetching epics...${RESET}"
  if ! fetch_jira_epics; then
    printf "\r                                      \r"
    echo "  ${RED}${ICON_WARN}${RESET} Story sync aborted because Jira epic data is incomplete."
    echo "  ${DIM}No milestone or issue changes were planned or applied.${RESET}"
    echo ""
    return 1
  else
    printf "\r                                      \r"
    echo "  ${GREEN}${ICON_OK}${RESET} ${DIM}${JIRA_EPICS_COUNT} epics fetched from Jira${RESET}"
    local ekey etitle edesc mdesc
    while IFS= read -r epic; do
      [[ -z "$epic" ]] && continue
      ekey=$(jq -r '.key' <<< "$epic")
      etitle=$(trim_whitespace "$(jq -r '.fields.summary // ""' <<< "$epic")")
      edesc=$(jq -r '.fields.description // ""' <<< "$epic")
      if is_suspicious_jira_epic_title "$etitle"; then
        ignored_title="${etitle:-<empty>}"
        ignored_epic_titles[$ekey]="$ignored_title"
        ignored_epic_plan+=("$(jq -n --arg key "$ekey" --arg title "$ignored_title" '{key:$key, title:$title}')")
        continue
      fi
      if [[ -n "$etitle" ]]; then
        epic_titles[$ekey]="$etitle"
        mdesc=$(jira_to_markdown "$edesc")
        epic_descs[$ekey]="${mdesc:+$mdesc

}<!-- jira:${ekey} -->"
      fi
    done < <(jq -c '.[]' <<< "$JIRA_EPICS_JSON")
  fi

  if ! ms_json=$(fetch_all_milestones); then
    echo "  ${RED}${ICON_WARN}${RESET} Story sync aborted because GitLab milestones could not be read completely."
    echo ""
    return 1
  fi
  existing_milestone_count=$(jq 'length' <<< "$ms_json")

  for ekey in "${(@k)epic_titles}"; do
    ms_match=$(jq -c --arg k "$ekey" '[.[] | select(.description // "" | contains("<!-- jira:" + $k + " -->"))][0] // empty' <<< "$ms_json" 2>/dev/null)
    if [[ -z "$ms_match" ]]; then
      ms_match=$(jq -c --arg t "${epic_titles[$ekey]}" '[.[] | select(.title == $t)][0] // empty' <<< "$ms_json" 2>/dev/null)
    fi

    ms_id=""
    old_ms_title=""
    old_ms_desc=""
    if [[ -n "$ms_match" ]]; then
      ms_id=$(jq -r '.id // empty' <<< "$ms_match")
      old_ms_title=$(jq -r '.title // ""' <<< "$ms_match")
      old_ms_desc=$(jq -r '.description // ""' <<< "$ms_match")
    fi

    if [[ -z "$ms_id" ]]; then
      milestone_plan=$(jq -n \
        --arg key "$ekey" \
        --arg title "${epic_titles[$ekey]}" \
        --arg description "${epic_descs[$ekey]:-}" \
        '{key:$key, title:$title, description:$description}')
      new_ms_plan+=("$milestone_plan")
    else
      title_changed=false
      description_changed=false
      [[ "$(trim_whitespace "${epic_titles[$ekey]}")" != "$(trim_whitespace "$old_ms_title")" ]] && title_changed=true
      [[ "${epic_descs[$ekey]:-}" != "$old_ms_desc" ]] && description_changed=true

      if $title_changed || $description_changed; then
        change_summary="description"
        if $title_changed && $description_changed; then
          change_summary="title, description"
        elif $title_changed; then
          change_summary="title"
        fi

        milestone_plan=$(jq -n \
          --arg id "$ms_id" \
          --arg key "$ekey" \
          --arg old_title "$old_ms_title" \
          --arg new_title "${epic_titles[$ekey]}" \
          --arg old_description "$old_ms_desc" \
          --arg new_description "${epic_descs[$ekey]:-}" \
          --arg change_summary "$change_summary" \
          --argjson title_changed "$title_changed" \
          --argjson description_changed "$description_changed" \
          '{id:($id|tonumber), key:$key, old_title:$old_title, new_title:$new_title, old_description:$old_description, new_description:$new_description, change_summary:$change_summary, title_changed:$title_changed, description_changed:$description_changed}')
        update_ms_plan+=("$milestone_plan")
      fi
    fi
  done

  while IFS= read -r story; do
    s_key=$(jq -r '.key' <<< "$story")
    s_summary=$(trim_whitespace "$(jq -r '.fields.summary // ""' <<< "$story")")
    s_desc=$(jq -r '.fields.description // ""' <<< "$story")
    s_priority=$(jq -r '.fields.priority.name // ""' <<< "$story")
    s_labels_json=$(jq '.fields.labels // []' <<< "$story")
    s_epic_key=$(jq -r '.fields.customfield_10000 // ""' <<< "$story")
    s_subtasks_json=$(jq '.fields.subtasks // []' <<< "$story")
    s_status_name=$(jq -r '.fields.status.name // ""' <<< "$story")
    s_status_category=$(jq -r '.fields.status.statusCategory.key // ""' <<< "$story")

    new_title="[${s_key}] ${s_summary}"
    new_description=$(build_story_sync_description "$s_desc" "$s_subtasks_json")
    labels_csv=$(build_story_sync_labels_csv "$s_labels_json" "$s_priority")
    desired_status_rank=$(jira_status_rank "$s_status_name" "$s_status_category")
    if [[ "$desired_status_rank" -lt 0 ]]; then
      unknown_status_key="${s_status_name}|${s_status_category}"
      if [[ -n "$s_status_name" && -z "${unknown_status_seen[$unknown_status_key]:-}" ]]; then
        unknown_status_plan+=("$(jq -n --arg name "$s_status_name" --arg category "$s_status_category" '{name:$name, category:$category}')")
        unknown_status_seen[$unknown_status_key]=1
      fi
      desired_status_rank=0
    fi
    labels_csv=$(normalize_status_labels_csv "$labels_csv" "$(gitlab_status_label_for_rank "$desired_status_rank")")
    new_milestone=""
    if [[ -n "$s_epic_key" && "$s_epic_key" != "null" ]]; then
      if [[ -n "${epic_titles[$s_epic_key]:-}" ]]; then
        new_milestone="${epic_titles[$s_epic_key]:-}"
      elif [[ -z "${ignored_epic_titles[$s_epic_key]:-}" && -z "${unresolved_epic_seen[$s_epic_key]:-}" ]]; then
        unresolved_epic_plan+=("$s_epic_key")
        unresolved_epic_seen[$s_epic_key]=1
      fi
    fi

    issue_plan=$(jq -n \
      --arg key "$s_key" \
      --arg summary "$s_summary" \
      --arg title "$new_title" \
      --arg description "$new_description" \
      --arg labels "$labels_csv" \
      --arg milestone "$new_milestone" \
      --arg status_display "$(status_rank_display "$desired_status_rank")" \
      --argjson status_rank "$desired_status_rank" \
      '{key:$key, summary:$summary, title:$title, description:$description, labels:$labels, milestone:$milestone, status_display:$status_display, status_rank:$status_rank}')
    issue_create_plan+=("$issue_plan")
  done < <(jq -c '.[]' <<< "$unsynced_json")

  if [[ "$synced_count" -gt 0 ]]; then
    while IFS= read -r story; do
      s_key=$(jq -r '.key' <<< "$story")
      s_summary=$(trim_whitespace "$(jq -r '.fields.summary // ""' <<< "$story")")
      s_desc=$(jq -r '.fields.description // ""' <<< "$story")
      s_priority=$(jq -r '.fields.priority.name // ""' <<< "$story")
      s_labels_json=$(jq '.fields.labels // []' <<< "$story")
      s_epic_key=$(jq -r '.fields.customfield_10000 // ""' <<< "$story")
      s_subtasks_json=$(jq '.fields.subtasks // []' <<< "$story")
      s_status_name=$(jq -r '.fields.status.name // ""' <<< "$story")
      s_status_category=$(jq -r '.fields.status.statusCategory.key // ""' <<< "$story")

      gl_issue=$(jq -c --arg k "$s_key" '.[] | select(.title | test("^\\[" + $k + "\\](\\s|$)"))' <<< "$gl_issues_json" | head -1)
      [[ -z "$gl_issue" ]] && continue
      gl_iid=$(jq -r '.iid' <<< "$gl_issue")
      gl_title=$(jq -r '.title' <<< "$gl_issue")
      gl_description=$(jq -r '.description // ""' <<< "$gl_issue")
      gl_labels_csv=$(jq -r '[.labels[]?] | join(",")' <<< "$gl_issue")
      gl_milestone=$(jq -r '.milestone.title // ""' <<< "$gl_issue")
      gl_state=$(jq -r '.state // "opened"' <<< "$gl_issue")

      new_title="[${s_key}] ${s_summary}"
      new_description=$(build_story_sync_description "$s_desc" "$s_subtasks_json")

      merged_labels="$gl_labels_csv"
      while IFS= read -r lbl; do
        [[ -z "$lbl" ]] && continue
        if ! tr ',' '\n' <<< "$merged_labels" | grep -qxF "$lbl"; then
          merged_labels="${merged_labels:+$merged_labels,}${lbl}"
        fi
      done < <(jq -r '.[]' <<< "$s_labels_json")
      if [[ -n "$s_priority" ]]; then
        merged_labels=$(echo "$merged_labels" | tr ',' '\n' | grep -v '^prio::' | tr '\n' ',' | sed 's/,$//')
        merged_labels="${merged_labels:+$merged_labels,}prio::${s_priority}"
      fi

      new_milestone=""
      story_uses_ignored_epic=false
      if [[ -n "$s_epic_key" && "$s_epic_key" != "null" ]]; then
        if [[ -n "${ignored_epic_titles[$s_epic_key]:-}" ]]; then
          story_uses_ignored_epic=true
          new_milestone="$gl_milestone"
        elif [[ -z "${epic_titles[$s_epic_key]:-}" ]]; then
          story_uses_ignored_epic=true
          new_milestone="$gl_milestone"
          if [[ -z "${unresolved_epic_seen[$s_epic_key]:-}" ]]; then
            unresolved_epic_plan+=("$s_epic_key")
            unresolved_epic_seen[$s_epic_key]=1
          fi
        else
          new_milestone="${epic_titles[$s_epic_key]:-}"
        fi
      fi

      desired_status_rank=$(jira_status_rank "$s_status_name" "$s_status_category")
      if [[ "$desired_status_rank" -lt 0 ]]; then
        unknown_status_key="${s_status_name}|${s_status_category}"
        if [[ -n "$s_status_name" && -z "${unknown_status_seen[$unknown_status_key]:-}" ]]; then
          unknown_status_plan+=("$(jq -n --arg name "$s_status_name" --arg category "$s_status_category" '{name:$name, category:$category}')")
          unknown_status_seen[$unknown_status_key]=1
        fi
      fi
      current_status_rank=$(gitlab_issue_status_rank "$gl_state" "$gl_labels_csv")
      status_changed=false
      status_state_event=""
      final_labels="$merged_labels"
      status_from="$(status_rank_display "$current_status_rank")"
      status_to="$status_from"
      if [[ "$desired_status_rank" -ge 0 && "$desired_status_rank" -gt "$current_status_rank" ]]; then
        status_changed=true
        status_to="$(status_rank_display "$desired_status_rank")"
        target_status_label="$(gitlab_status_label_for_rank "$desired_status_rank")"
        final_labels="$(normalize_status_labels_csv "$merged_labels" "$target_status_label")"
        if [[ "$desired_status_rank" -eq 3 ]]; then
          status_state_event="close"
        fi
      fi

      title_changed=false
      description_changed=false
      labels_changed=false
      labels_visible_changed=false
      milestone_changed=false
      [[ "$(trim_whitespace "$new_title")" != "$(trim_whitespace "$gl_title")" ]] && title_changed=true
      [[ "$new_description" != "$gl_description" ]] && description_changed=true
      if ! csv_sets_equal "$final_labels" "$gl_labels_csv"; then
        labels_changed=true
      fi
      if ! csv_sets_equal_ignoring_status "$final_labels" "$gl_labels_csv"; then
        labels_visible_changed=true
      fi
      if ! $story_uses_ignored_epic && [[ "$(trim_whitespace "$new_milestone")" != "$(trim_whitespace "$gl_milestone")" ]]; then
        milestone_changed=true
      fi

      if ! $title_changed && ! $description_changed && ! $labels_changed && ! $milestone_changed && ! $status_changed; then
        ((preview_uptodate++))
        continue
      fi

      changed_fields=()
      $title_changed && changed_fields+=("title")
      $description_changed && changed_fields+=("description")
      $labels_visible_changed && changed_fields+=("labels")
      $milestone_changed && changed_fields+=("milestone")
      $status_changed && changed_fields+=("status")
      change_summary="${(j:, :)changed_fields}"

      issue_plan=$(jq -n \
        --arg iid "$gl_iid" \
        --arg key "$s_key" \
        --arg summary "$s_summary" \
        --arg old_title "$gl_title" \
        --arg new_title "$new_title" \
        --arg old_description "$gl_description" \
        --arg new_description "$new_description" \
        --arg old_labels "$gl_labels_csv" \
        --arg new_labels "$final_labels" \
        --arg old_milestone "$gl_milestone" \
        --arg new_milestone "$new_milestone" \
        --arg old_status "$status_from" \
        --arg new_status "$status_to" \
        --arg state_event "$status_state_event" \
        --arg change_summary "$change_summary" \
        --argjson title_changed "$title_changed" \
        --argjson description_changed "$description_changed" \
        --argjson labels_changed "$labels_changed" \
        --argjson labels_visible_changed "$labels_visible_changed" \
        --argjson milestone_changed "$milestone_changed" \
        --argjson status_changed "$status_changed" \
        '{iid:($iid|tonumber), key:$key, summary:$summary, old_title:$old_title, new_title:$new_title, old_description:$old_description, new_description:$new_description, old_labels:$old_labels, new_labels:$new_labels, old_milestone:$old_milestone, new_milestone:$new_milestone, old_status:$old_status, new_status:$new_status, state_event:$state_event, change_summary:$change_summary, title_changed:$title_changed, description_changed:$description_changed, labels_changed:$labels_changed, labels_visible_changed:$labels_visible_changed, milestone_changed:$milestone_changed, status_changed:$status_changed}')
      issue_update_plan+=("$issue_plan")
    done < <(jq -c '.[]' <<< "$synced_json")
  fi

  if [[ ${#unresolved_epic_plan[@]} -gt 0 ]]; then
    echo ""
    echo "  ${RED}${ICON_WARN}${RESET} Story sync aborted because referenced Jira epics are missing:"
    for ekey in "${unresolved_epic_plan[@]}"; do
      echo "    ${ekey}"
    done
    echo "  ${DIM}Milestone changes are suspended; existing assignments remain unchanged.${RESET}"
    echo ""
    return 1
  fi

  echo -n "  ${DIM}Checking project labels...${RESET}"
  if ! labels_json=$(fetch_all_labels); then
    printf "\r                                      \r"
    echo "  ${RED}${ICON_WARN}${RESET} Story sync aborted because GitLab labels could not be read completely."
    echo ""
    return 1
  fi
  existing_labels=$(jq -r '.[].name' <<< "$labels_json")
  for issue_plan in "${issue_create_plan[@]}" "${issue_update_plan[@]}"; do
    [[ -z "$issue_plan" ]] && continue
    plan_labels_csv=$(jq -r '.labels // .new_labels // ""' <<< "$issue_plan")
    while IFS= read -r lbl; do
      [[ -z "$lbl" ]] && continue
      if ! grep -qxF "$lbl" <<< "$existing_labels" && [[ -z "${missing_label_seen[$lbl]:-}" ]]; then
        missing_project_labels+=("$lbl")
        missing_label_seen[$lbl]=1
      fi
    done < <(csv_to_lines "$plan_labels_csv")
  done
  printf "\r                                      \r"

  echo ""
  hr
  echo ""
  echo "  ${BOLD}Preview${RESET}"
  echo ""
  if [[ ${#new_ms_plan[@]} -gt 0 ]]; then
    echo "  ${ICON_MILE} ${#new_ms_plan[@]} milestones to create:"
    for milestone_plan in "${new_ms_plan[@]}"; do
      echo "    $(jq -r '.title' <<< "$milestone_plan")"
    done
  fi
  if [[ ${#update_ms_plan[@]} -gt 0 ]]; then
    echo "  ${ICON_SYNC} ${#update_ms_plan[@]} milestones to update:"
    for milestone_plan in "${update_ms_plan[@]}"; do
      echo "    $(jq -r '.new_title' <<< "$milestone_plan") ${DIM}($(jq -r '.change_summary' <<< "$milestone_plan"))${RESET}"
      if [[ "$(jq -r '.title_changed' <<< "$milestone_plan")" == "true" ]]; then
        echo "      title: $(preview_display_value "$(jq -r '.old_title' <<< "$milestone_plan")") -> $(preview_display_value "$(jq -r '.new_title' <<< "$milestone_plan")")"
      fi
      if [[ "$(jq -r '.description_changed' <<< "$milestone_plan")" == "true" ]]; then
        echo "      description: changed"
      fi
    done
  fi
  if [[ ${#ignored_epic_plan[@]} -gt 0 ]]; then
    echo "  ${YELLOW}${ICON_WARN}${RESET} ${#ignored_epic_plan[@]} suspicious Jira epics ignored:"
    for milestone_plan in "${ignored_epic_plan[@]}"; do
      echo "    [$(jq -r '.key' <<< "$milestone_plan")] $(jq -r '.title' <<< "$milestone_plan")"
    done
  fi
  if [[ ${#unknown_status_plan[@]} -gt 0 ]]; then
    echo "  ${YELLOW}${ICON_WARN}${RESET} ${#unknown_status_plan[@]} Jira statuses not mapped:"
    for issue_plan in "${unknown_status_plan[@]}"; do
      echo "    $(jq -r '.name' <<< "$issue_plan") ${DIM}($(jq -r '.category // "unknown"' <<< "$issue_plan"))${RESET}"
    done
  fi
  if [[ ${#missing_project_labels[@]} -gt 0 ]]; then
    echo "  ${ICON_LABEL} ${#missing_project_labels[@]} project labels to create:"
    for lbl in "${missing_project_labels[@]}"; do
      echo "    ${lbl}"
    done
  fi
  if [[ ${#issue_create_plan[@]} -gt 0 ]]; then
    echo "  ${GREEN}${ICON_NEW}${RESET} ${#issue_create_plan[@]} issues to create:"
    for issue_plan in "${issue_create_plan[@]}"; do
      if [[ "$(jq -r '.status_rank' <<< "$issue_plan")" -gt 0 ]]; then
        echo "    [$(jq -r '.key' <<< "$issue_plan")] $(jq -r '.summary' <<< "$issue_plan") ${DIM}(status: $(jq -r '.status_display' <<< "$issue_plan"))${RESET}"
      else
        echo "    [$(jq -r '.key' <<< "$issue_plan")] $(jq -r '.summary' <<< "$issue_plan")"
      fi
    done
  fi
  if [[ ${#issue_update_plan[@]} -gt 0 ]]; then
    echo "  ${ICON_SYNC} ${#issue_update_plan[@]} issue updates planned:"
    for issue_plan in "${issue_update_plan[@]}"; do
      echo "    [$(jq -r '.key' <<< "$issue_plan")] $(jq -r '.summary' <<< "$issue_plan") ${DIM}($(jq -r '.change_summary' <<< "$issue_plan"))${RESET}"
      if [[ "$(jq -r '.title_changed' <<< "$issue_plan")" == "true" ]]; then
        echo "      title: $(preview_display_value "$(jq -r '.old_title' <<< "$issue_plan")") -> $(preview_display_value "$(jq -r '.new_title' <<< "$issue_plan")")"
      fi
      if [[ "$(jq -r '.description_changed' <<< "$issue_plan")" == "true" ]]; then
        echo "      description: changed"
      fi
      if [[ "$(jq -r '.labels_visible_changed' <<< "$issue_plan")" == "true" ]]; then
        echo "      labels:"
        print_story_label_preview_details "$(jq -r '.old_labels' <<< "$issue_plan")" "$(jq -r '.new_labels' <<< "$issue_plan")"
      fi
      if [[ "$(jq -r '.milestone_changed' <<< "$issue_plan")" == "true" ]]; then
        echo "      milestone: $(preview_display_value "$(jq -r '.old_milestone' <<< "$issue_plan")") -> $(preview_display_value "$(jq -r '.new_milestone' <<< "$issue_plan")")"
      fi
      if [[ "$(jq -r '.status_changed' <<< "$issue_plan")" == "true" ]]; then
        echo "      status: $(jq -r '.old_status' <<< "$issue_plan") -> $(jq -r '.new_status' <<< "$issue_plan")"
      fi
    done
  elif [[ $synced_count -gt 0 ]]; then
    echo "  ${DIM}No issue updates required.${RESET}"
  fi
  if [[ $preview_uptodate -gt 0 ]]; then
    echo "  ${DIM}${preview_uptodate} synced issues already up-to-date${RESET}"
  fi
  echo ""
  hr
  echo ""

  if $dry_run; then
    echo "  ${GREEN}${ICON_OK}${RESET} ${BOLD}Dry-run complete${RESET}"
    echo ""
    local preview_issue_parts="${GREEN}${#issue_create_plan[@]} to create${RESET}"
    [[ ${#issue_update_plan[@]} -gt 0 ]] && preview_issue_parts="${preview_issue_parts}, ${YELLOW}${#issue_update_plan[@]} to update${RESET}"
    [[ $preview_uptodate -gt 0 ]] && preview_issue_parts="${preview_issue_parts}, ${DIM}${preview_uptodate} up-to-date${RESET}"
    echo "  ${ICON_TITLE} Issues:     ${preview_issue_parts}"
    local preview_ms_parts=""
    [[ ${#new_ms_plan[@]} -gt 0 ]] && preview_ms_parts="${#new_ms_plan[@]} to create"
    [[ ${#update_ms_plan[@]} -gt 0 ]] && preview_ms_parts="${preview_ms_parts:+$preview_ms_parts, }${#update_ms_plan[@]} to update"
    if [[ -n "$preview_ms_parts" ]]; then
      echo "  ${ICON_MILE} Milestones: ${YELLOW}${preview_ms_parts}${RESET}"
    fi
    if [[ ${#missing_project_labels[@]} -gt 0 ]]; then
      echo "  ${ICON_LABEL} Labels:     ${YELLOW}${#missing_project_labels[@]} to create${RESET}"
    fi
    echo "  ${DIM}No GitLab changes were applied.${RESET}"
    echo ""
    return 0
  fi

  echo -n "  ${BOLD}Proceed?${RESET} ${DIM}(y/n)${RESET} "
  read -r confirm
  if [[ "$confirm" != "y" ]]; then
    echo ""
    echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted."
    return 0
  fi

  echo ""
  if [[ "$gl_issue_count" -gt 0 || "$existing_milestone_count" -gt 0 ]]; then
    echo -n "  ${BOLD}Create local snapshot first?${RESET} ${DIM}(y/n)${RESET} "
    read -r snapshot_confirm
    if [[ "$snapshot_confirm" == "y" ]]; then
      echo ""
      if ! export_gitlab_snapshot "pre-sync-stories"; then
        echo "  ${RED}${ICON_WARN}${RESET} Sync aborted because snapshot export failed."
        echo ""
        return 1
      fi
    fi
  else
    echo "  ${DIM}Skipping snapshot prompt: no existing issues or milestones to back up${RESET}"
  fi

  echo ""

  if [[ ${#new_ms_plan[@]} -gt 0 || ${#update_ms_plan[@]} -gt 0 ]]; then
    echo "  ${DIM}Creating milestones...${RESET}"
    echo ""
  fi
  for milestone_plan in "${new_ms_plan[@]}"; do
    ms_title=$(jq -r '.title' <<< "$milestone_plan")
    ms_desc=$(jq -r '.description // ""' <<< "$milestone_plan")
    cmd=(glab api "projects/$project_id/milestones" -X POST -f "title=$ms_title")
    [[ -n "$ms_desc" ]] && cmd+=(-f "description=$ms_desc")
    if require_writes_allowed "create GitLab milestone" && "${cmd[@]}" &>/dev/null; then
      echo "  ${GREEN}${ICON_OK}${RESET} ${ms_title} ${DIM}(created)${RESET}"
      ((ms_created++))
    else
      echo "  ${RED}${ICON_WARN}${RESET} ${ms_title} ${DIM}— failed${RESET}"
      ((ms_failed++))
    fi
  done

  for milestone_plan in "${update_ms_plan[@]}"; do
    ms_id=$(jq -r '.id' <<< "$milestone_plan")
    ms_title=$(jq -r '.new_title' <<< "$milestone_plan")
    ms_desc=$(jq -r '.new_description // ""' <<< "$milestone_plan")
    title_changed=$(jq -r '.title_changed' <<< "$milestone_plan")
    description_changed=$(jq -r '.description_changed' <<< "$milestone_plan")
    cmd=(glab api "projects/$project_id/milestones/$ms_id" -X PUT)
    [[ "$title_changed" == "true" ]] && cmd+=(-f "title=$ms_title")
    [[ "$description_changed" == "true" ]] && cmd+=(-f "description=$ms_desc")
    if require_writes_allowed "update GitLab milestone" && retry_idempotent "${cmd[@]}" &>/dev/null; then
      echo "  ${GREEN}${ICON_OK}${RESET} ${ms_title} ${DIM}(updated)${RESET}"
      ((ms_updated++))
    else
      echo "  ${RED}${ICON_WARN}${RESET} ${ms_title} ${DIM}— update failed${RESET}"
      ((ms_failed++))
    fi
  done

  if [[ ${#new_ms_plan[@]} -gt 0 || ${#update_ms_plan[@]} -gt 0 ]]; then
    if ! ms_json=$(fetch_all_milestones); then
      echo "  ${RED}${ICON_WARN}${RESET} Could not verify milestone changes; dependent issues will not be changed."
      echo ""
      return 1
    fi
  fi

  if [[ ${#missing_project_labels[@]} -gt 0 ]]; then
    echo -n "  ${DIM}Syncing labels...${RESET}"
    for lbl in "${missing_project_labels[@]}"; do
      if require_writes_allowed "create GitLab label" && glab label create -n "$lbl" &>/dev/null; then
        ((labels_created++))
      else
        ((label_failed++))
      fi
    done
    printf "\r                                      \r"
    if ! labels_json=$(fetch_all_labels); then
      echo "  ${RED}${ICON_WARN}${RESET} Could not verify label changes; dependent issues will not be changed."
      echo ""
      return 1
    fi
    existing_labels=$(jq -r '.[].name' <<< "$labels_json")
  fi

  if [[ ${#issue_create_plan[@]} -gt 0 ]]; then
    echo "  ${DIM}Creating issues...${RESET}"
    echo ""
  fi
  for issue_plan in "${issue_create_plan[@]}"; do
    s_key=$(jq -r '.key' <<< "$issue_plan")
    s_summary=$(jq -r '.summary' <<< "$issue_plan")
    new_title=$(jq -r '.title' <<< "$issue_plan")
    new_description=$(jq -r '.description' <<< "$issue_plan")
    labels_csv=$(jq -r '.labels // ""' <<< "$issue_plan")
    new_milestone=$(jq -r '.milestone // ""' <<< "$issue_plan")
    issue_create_status_rank=$(jq -r '.status_rank // 0' <<< "$issue_plan")

    dependency_failed=false
    while IFS= read -r lbl; do
      [[ -z "$lbl" ]] && continue
      if ! grep -qxF "$lbl" <<< "$existing_labels"; then
        dependency_failed=true
        echo "  ${RED}${ICON_WARN}${RESET} [${s_key}] ${s_summary} ${DIM}— required label '${lbl}' is unavailable${RESET}"
        break
      fi
    done < <(csv_to_lines "$labels_csv")
    if $dependency_failed; then
      ((failed++))
      continue
    fi

    cmd=(glab api "projects/$project_id/issues" -X POST -f "title=$new_title" -f "description=$new_description")
    [[ -n "$labels_csv" ]] && cmd+=(-f "labels=$labels_csv")
    if [[ -n "$new_milestone" ]]; then
      ms_id=$(jq -r --arg t "$new_milestone" '[.[] | select(.title == $t)][0] | .id // empty' <<< "$ms_json" 2>/dev/null)
      if [[ -z "$ms_id" ]]; then
        echo "  ${RED}${ICON_WARN}${RESET} [${s_key}] ${s_summary} ${DIM}— required milestone is unavailable${RESET}"
        ((failed++))
        continue
      fi
      cmd+=(-f "milestone_id=$ms_id")
    fi

    output=""
    if require_writes_allowed "create GitLab issue" \
      && output=$("${cmd[@]}" 2>&1) \
      && jq -e 'type == "object" and (.iid | type == "number")' <<< "$output" &>/dev/null; then
      if [[ "$issue_create_status_rank" -eq 3 ]]; then
        created_iid=$(jq -r '.iid' <<< "$output")
        if [[ -n "$created_iid" ]]; then
          if require_writes_allowed "close GitLab issue" \
            && retry_idempotent glab api "projects/$project_id/issues/$created_iid" -X PUT -f "state_event=close" >/dev/null 2>&1; then
            echo "  ${GREEN}${ICON_OK}${RESET} [${s_key}] ${s_summary} ${DIM}(closed)${RESET}"
          else
            echo "  ${RED}${ICON_WARN}${RESET} [${s_key}] ${s_summary} ${DIM}(created, close failed)${RESET}"
            ((failed++))
          fi
        else
          echo "  ${YELLOW}${ICON_WARN}${RESET} [${s_key}] ${s_summary} ${DIM}(created, close skipped)${RESET}"
        fi
      else
        echo "  ${GREEN}${ICON_OK}${RESET} [${s_key}] ${s_summary}"
      fi
      ((created++))
    else
      echo "  ${RED}${ICON_WARN}${RESET} [${s_key}] ${s_summary} ${DIM}— create failed or returned an invalid response${RESET}"
      ((failed++))
    fi
  done

  issue_uptodate=$preview_uptodate
  if [[ ${#issue_update_plan[@]} -gt 0 ]]; then
    echo ""
    echo "  ${DIM}Checking for updates...${RESET}"
    echo ""

    for issue_plan in "${issue_update_plan[@]}"; do
      gl_iid=$(jq -r '.iid' <<< "$issue_plan")
      s_key=$(jq -r '.key' <<< "$issue_plan")
      s_summary=$(jq -r '.summary' <<< "$issue_plan")
      new_title=$(jq -r '.new_title' <<< "$issue_plan")
      new_description=$(jq -r '.new_description' <<< "$issue_plan")
      merged_labels=$(jq -r '.new_labels // ""' <<< "$issue_plan")
      new_milestone=$(jq -r '.new_milestone // ""' <<< "$issue_plan")
      title_changed=$(jq -r '.title_changed' <<< "$issue_plan")
      description_changed=$(jq -r '.description_changed' <<< "$issue_plan")
      labels_changed=$(jq -r '.labels_changed' <<< "$issue_plan")
      milestone_changed=$(jq -r '.milestone_changed' <<< "$issue_plan")
      status_changed=$(jq -r '.status_changed' <<< "$issue_plan")
      status_state_event=$(jq -r '.state_event // ""' <<< "$issue_plan")

      dependency_failed=false
      while IFS= read -r lbl; do
        [[ -z "$lbl" ]] && continue
        if ! grep -qxF "$lbl" <<< "$existing_labels"; then
          dependency_failed=true
          echo "  ${RED}${ICON_WARN}${RESET} [${s_key}] ${s_summary} ${DIM}— required label '${lbl}' is unavailable${RESET}"
          break
        fi
      done < <(csv_to_lines "$merged_labels")
      if $dependency_failed; then
        ((failed++))
        continue
      fi

      cmd=(glab api "projects/$project_id/issues/$gl_iid" -X PUT)
      [[ "$title_changed" == "true" ]] && cmd+=(-f "title=$new_title")
      [[ "$description_changed" == "true" ]] && cmd+=(-f "description=$new_description")
      if [[ "$labels_changed" == "true" ]]; then
        cmd+=(-f "labels=$merged_labels")
      fi
      if [[ "$milestone_changed" == "true" ]]; then
        if [[ -n "$new_milestone" ]]; then
          ms_id=$(jq -r --arg t "$new_milestone" '[.[] | select(.title == $t)][0] | .id // empty' <<< "$ms_json" 2>/dev/null)
          if [[ -z "$ms_id" ]]; then
            echo "  ${RED}${ICON_WARN}${RESET} [${s_key}] ${s_summary} ${DIM}— required milestone is unavailable${RESET}"
            ((failed++))
            continue
          fi
          cmd+=(-f "milestone_id=$ms_id")
        else
          cmd+=(-f "milestone_id=0")
        fi
      fi
      if [[ "$status_changed" == "true" && -n "$status_state_event" ]]; then
        cmd+=(-f "state_event=$status_state_event")
      fi

      if require_writes_allowed "update GitLab issue" && retry_idempotent "${cmd[@]}" &>/dev/null; then
        echo "  ${GREEN}${ICON_OK}${RESET} [${s_key}] ${s_summary} ${DIM}(updated)${RESET}"
        ((issue_updated++))
      else
        echo "  ${RED}${ICON_WARN}${RESET} [${s_key}] ${s_summary} ${DIM}— update failed${RESET}"
        ((failed++))
      fi
    done
  fi

  echo ""
  hr
  echo ""
  echo "  ${BOLD}Sync Complete${RESET}"
  echo ""
  local issue_parts="${GREEN}${created} created${RESET}"
  [[ $issue_updated -gt 0 ]] && issue_parts="${issue_parts}, ${GREEN}${issue_updated} updated${RESET}"
  [[ $issue_uptodate -gt 0 ]] && issue_parts="${issue_parts}, ${DIM}${issue_uptodate} up-to-date${RESET}"
  [[ $failed -gt 0 ]] && issue_parts="${issue_parts}, ${RED}${failed} failed${RESET}"
  echo "  ${ICON_TITLE} Issues:     ${issue_parts}"
  local ms_parts=""
  [[ $ms_created -gt 0 ]] && ms_parts="${ms_created} created"
  [[ $ms_updated -gt 0 ]] && ms_parts="${ms_parts:+$ms_parts, }${ms_updated} updated"
  if [[ -n "$ms_parts" ]]; then
    echo "  ${ICON_MILE} Milestones: ${GREEN}${ms_parts}${RESET}"
  fi
  if [[ $labels_created -gt 0 ]]; then
    echo "  ${ICON_LABEL} Labels:     ${GREEN}${labels_created} created${RESET}"
  fi
  if [[ $label_failed -gt 0 ]]; then
    echo "  ${ICON_LABEL} Labels:     ${RED}${label_failed} failed${RESET}"
  fi
  if [[ $ms_failed -gt 0 ]]; then
    echo "  ${ICON_MILE} Milestones: ${RED}${ms_failed} failed${RESET}"
  fi
  echo ""
  (( failed == 0 && ms_failed == 0 && label_failed == 0 ))
}
