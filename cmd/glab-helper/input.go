package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

func readConfirmation(input *bufio.Reader, output io.Writer, prompt string) (bool, error) {
	fmt.Fprint(output, prompt)
	answer, err := input.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer = strings.TrimSpace(answer)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}

func listOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
