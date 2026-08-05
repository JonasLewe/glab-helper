package main

import (
	"fmt"
	"io"
	"os"
)

const version = "0.1.0"
const usage = "Usage: glab-helper [--dev | --maintenance] [--dry-run] [--version]"

const help = `
  glab-helper — Interactive GitLab workflow helper

  ` + usage + `

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
	var dev, maintenance, showHelp, showVersion bool

	for _, arg := range args {
		switch arg {
		case "--dev", "-d":
			dev = true
		case "--dry-run":
			continue
		case "--maintenance":
			maintenance = true
		case "--help", "-h":
			showHelp = true
		case "--version":
			showVersion = true
		default:
			fmt.Fprintf(stderr, "Unknown argument: %s\n", arg)
			fmt.Fprintln(stderr, usage)
			return 2
		}
	}

	if dev && maintenance {
		fmt.Fprintln(stderr, "--dev and --maintenance cannot be combined.")
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "glab-helper %s\n", version)
		return 0
	}
	if showHelp {
		fmt.Fprint(stdout, help)
		return 0
	}

	fmt.Fprintln(stderr, "Interactive workflow not migrated yet; use the Zsh glab-helper.")
	return 2
}
