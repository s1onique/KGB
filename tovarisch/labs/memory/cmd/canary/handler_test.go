// handler_test.go — Tests for canary HTTP handlers
//
// Tests for /operate endpoint with all required scenarios.

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleOperatePOSTCount1(t *testing.T) {
	c, _ := NewCanary(ModeGrowing, 1024*1024)
	c.SetReady()

	req := httptest.NewRequest(http.MethodPost, "/operate?count=1", nil)
	w := httptest.NewRecorder()
	c.handleOperate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["completed"] != float64(1) {
		t.Errorf("completed: got %v, want 1", resp["completed"])
	}
}

func TestHandleOperateCount0(t *testing.T) {
	c, _ := NewCanary(ModeGrowing, 1024*1024)
	c.SetReady()

	// count=0 should succeed with 0 completed
	req := httptest.NewRequest(http.MethodPost, "/operate?count=0", nil)
	w := httptest.NewRecorder()
	c.handleOperate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["completed"] != float64(0) {
		t.Errorf("completed: got %v, want 0", resp["completed"])
	}
}

func TestHandleOperateNegativeCount(t *testing.T) {
	c, _ := NewCanary(ModeGrowing, 1024*1024)
	c.SetReady()

	req := httptest.NewRequest(http.MethodPost, "/operate?count=-1", nil)
	w := httptest.NewRecorder()
	c.handleOperate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleOperateMalformedCount(t *testing.T) {
	c, _ := NewCanary(ModeGrowing, 1024*1024)
	c.SetReady()

	req := httptest.NewRequest(http.MethodPost, "/operate?count=abc", nil)
	w := httptest.NewRecorder()
	c.handleOperate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleOperateCountAboveLimit(t *testing.T) {
	c, _ := NewCanary(ModeGrowing, 1024*1024)
	c.SetReady()

	// MaxOperations = 1000, so 2000 should fail
	req := httptest.NewRequest(http.MethodPost, "/operate?count=2000", nil)
	w := httptest.NewRecorder()
	c.handleOperate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleOperateGETNotAllowed(t *testing.T) {
	c, _ := NewCanary(ModeGrowing, 1024*1024)
	c.SetReady()

	req := httptest.NewRequest(http.MethodGet, "/operate?count=1", nil)
	w := httptest.NewRecorder()
	c.handleOperate(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleOperatePUTNotAllowed(t *testing.T) {
	c, _ := NewCanary(ModeGrowing, 1024*1024)
	c.SetReady()

	req := httptest.NewRequest(http.MethodPut, "/operate?count=1", nil)
	w := httptest.NewRecorder()
	c.handleOperate(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestGrowingOperationChangesRetainedBytes(t *testing.T) {
	c, _ := NewCanary(ModeGrowing, 1024*1024) // 1MB blocks
	c.SetReady()

	// Perform 5 growing operations
	req := httptest.NewRequest(http.MethodPost, "/operate?count=5", nil)
	w := httptest.NewRecorder()
	c.handleOperate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// 5 operations × 1MB = 5MB = 5242880 bytes
	expectedBytes := int64(5 * 1024 * 1024)
	state := c.State()
	if state.AllocatedBytes != expectedBytes {
		t.Errorf("AllocatedBytes: got %d, want %d", state.AllocatedBytes, expectedBytes)
	}

	if state.RetainedBlocks != 5 {
		t.Errorf("RetainedBlocks: got %d, want 5", state.RetainedBlocks)
	}
}

func TestBoundedOperationLeavesCapacityUnchanged(t *testing.T) {
	c, _ := NewCanary(ModeBounded, 1024*1024)
	c.SetReady()

	// Perform bounded operations
	req := httptest.NewRequest(http.MethodPost, "/operate?count=10", nil)
	w := httptest.NewRecorder()
	c.handleOperate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	state := c.State()
	// Buffer capacity should remain the same
	if state.BufferCapacity != 1024*1024 {
		t.Errorf("BufferCapacity: got %d, want %d", state.BufferCapacity, 1024*1024)
	}

	// Operation count should equal completed
	if state.OperationCount != 10 {
		t.Errorf("OperationCount: got %d, want 10", state.OperationCount)
	}
}

func TestDescriptorOperationChangesFDCount(t *testing.T) {
	c, _ := NewCanary(ModeDescriptor, 1024*1024)
	c.SetReady()

	// Perform 3 descriptor operations (each creates 2 FDs)
	req := httptest.NewRequest(http.MethodPost, "/operate?count=3", nil)
	w := httptest.NewRecorder()
	c.handleOperate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	state := c.State()
	// Each operation opens a pipe pair (2 FDs)
	if state.FDCount != 6 {
		t.Errorf("FDCount: got %d, want 6", state.FDCount)
	}
}

func TestStoppedCanaryCompletesZeroOperations(t *testing.T) {
	c, _ := NewCanary(ModeGrowing, 1024*1024)
	c.SetReady()
	c.Stop()

	req := httptest.NewRequest(http.MethodPost, "/operate?count=5", nil)
	w := httptest.NewRecorder()
	c.handleOperate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["completed"] != float64(0) {
		t.Errorf("completed: got %v, want 0", resp["completed"])
	}
}

func TestConcurrentRequestsDoNotDeadlock(t *testing.T) {
	c, _ := NewCanary(ModeBounded, 1024*1024)
	c.SetReady()

	// Run concurrent requests
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodPost, "/operate?count=1", nil)
			w := httptest.NewRecorder()
			c.handleOperate(w, req)
			done <- (w.Code == http.StatusOK)
		}()
	}

	// Wait for all to complete (with timeout)
	timeout := time.After(5 * time.Second)
	for i := 0; i < 10; i++ {
		select {
		case success := <-done:
			if !success {
				t.Errorf("request %d failed", i)
			}
		case <-timeout:
			t.Fatal("deadlock: timeout waiting for concurrent requests")
		}
	}
}

func TestHandleState(t *testing.T) {
	c, _ := NewCanary(ModeGrowing, 1024*1024)
	c.SetReady()

	req := httptest.NewRequest(http.MethodGet, "/state", nil)
	w := httptest.NewRecorder()
	c.handleState(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var state State
	if err := json.NewDecoder(w.Body).Decode(&state); err != nil {
		t.Fatalf("decode state: %v", err)
	}

	if state.Mode != ModeGrowing {
		t.Errorf("Mode: got %s, want growing", state.Mode)
	}

	if !state.Ready {
		t.Error("Ready should be true")
	}
}

func TestHandleHealth(t *testing.T) {
	c, _ := NewCanary(ModeBounded, 1024*1024)
	c.SetReady()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	c.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestDefaultCountIs1(t *testing.T) {
	c, _ := NewCanary(ModeGrowing, 1024*1024)
	c.SetReady()

	// No count parameter defaults to 1
	req := httptest.NewRequest(http.MethodPost, "/operate", nil)
	w := httptest.NewRecorder()
	c.handleOperate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["completed"] != float64(1) {
		t.Errorf("completed: got %v, want 1", resp["completed"])
	}
}

func TestCloseFDs(t *testing.T) {
	c, _ := NewCanary(ModeDescriptor, 1024*1024)
	c.SetReady()

	// Open some FDs
	req := httptest.NewRequest(http.MethodPost, "/operate?count=3", nil)
	w := httptest.NewRecorder()
	c.handleOperate(w, req)

	// Close FDs
	c.CloseFDs()

	state := c.State()
	if state.FDCount != 0 {
		t.Errorf("FDCount after close: got %d, want 0", state.FDCount)
	}
}

// Test with body to ensure POST body handling works
func TestHandleOperateWithBody(t *testing.T) {
	c, _ := NewCanary(ModeGrowing, 1024*1024)
	c.SetReady()

	body := bytes.NewBufferString("")
	req := httptest.NewRequest(http.MethodPost, "/operate?count=2", body)
	w := httptest.NewRecorder()
	c.handleOperate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	state := c.State()
	if state.OperationCount != 2 {
		t.Errorf("OperationCount: got %d, want 2", state.OperationCount)
	}
}
