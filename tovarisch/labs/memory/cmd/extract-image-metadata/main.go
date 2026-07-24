// cmd/extract-image-metadata — Extract Docker image metadata as JSON
//
// Extracts the canonical image ID and repository digests from a Docker
// image using the Engine API. Used by build_tovarisch_canary_image.sh
// to replace Python JSON parsing with a typed Go binary.
//
// Usage:
//
//	extract-image-metadata <image-ref>
//
// Output (JSON):
//
//	{
//	  "image_id": "sha256:abc123...",
//	  "repo_digests": ["registry/repo@sha256:def456..."]
//	}
//
// Exit codes:
//	0 - success
//	1 - image not found or other Docker error
//	2 - usage error

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

func main() {
	if err := run(os.Args); err != nil {
		if errors.Is(err, errUsage) {
			flag.Usage()
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

var errUsage = errors.New("usage error")

// ImageMetadata holds the extracted image information.
type ImageMetadata struct {
	ImageID     string   `json:"image_id"`
	RepoDigests []string `json:"repo_digests"`
}

func run(args []string) error {
	fs := flag.NewFlagSet("extract-image-metadata", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <image-ref>\n\nFlags:\n", fs.Name())
		fs.PrintDefaults()
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errUsage
	}

	if fs.NArg() != 1 {
		return errUsage
	}

	imageRef := fs.Arg(0)
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	if _, err := cli.Ping(ctx); err != nil {
		return fmt.Errorf("docker ping: %w", err)
	}

	// Use ImageInspectWithRaw to get the canonical image ID
	inspect, raw, err := cli.ImageInspectWithRaw(ctx, imageRef)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return fmt.Errorf("image not found: %s", imageRef)
		}
		return fmt.Errorf("inspect image: %w", err)
	}

	// Extract RepoDigests from raw JSON if available
	var rawInspect types.ImageInspect
	if err := json.Unmarshal(raw, &rawInspect); err == nil {
		meta := ImageMetadata{
			ImageID:     inspect.ID,
			RepoDigests: rawInspect.RepoDigests,
		}
		return writeJSON(meta)
	}

	// Fallback: use RepoDigests from ImageInspect if unmarshaling raw failed
	meta := ImageMetadata{
		ImageID:     inspect.ID,
		RepoDigests: inspect.RepoDigests,
	}
	return writeJSON(meta)
}

func writeJSON(meta ImageMetadata) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "")
	if err := enc.Encode(meta); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}
