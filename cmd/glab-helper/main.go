package main

import (
	"fmt"
	"io"
	"os"
)

const version = "0.1.0"

const help = `
  glab-helper — Interactive GitLab workflow helper

  Usage: glab-helper [--dev | --maintenance] [--dry-run] [--version]

  Options:
    --dev, -d   Show developer actions and skip the Jira target-project check
    --maintenance  Show destructive project maintenance actions
    --dry-run   Read-only mode; preview Jira syncs without any writes
    --version   Show version and exit

  Run from any cloned GitLab repo. Requires: glab, fzf, jq
  Authenticate first: glab auth login

`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "Interactive workflow not migrated yet; use the Zsh glab-helper.")
		return 2
	}

	switch args[0] {
	case "--help", "-h":
		fmt.Fprint(stdout, help)
		return 0
	case "--version":
		fmt.Fprintf(stdout, "glab-helper %s\n", version)
		return 0
	default:
		fmt.Fprintf(stderr, "Unknown argument: %s\n", args[0])
		fmt.Fprintln(stderr, "Usage: glab-helper [--help | --version]")
		return 2
	}
}
