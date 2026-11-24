//go:build windows

package provider

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

func promptPassphraseTTY(filename string) (string, error) {
	conIn, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("opening CONIN$ to prompt for passphrase for %s: %w", filename, err)
	}
	defer func() { _ = conIn.Close() }()

	conOut, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("opening CONOUT$ to prompt for passphrase for %s: %w", filename, err)
	}
	defer func() { _ = conOut.Close() }()

	if _, err := fmt.Fprintf(conOut, "Passphrase for %s: ", filename); err != nil {
		return "", fmt.Errorf("writing passphrase prompt: %w", err)
	}

	pass, err := term.ReadPassword(int(conIn.Fd()))
	_, _ = fmt.Fprintln(conOut)
	if err != nil {
		return "", fmt.Errorf("reading passphrase: %w", err)
	}

	return string(pass), nil
}
