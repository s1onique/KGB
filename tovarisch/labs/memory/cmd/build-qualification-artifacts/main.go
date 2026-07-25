// build-qualification-artifacts — CORRECTION49 S49 placeholder
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/qualification"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("build-qualification-artifacts", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	sourceRoot := fs.String("source-root", "", "detached source checkout")
	artifactRoot := fs.String("artifact-root", "", "external artifact directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *sourceRoot == "" || *artifactRoot == "" || fs.NArg() != 0 {
		return fmt.Errorf("usage: build-qualification-artifacts --source-root <repo> --artifact-root <external>")
	}
	path, err := qualification.BuildQualificationArtifacts(qualification.BuildOptions{SourceRoot: *sourceRoot, ArtifactRoot: *artifactRoot})
	if err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", path)
	return nil
}
