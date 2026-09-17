// Command hashpassword bcrypt-hashes a password for MASTER_PASSWORD_HASH, so the plaintext never
// has to be typed into an env file or deploy config - only the hash does.
//
// Usage:
//
//	go run ./tools/hashpassword                  # prompts, input hidden
//	go run ./tools/hashpassword 'my-password'     # from an argument (visible in shell history)
package main

import (
	"bufio"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

func main() {
	password, err := readPassword()
	if err != nil {
		fmt.Fprintln(os.Stderr, "hashpassword:", err)
		os.Exit(1)
	}
	if len(password) == 0 {
		fmt.Fprintln(os.Stderr, "hashpassword: password must not be empty")
		os.Exit(1)
	}
	hash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hashpassword:", err)
		os.Exit(1)
	}
	fmt.Println(string(hash))
}

func readPassword() ([]byte, error) {
	if len(os.Args) > 1 {
		return []byte(os.Args[1]), nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Password: ")
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return password, err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return nil, err
	}
	return []byte(trimNewline(line)), nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
