// image_identity.go — Exact Image Identity Resolution and Validation
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03-P0-10-EXACT-IMAGE-ID-NO-PULL-SMOKE01
//
// Provides authoritative image identity resolution with exact ID validation.
// Ensures the matrix uses immutable local image identities with no pull behavior.
//
// Key properties:
// - Canonical image ID validation (sha256:<64 lowercase hex>)
// - One-time resolution with freeze
// - No pull policy in execution
// - Post-create inspection verification
// - Fail-closed on identity mismatch

package dockerlab

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// canonicalImageIDPattern matches the only accepted image ID form.
var canonicalImageIDPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ErrEmptyImageID is returned when the image ID is empty.
var ErrEmptyImageID = fmt.Errorf("image ID is empty")

// ErrInvalidImageIDFormat is returned when the image ID doesn't match canonical form.
type ErrInvalidImageIDFormat struct {
	Input   string
	Reason  string
}

func (e *ErrInvalidImageIDFormat) Error() string {
	return fmt.Sprintf("invalid image ID format %q: %s", e.Input, e.Reason)
}

// ErrImageNotFound is returned when the image cannot be resolved.
type ErrImageNotFound struct {
	Reference string
}

func (e *ErrImageNotFound) Error() string {
	return fmt.Sprintf("image not found: %s", e.Reference)
}

// ErrAmbiguousOutput is returned when image inspection produces ambiguous output.
type ErrAmbiguousOutput struct {
	Reference string
	Output    string
}

func (e *ErrAmbiguousOutput) Error() string {
	return fmt.Sprintf("ambiguous image inspect output for %q: %s", e.Reference, e.Output)
}

// ResolvedImageIdentity is the canonical immutable image identity structure.
// It captures the resolved identity once and provides validation utilities.
type ResolvedImageIdentity struct {
	// DescriptiveReference is the human-facing reference (tag or full reference).
	// Used for diagnostics only - never for execution authority.
	DescriptiveReference string

	// ImageID is the mandatory full canonical local image ID.
	// Must match ^sha256:[0-9a-f]{64}$
	ImageID string

	// RepositoryDigest is the optional immutable registry identity.
	// May be empty for local-only images.
	RepositoryDigest string
}

// ValidateExactImageID validates that the given string is a canonical image ID.
// Returns nil if valid, ErrEmptyImageID if empty, or ErrInvalidImageIDFormat otherwise.
func ValidateExactImageID(imageID string) error {
	if imageID == "" {
		return ErrEmptyImageID
	}

	// Check for tag patterns (not allowed in image ID field)
	if strings.Contains(imageID, ":") {
		// Could be "tag" or "sha256:..."
		if !strings.HasPrefix(imageID, "sha256:") {
			return &ErrInvalidImageIDFormat{
				Input:  imageID,
				Reason: "must use sha256: prefix for image ID, not tag",
			}
		}
	}

	// Check for uppercase (not allowed)
	if imageID != strings.ToLower(imageID) {
		return &ErrInvalidImageIDFormat{
			Input:  imageID,
			Reason: "must be lowercase",
		}
	}

	// Check for canonical form
	if !canonicalImageIDPattern.MatchString(imageID) {
		// Provide specific reason
		if strings.HasPrefix(imageID, "sha256:") {
			suffix := strings.TrimPrefix(imageID, "sha256:")
			if len(suffix) != 64 {
				return &ErrInvalidImageIDFormat{
					Input:  imageID,
					Reason: fmt.Sprintf("sha256 suffix must be exactly 64 hex chars, got %d", len(suffix)),
				}
			}
			if !isHex(suffix) {
				return &ErrInvalidImageIDFormat{
					Input:  imageID,
					Reason: "sha256 suffix must be lowercase hexadecimal",
				}
			}
		}
		return &ErrInvalidImageIDFormat{
			Input:  imageID,
			Reason: "must match sha256:<64 lowercase hex>",
		}
	}

	return nil
}

// isHex returns true if all characters are lowercase hex.
func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// ResolveImageIdentity resolves a descriptive image reference to an immutable local image ID.
// Resolution happens exactly once - the returned identity is frozen for the lifetime of the call.
func (c *Client) ResolveImageIdentity(ctx context.Context, reference string) (*ResolvedImageIdentity, error) {
	// Step 1: Get full image ID via inspect
	inspect, _, err := c.Client.ImageInspectWithRaw(ctx, reference)
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "No such image") ||
			strings.Contains(err.Error(), "not found") {
			return nil, &ErrImageNotFound{Reference: reference}
		}
		return nil, fmt.Errorf("inspect image %s: %w", reference, err)
	}

	// Step 2: Validate the returned ID is canonical
	imageID := inspect.ID
	if err := ValidateExactImageID(imageID); err != nil {
		return nil, fmt.Errorf("resolved image ID validation: %w", err)
	}

	// Step 3: Get repository digests if available
	var repoDigest string
	digests := inspect.RepoDigests
	if len(digests) > 0 {
		// Use the first digest as the canonical repository digest
		repoDigest = digests[0]
	}

	return &ResolvedImageIdentity{
		DescriptiveReference: reference,
		ImageID:             imageID,
		RepositoryDigest:     repoDigest,
	}, nil
}

// ImageInspectFullID returns the full canonical image ID for a reference.
// This is a convenience method for direct image ID resolution.
func (c *Client) ImageInspectFullID(ctx context.Context, reference string) (string, error) {
	inspect, _, err := c.Client.ImageInspectWithRaw(ctx, reference)
	if err != nil {
		if strings.Contains(err.Error(), "No such image") ||
			strings.Contains(err.Error(), "not found") {
			return "", &ErrImageNotFound{Reference: reference}
		}
		return "", fmt.Errorf("inspect image %s: %w", reference, err)
	}

	// Validate and return
	if err := ValidateExactImageID(inspect.ID); err != nil {
		return "", fmt.Errorf("image ID validation: %w", err)
	}

	return inspect.ID, nil
}

// InspectContainerActualImageID returns the actual image ID used by a container.
// This is used to verify the container was created from the expected image.
func (c *Client) InspectContainerActualImageID(ctx context.Context, containerID string) (string, error) {
	inspect, err := c.Client.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("inspect container %s: %w", containerID, err)
	}

	// Validate the returned image ID
	if err := ValidateExactImageID(inspect.Image); err != nil {
		return "", fmt.Errorf("container image ID validation: %w", err)
	}

	return inspect.Image, nil
}

// VerifyContainerImageBinding verifies that a container was created from the expected image.
// Returns nil on match, error on mismatch.
func (c *Client) VerifyContainerImageBinding(ctx context.Context, containerID, expectedImageID string) error {
	actualImageID, err := c.InspectContainerActualImageID(ctx, containerID)
	if err != nil {
		return fmt.Errorf("inspect container image: %w", err)
	}

	if actualImageID != expectedImageID {
		return fmt.Errorf("image ID mismatch: expected %s, got %s", expectedImageID, actualImageID)
	}

	return nil
}
