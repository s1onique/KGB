// network_authority_test.go — Hermetic tests for the network
// authority helpers.

package dockerlab

import (
	"context"
	"errors"
	"github.com/docker/docker/api/types"
	"strings"
	"testing"
)

func TestCreateAndInspectNetwork_Success(t *testing.T) {
	fake := newRecordingDockerRuntime()
	qc := NewQualifiedClient(fake)
	ctx := context.Background()

	netID, err := qc.CreateAndInspectNetwork(ctx, "kgb-lab-test-net")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if err := ValidateCanonicalNetworkIDLenient(netID); err != nil {
		t.Errorf("expected canonical network ID, got %q: %v", netID, err)
	}
	if fake.networkCreateCalls != 1 {
		t.Errorf("expected 1 network create call, got %d", fake.networkCreateCalls)
	}
	if fake.networkInspectCalls != 1 {
		t.Errorf("expected 1 network inspect call, got %d", fake.networkInspectCalls)
	}
	// The fake must not invoke any pull during network creation.
	if fake.imagePullCalls != 0 {
		t.Errorf("expected 0 pull calls, got %d", fake.imagePullCalls)
	}
}

func TestCreateAndInspectNetwork_EmptyNameFails(t *testing.T) {
	fake := newRecordingDockerRuntime()
	qc := NewQualifiedClient(fake)
	ctx := context.Background()

	_, err := qc.CreateAndInspectNetwork(ctx, "")
	if err == nil {
		t.Fatal("expected empty name to fail")
	}
	if fake.networkCreateCalls != 0 {
		t.Errorf("expected 0 network create calls, got %d", fake.networkCreateCalls)
	}
}

func TestCreateAndInspectNetwork_CreateErrorPropagates(t *testing.T) {
	fake := newRecordingDockerRuntime()
	fake.networkCreateErr = errors.New("network daemon down")
	qc := NewQualifiedClient(fake)
	ctx := context.Background()

	_, err := qc.CreateAndInspectNetwork(ctx, "kgb-lab-test-net")
	if err == nil {
		t.Fatal("expected create error to propagate")
	}
	if !strings.Contains(err.Error(), "network daemon down") {
		t.Errorf("expected wrapped create error, got: %v", err)
	}
	if fake.networkCreateCalls != 1 {
		t.Errorf("expected 1 network create call, got %d", fake.networkCreateCalls)
	}
	if fake.networkInspectCalls != 0 {
		t.Errorf("expected 0 network inspect calls, got %d", fake.networkInspectCalls)
	}
}

func TestCreateAndInspectNetwork_InspectErrorTriggersCleanup(t *testing.T) {
	fake := newRecordingDockerRuntime()
	fake.networkInspectErr = errors.New("inspect failed")
	qc := NewQualifiedClient(fake)
	ctx := context.Background()

	_, err := qc.CreateAndInspectNetwork(ctx, "kgb-lab-test-net")
	if err == nil {
		t.Fatal("expected inspect error to propagate")
	}
	// The created network must be removed on inspect failure.
	if fake.networkRemoveCalls != 1 {
		t.Errorf("expected 1 network remove call (cleanup), got %d", fake.networkRemoveCalls)
	}
}

func TestCreateAndInspectNetwork_MismatchTriggersCleanup(t *testing.T) {
	fake := newRecordingDockerRuntime()
	qc := NewQualifiedClient(fake)
	ctx := context.Background()

	// Force NetworkInspect to return a different ID than the create response.
	wrongID := strings.Repeat("d", 64)
	fake.networkInspectOverride = func(networkID string) (types.NetworkResource, error) {
		return types.NetworkResource{ID: wrongID, Name: "kgb-lab-test-net"}, nil
	}

	_, err := qc.CreateAndInspectNetwork(ctx, "kgb-lab-test-net")
	if err == nil {
		t.Fatal("expected create/inspect mismatch to fail")
	}
	if !strings.Contains(err.Error(), "create/inspect mismatch") {
		t.Errorf("expected mismatch error, got: %v", err)
	}
	if fake.networkRemoveCalls != 1 {
		t.Errorf("expected 1 network remove call (cleanup), got %d", fake.networkRemoveCalls)
	}
}

func TestValidateCanonicalNetworkIDLenient(t *testing.T) {
	cases := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid", strings.Repeat("a", 64), false},
		{"empty", "", true},
		{"short", strings.Repeat("a", 32), true},
		{"long", strings.Repeat("a", 65), true},
		{"uppercase", strings.Repeat("A", 64), true},
		{"non-hex", strings.Repeat("z", 64), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCanonicalNetworkIDLenient(tc.id)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateCanonicalNetworkIDLenient(%q) err=%v, wantErr=%v", tc.id, err, tc.wantErr)
			}
		})
	}
}
