package git

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
)

const remoteBranchFormat = "%(refname:strip=3)%00%(symref)"
const branchFormat = "%(refname)%00%(symref)"

type RemoteBranch struct {
	Name string
}

type Branch struct {
	Name   string
	Local  bool
	Remote bool
}

func (client *Client) PruneRemoteBranches(ctx context.Context) error {
	if _, err := client.output(ctx, "fetch", "--prune", "origin", "--quiet"); err != nil {
		return fmt.Errorf("refresh and prune origin branches: %w", err)
	}
	return nil
}

func (client *Client) ListBranches(ctx context.Context) ([]Branch, error) {
	output, err := client.output(
		ctx,
		"for-each-ref",
		"--sort=-committerdate",
		"--format="+branchFormat,
		"refs/heads/",
		"refs/remotes/origin/",
	)
	if err != nil {
		return nil, fmt.Errorf("list current local and origin branches: %w", err)
	}
	branches, err := parseBranches(output)
	if err != nil {
		return nil, fmt.Errorf("decode current local and origin branches: %w", err)
	}
	return branches, nil
}

func (client *Client) CreateBranch(ctx context.Context, name string, base Branch) error {
	if _, err := client.output(ctx, "check-ref-format", "--branch", name); err != nil {
		return fmt.Errorf("validate branch name %q: %w", name, err)
	}
	startPoint := base.Name
	if base.Remote {
		startPoint = "origin/" + base.Name
	} else if !base.Local {
		return fmt.Errorf("base branch %q is unavailable", base.Name)
	}
	if _, err := client.output(ctx, "branch", name, startPoint); err != nil {
		return fmt.Errorf("create branch %q from %q: %w", name, startPoint, err)
	}
	return nil
}

func (client *Client) CheckoutBranch(ctx context.Context, branch Branch) error {
	var args []string
	if branch.Local {
		args = []string{"checkout", branch.Name}
	} else if branch.Remote {
		args = []string{"checkout", "-b", branch.Name, "origin/" + branch.Name}
	} else {
		return fmt.Errorf("branch %q is unavailable", branch.Name)
	}
	if _, err := client.output(ctx, args...); err != nil {
		return fmt.Errorf("check out branch %q: %w", branch.Name, err)
	}
	return nil
}

func (client *Client) ListRemoteBranches(ctx context.Context) ([]RemoteBranch, error) {
	output, err := client.output(
		ctx,
		"for-each-ref",
		"--sort=-committerdate",
		"--format="+remoteBranchFormat,
		"refs/remotes/origin/",
	)
	if err != nil {
		return nil, fmt.Errorf("list current origin branches: %w", err)
	}

	branches, err := parseRemoteBranches(output)
	if err != nil {
		return nil, fmt.Errorf("decode current origin branches: %w", err)
	}
	return branches, nil
}

func parseRemoteBranches(data []byte) ([]RemoteBranch, error) {
	branches := make([]RemoteBranch, 0)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		parts := bytes.SplitN(scanner.Bytes(), []byte{0}, 2)
		if len(parts) != 2 || len(parts[0]) == 0 {
			return nil, fmt.Errorf("line %d: expected branch name and symbolic-ref marker", lineNumber)
		}
		if len(parts[1]) > 0 {
			continue
		}
		branches = append(branches, RemoteBranch{Name: string(parts[0])})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan branch list: %w", err)
	}

	return branches, nil
}

func parseBranches(data []byte) ([]Branch, error) {
	branches := make([]Branch, 0)
	indexes := make(map[string]int)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		parts := bytes.SplitN(scanner.Bytes(), []byte{0}, 2)
		if len(parts) != 2 || len(parts[0]) == 0 {
			return nil, fmt.Errorf("line %d: expected ref name and symbolic-ref marker", lineNumber)
		}
		if len(parts[1]) > 0 {
			continue
		}
		refName := string(parts[0])
		branch := Branch{}
		switch {
		case strings.HasPrefix(refName, "refs/heads/"):
			branch = Branch{Name: strings.TrimPrefix(refName, "refs/heads/"), Local: true}
		case strings.HasPrefix(refName, "refs/remotes/origin/"):
			branch = Branch{Name: strings.TrimPrefix(refName, "refs/remotes/origin/"), Remote: true}
		default:
			return nil, fmt.Errorf("line %d: unexpected ref %q", lineNumber, refName)
		}
		if branch.Name == "" {
			return nil, fmt.Errorf("line %d: empty branch name", lineNumber)
		}
		if index, exists := indexes[branch.Name]; exists {
			branches[index].Local = branches[index].Local || branch.Local
			branches[index].Remote = branches[index].Remote || branch.Remote
			continue
		}
		indexes[branch.Name] = len(branches)
		branches = append(branches, branch)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan branch list: %w", err)
	}
	return branches, nil
}
