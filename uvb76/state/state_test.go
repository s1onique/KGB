package state

import (
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.GetSnapshotCount() != 0 {
		t.Errorf("Expected empty manager, got count %d", m.GetSnapshotCount())
	}
}

func TestUpdateAndGetSnapshot(t *testing.T) {
	m := NewManager()

	snap := &TargetSnapshot{
		TargetID:  "test-1",
		ScrapedAt: time.Now().UTC(),
		Reachable: true,
		Status:    "ok",
	}

	m.UpdateSnapshot("test-1", snap)

	retrieved := m.GetSnapshot("test-1")
	if retrieved == nil {
		t.Fatal("GetSnapshot returned nil")
	}
	if retrieved.TargetID != "test-1" {
		t.Errorf("Expected target ID 'test-1', got '%s'", retrieved.TargetID)
	}
	if retrieved.Reachable != true {
		t.Error("Expected Reachable to be true")
	}
}

func TestGetAllSnapshots(t *testing.T) {
	m := NewManager()

	m.UpdateSnapshot("test-1", &TargetSnapshot{TargetID: "test-1", Reachable: true})
	m.UpdateSnapshot("test-2", &TargetSnapshot{TargetID: "test-2", Reachable: false})

	snaps := m.GetAllSnapshots()
	if len(snaps) != 2 {
		t.Errorf("Expected 2 snapshots, got %d", len(snaps))
	}
}

func TestGetNonexistentSnapshot(t *testing.T) {
	m := NewManager()
	snap := m.GetSnapshot("nonexistent")
	if snap != nil {
		t.Error("Expected nil for nonexistent snapshot")
	}
}

func TestBoundedState(t *testing.T) {
	m := NewManager()

	// Add 10 snapshots (should be bounded by config)
	for i := 0; i < 10; i++ {
		m.UpdateSnapshot(string(rune('0'+i)), &TargetSnapshot{
			TargetID:  string(rune('0' + i)),
			Reachable: true,
		})
	}

	if m.GetSnapshotCount() != 10 {
		t.Errorf("Expected 10 snapshots, got %d", m.GetSnapshotCount())
	}

	// Update one existing - should replace, not grow
	m.UpdateSnapshot("0", &TargetSnapshot{TargetID: "0", Reachable: false})

	if m.GetSnapshotCount() != 10 {
		t.Errorf("Expected still 10 snapshots after update, got %d", m.GetSnapshotCount())
	}
}
