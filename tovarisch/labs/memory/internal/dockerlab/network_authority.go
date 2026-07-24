// network_authority.go — Network ID authority helpers for the
// qualified execution path.
//
// CORRECTION16: the qualified execution path must create an
// isolated lab network through the DockerRuntime, capture the
// returned ID, and inspect the network to confirm the canonical
// identity. The container create request then uses the canonical
// network ID via create-time networking.

package dockerlab

import (
	"context"
	"errors"
	"fmt"

	"github.com/docker/docker/api/types"
)

// ErrNetworkIdentityEmpty is returned when the network ID is empty.
var ErrNetworkIdentityEmpty = errors.New("network ID is empty")

// ErrNetworkIdentityMalformed is returned when the network ID is not
// 64 lowercase hex characters (the canonical Docker network ID form).
var ErrNetworkIdentityMalformed = errors.New("network ID is not 64 lowercase hex characters")

// ValidateCanonicalNetworkIDLenient validates that the given string is
// a canonical Docker network ID (64 lowercase hex characters).
func ValidateCanonicalNetworkIDLenient(id string) error {
	if id == "" {
		return ErrNetworkIdentityEmpty
	}
	if len(id) != 64 {
		return fmt.Errorf("%w: got %d chars", ErrNetworkIdentityMalformed, len(id))
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return ErrNetworkIdentityMalformed
		}
	}
	return nil
}

// CreateAndInspectNetwork creates an isolated lab network through the
// runtime and inspects it to confirm the canonical network ID. The
// returned string is the canonical network ID (the create-response and
// the inspect-response must agree).
//
// The create and inspect calls are routed through the same runtime
// that the production container-create call uses, so the same
// authority governs every step. A network whose create-response ID
// disagrees with its inspect-response ID is rejected with a fail-closed
// error; the network is removed before returning.
func (q *QualifiedClient) CreateAndInspectNetwork(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", errors.New("network name is empty")
	}
	if q.runtime == nil {
		return "", errors.New("runtime is nil")
	}
	createResp, err := q.runtime.NetworkCreate(ctx, name, types.NetworkCreate{
		Driver:     "bridge",
		Attachable: true,
		Labels: map[string]string{
			"kgb.dev/lab":          "tovarisch-memory",
			"kgb.dev/lab.run-name": name,
		},
	})
	if err != nil {
		return "", fmt.Errorf("create network %q: %w", name, err)
	}
	if err := ValidateCanonicalNetworkIDLenient(createResp.ID); err != nil {
		_ = q.runtime.NetworkRemove(ctx, createResp.ID)
		return "", fmt.Errorf("created network %q has invalid ID: %w", name, err)
	}
	insp, err := q.runtime.NetworkInspect(ctx, createResp.ID, types.NetworkInspectOptions{})
	if err != nil {
		_ = q.runtime.NetworkRemove(ctx, createResp.ID)
		return "", fmt.Errorf("inspect network %q: %w", createResp.ID, err)
	}
	if insp.ID == "" {
		_ = q.runtime.NetworkRemove(ctx, createResp.ID)
		return "", fmt.Errorf("inspected network %q has empty ID", createResp.ID)
	}
	if insp.ID != createResp.ID {
		_ = q.runtime.NetworkRemove(ctx, createResp.ID)
		return "", fmt.Errorf("network create/inspect mismatch: create=%q inspect=%q",
			createResp.ID, insp.ID)
	}
	return insp.ID, nil
}
