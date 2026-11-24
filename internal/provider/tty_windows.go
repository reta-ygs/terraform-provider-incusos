//go:build windows

package provider

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

func promptPassphraseTTY(filename string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("cannot prompt for passphrase for %s: no interactive terminal available", filename)
	}

	if _, err := fmt.Fprintf(os.Stderr, "Passphrase for %s: ", filename); err != nil {
		return "", fmt.Errorf("writing passphrase prompt: %w", err)
	}

	pass, err := term.ReadPassword(fd)
	_, _ = fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("reading passphrase: %w", err)
	}

	return string(pass), nil
}
