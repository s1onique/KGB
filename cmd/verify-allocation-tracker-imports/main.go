// verify-allocation-tracker-imports enforces the runtime package boundary.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/s1onique/KGB/internal/tooling/allocationtrackerimports"
)

func main() {
	selfTest := flag.Bool("self-test", false, "run the hermetic mutation suite")
	flag.Parse()

	if *selfTest {
		if err := allocationtrackerimports.SelfTest(); err != nil {
			fmt.Fprintf(os.Stderr, "[gate] FAIL: self-test: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("[gate] PASS: self-test (15 fixture classes + 2 fail-closed mutations; compilable decoy zig test passed)")
		return
	}

	repoRoot, err := allocationtrackerimports.FindRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[gate] FAIL: gate failed closed: %v\n", err)
		os.Exit(2)
	}
	findings, err := allocationtrackerimports.VerifyRepo(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[gate] FAIL: gate failed closed: %v\n", err)
		os.Exit(2)
	}
	if len(findings) != 0 {
		fmt.Fprintln(os.Stderr, "[gate] FAIL: allocation_tracker import-boundary violation detected")
		for _, finding := range findings {
			fmt.Fprintln(os.Stderr, finding.String())
		}
		os.Exit(1)
	}
	fmt.Println("[gate] PASS: external @import syntax is literal; no private allocation_tracker sibling imported")
}
