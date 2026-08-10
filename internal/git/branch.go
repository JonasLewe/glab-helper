package git

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
)

const remoteBranchFormat = "%(refname:strip=3)%00%(symref)"

type RemoteBranch struct {
	Name string
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
