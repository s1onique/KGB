// genhash generates a sha256:<salt>:<hex> password hash for UVB-76 config.
//
// Usage:
//
//	cd uvb76
//	go run ./tools/genhash
//	# Enter password when prompted
//
// Output format: sha256:<32-char-hex-salt>:<64-char-hex-hash>
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/s1onique/KGB/uvb76/config"
)

func main() {
	// Read password from stdin (supports piped input or interactive)
	var password string

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Input is piped
		reader := bufio.NewReader(os.Stdin)
		password, _ = reader.ReadString('\n')
		password = strings.TrimSpace(password)
	} else {
		// Interactive mode - read from terminal
		fmt.Print("Enter password: ")
		reader := bufio.NewReader(os.Stdin)
		password, _ = reader.ReadString('\n')
		password = strings.TrimSpace(password)
	}

	if password == "" {
		fmt.Fprintln(os.Stderr, "Error: password cannot be empty")
		os.Exit(1)
	}

	salt, err := config.GenerateSalt()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating salt: %v\n", err)
		os.Exit(1)
	}

	hash, err := config.HashPassword(password, salt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error hashing password: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(hash)
}
