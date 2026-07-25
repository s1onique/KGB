package main

import (
	"fmt"
	"os"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: live-qualification-verifier <bundle.json>")
		os.Exit(2)
	}
	if err := evidence.VerifyLiveQualificationBundle(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "REJECT: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("PASS: live qualification bundle verified")
}
