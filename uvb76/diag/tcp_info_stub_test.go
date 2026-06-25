// Package diag implements diagnostic capture for UVB-76.
//go:build !linux
// +build !linux

package diag

import (
	"context"
	"net"
	"testing"
)

func TestGetTcpInfo_NonLinux(t *testing.T) {
	ctx := context.Background()

	// Create a dummy connection (won't actually connect)
	conn, err := net.Dial("tcp", "localhost:80")
	if err != nil {
		t.Skip("cannot create connection")
	}
	defer conn.Close()

	result := GetTcpInfo(ctx, conn)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Available {
		t.Error("expected Available=false on non-Linux platform")
	}
	if result.Error == nil {
		t.Error("expected error on non-Linux platform")
	}
	if result.Error.Kind != "unsupported" {
		t.Errorf("expected error kind 'unsupported', got '%s'", result.Error.Kind)
	}
}

func TestGetTcpInfoFromSyntheticDial_NonLinux(t *testing.T) {
	ctx := context.Background()

	result, conn, err := GetTcpInfoFromSyntheticDial(ctx, "localhost:80")

	if conn != nil {
		t.Error("expected nil connection on non-Linux")
		conn.Close()
	}

	if err == nil {
		t.Error("expected error on non-Linux platform")
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Available {
		t.Error("expected Available=false on non-Linux platform")
	}
	if result.Error == nil {
		t.Error("expected error on non-Linux platform")
	}
	if result.IsSynthetic != true {
		t.Error("expected IsSynthetic=true for synthetic dial attempt")
	}
}

func TestTcpInfoError(t *testing.T) {
	err := &TcpInfoError{
		Kind:    "test_kind",
		Message: "test message",
	}

	if err.Error() != "test message" {
		t.Errorf("expected 'test message', got '%s'", err.Error())
	}
}
