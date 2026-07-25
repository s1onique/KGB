// cmd/extract-image-metadata writes canonical canary-image-build/v2 metadata.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/docker/docker/client"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/buildmetadata"
)

var errUsage = errors.New("usage error")

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		if errors.Is(err, errUsage) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("extract-image-metadata", flag.ContinueOnError)
	sourceCommit := fs.String("source-commit", "", "source commit OID")
	sourceTree := fs.String("source-tree", "", "repository tree OID")
	canaryTree := fs.String("canary-source-tree", "", "canary source tree OID")
	binary := fs.String("canary-binary", "", "built canary binary")
	buildKit := fs.String("buildkit-metadata", "", "buildx metadata JSON")
	output := fs.String("output", "", "canonical metadata output path")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 1 {
		return errUsage
	}
	if *sourceCommit == "" || *sourceTree == "" || *canaryTree == "" || *binary == "" || *output == "" {
		return errUsage
	}
	imageRef := fs.Arg(0)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("create Docker client: %w", err)
	}
	defer cli.Close()
	inspect, _, err := cli.ImageInspectWithRaw(context.Background(), imageRef)
	if err != nil {
		return fmt.Errorf("inspect image: %w", err)
	}
	hash, revision, modified, err := buildmetadata.BinaryAuthority(*binary)
	if err != nil {
		return err
	}
	manifestDigest, indexDigest, err := buildmetadata.BuildKitDigests(*buildKit)
	if err != nil {
		return fmt.Errorf("parse BuildKit metadata: %w", err)
	}
	metadata := buildmetadata.CanaryImageBuild{
		SchemaVersion: buildmetadata.SchemaVersion, SourceCommit: *sourceCommit, SourceTree: *sourceTree,
		CanarySourceTree: *canaryTree, RequestedReference: imageRef, EngineImageID: inspect.ID,
		RepoDigests: inspect.RepoDigests, BuildKitManifestDigest: manifestDigest, BuildKitIndexDigest: indexDigest,
		CanaryBinarySHA256: hash, CanaryVCSRevision: revision, CanaryVCSModified: modified,
	}
	if metadata.RepoDigests == nil {
		metadata.RepoDigests = []string{}
	}
	if err := buildmetadata.WriteAtomic(*output, metadata); err != nil {
		return fmt.Errorf("write canonical metadata: %w", err)
	}
	fmt.Printf("engine_image_id=%s\n", metadata.EngineImageID)
	fmt.Printf("buildkit_manifest_digest=%s\n", metadata.BuildKitManifestDigest)
	fmt.Printf("buildkit_index_digest=%s\n", metadata.BuildKitIndexDigest)
	return nil
}
