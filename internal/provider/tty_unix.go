//go:build !windows

package provider

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

func promptPassphraseTTY(filename string) (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("opening /dev/tty to prompt for passphrase for %s: %w", filename, err)
	}
	defer func() { _ = tty.Close() }()

	if _, err := fmt.Fprintf(tty, "Passphrase for %s: ", filename); err != nil {
		return "", fmt.Errorf("writing passphrase prompt: %w", err)
	}

	pass, err := term.ReadPassword(int(tty.Fd()))
	_, _ = fmt.Fprintln(tty)
	if err != nil {
		return "", fmt.Errorf("reading passphrase: %w", err)
	}

	return string(pass), nil
}
