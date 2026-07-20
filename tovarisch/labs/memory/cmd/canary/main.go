// cmd/canary/main.go — Deterministic Memory Canary
//
// Compiled canary for memory behavior classification.
// Runs as a Docker container subject with machine-readable counters.
//
// Reference: kgb://factory/workflow

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

const (
	// MaxOperations is the maximum operations allowed per request
	MaxOperations = 1000
)

// Mode represents the canary operation mode.
type Mode string

const (
	ModeBounded    Mode = "bounded"
	ModeGrowing    Mode = "growing"
	ModeDescriptor Mode = "descriptor"
)

// State holds the canary's current state.
type State struct {
	Mode           Mode          `json:"mode"`
	RetainedBlocks int           `json:"retained_blocks"`
	RetainedBytes  int64         `json:"retained_bytes"`
	OperationCount int64         `json:"operation_count"`
	FDCount        int           `json:"fd_count"`
	BufferCapacity int64         `json:"buffer_capacity,omitempty"`
	AllocatedBytes int64         `json:"allocated_bytes,omitempty"`
	Ready          bool          `json:"ready"`
	Uptime         time.Duration `json:"uptime"`
	StartTime      time.Time     `json:"start_time"`
}

// Canary implements the memory canary behavior.
type Canary struct {
	mu        sync.Mutex
	mode      Mode
	state     State
	stopped   bool
	startTime time.Time

	// Bounded mode
	buffer []byte

	// Growing mode
	retained  [][]byte
	blockSize int64

	// Descriptor mode
	openFDs []io.Closer
}

func main() {
	mode := flag.String("mode", "bounded", "Mode: bounded, growing, descriptor")
	blockSize := flag.Int64("block-size", 1024*1024, "Block size in bytes for growing mode")
	port := flag.Int("port", 8080, "HTTP status port")
	flag.Parse()

	c, err := NewCanary(Mode(*mode), *blockSize)
	if err != nil {
		log.Fatalf("create canary: %v", err)
	}

	// Start HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/state", c.handleState)
	mux.HandleFunc("/health", c.handleHealth)
	mux.HandleFunc("/operate", c.handleOperate)

	addr := fmt.Sprintf(":%d", *port)
	srv := &http.Server{Addr: addr, Handler: mux}

	// Start server in goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	}()

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Mark as ready
	c.SetReady()

	<-sigCh

	// Graceful shutdown
	c.Stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)

	// Close all descriptors in descriptor mode
	c.CloseFDs()
}

func NewCanary(mode Mode, blockSize int64) (*Canary, error) {
	c := &Canary{
		mode:      mode,
		blockSize: blockSize,
		startTime: time.Now(),
	}

	switch mode {
	case ModeBounded:
		// Allocate one fixed-capacity buffer
		c.buffer = make([]byte, 1024*1024) // 1MB buffer
		c.state.BufferCapacity = int64(len(c.buffer))

	case ModeGrowing:
		// Initialize retained slice
		c.retained = make([][]byte, 0)
		c.state.AllocatedBytes = 0

	case ModeDescriptor:
		// Initialize FDs slice
		c.openFDs = make([]io.Closer, 0)

	default:
		return nil, fmt.Errorf("unknown mode: %s", mode)
	}

	return c, nil
}

func (c *Canary) SetReady() {
	c.mu.Lock()
	c.state.Ready = true
	c.mu.Unlock()
}

func (c *Canary) Stop() {
	c.mu.Lock()
	c.stopped = true
	c.mu.Unlock()
}

func (c *Canary) CloseFDs() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, fd := range c.openFDs {
		fd.Close()
	}
	c.openFDs = c.openFDs[:0]
	c.state.FDCount = 0
}

func (c *Canary) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()

	s := c.state
	s.Mode = c.mode
	s.Uptime = time.Since(c.startTime)

	switch c.mode {
	case ModeBounded:
		// Count open FDs
		s.FDCount = countFDs()

	case ModeGrowing:
		s.RetainedBlocks = len(c.retained)
		s.RetainedBytes = int64(c.state.AllocatedBytes)

	case ModeDescriptor:
		s.FDCount = len(c.openFDs)
	}

	return s
}

// BoundedOperation performs one bounded mutation operation.
func (c *Canary) BoundedOperation() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped || c.mode != ModeBounded {
		return
	}

	// Mutate the buffer
	for i := range c.buffer {
		c.buffer[i] = byte(i % 256)
	}

	c.state.OperationCount++
}

// GrowingOperation performs one growing memory operation.
func (c *Canary) GrowingOperation() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped || c.mode != ModeGrowing {
		return
	}

	// Allocate and touch every page
	block := make([]byte, c.blockSize)
	for i := range block {
		block[i] = 1
	}

	c.retained = append(c.retained, block)
	c.state.AllocatedBytes += c.blockSize
	c.state.OperationCount++
}

// DescriptorOperation performs one descriptor leak operation.
func (c *Canary) DescriptorOperation() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped || c.mode != ModeDescriptor {
		return
	}

	// Open a real pipe pair
	r, w, err := os.Pipe()
	if err != nil {
		return
	}

	c.openFDs = append(c.openFDs, r, w)
	c.state.OperationCount++
}

func (c *Canary) handleState(w http.ResponseWriter, r *http.Request) {
	state := c.State()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

func (c *Canary) handleHealth(w http.ResponseWriter, r *http.Request) {
	state := c.State()
	if state.Ready {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK\nMode: %s\nOperations: %d\n", c.mode, state.OperationCount)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "NOT READY\n")
	}
}

// handleOperate performs N operations based on mode.
// POST /operate?count=N (only POST allowed)
func (c *Canary) handleOperate(w http.ResponseWriter, r *http.Request) {
	// Only POST allowed
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse count parameter
	countStr := r.URL.Query().Get("count")
	var count int
	if countStr == "" {
		count = 1
	} else {
		n, err := strconv.Atoi(countStr)
		if err != nil {
			http.Error(w, "invalid count parameter", http.StatusBadRequest)
			return
		}
		count = n
	}

	// Reject negative or excessive counts
	if count < 0 {
		http.Error(w, "count must be non-negative", http.StatusBadRequest)
		return
	}
	if count > MaxOperations {
		http.Error(w, fmt.Sprintf("count exceeds maximum of %d", MaxOperations), http.StatusBadRequest)
		return
	}

	// Perform operations based on mode
	var completed int
	c.mu.Lock()
	if !c.stopped {
		switch c.mode {
		case ModeBounded:
			// Inline bounded operation to avoid deadlock
			for i := 0; i < count; i++ {
				if c.buffer != nil {
					for j := range c.buffer {
						c.buffer[j] = byte(j % 256)
					}
				}
				c.state.OperationCount++
				completed++
			}
		case ModeGrowing:
			for i := 0; i < count; i++ {
				block := make([]byte, c.blockSize)
				for j := range block {
					block[j] = 1
				}
				c.retained = append(c.retained, block)
				c.state.AllocatedBytes += c.blockSize
				c.state.OperationCount++
				completed++
			}
		case ModeDescriptor:
			for i := 0; i < count; i++ {
				r, w, err := os.Pipe()
				if err == nil {
					c.openFDs = append(c.openFDs, r, w)
					c.state.OperationCount++
					completed++
				}
			}
		}
	}
	// Get current operation count while still holding lock
	opCount := c.state.OperationCount
	c.mu.Unlock()

	// Return result
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"attempted":       count,
		"completed":       completed,
		"operation_count": opCount,
	})
}

// countFDs counts open file descriptors for the current process.
func countFDs() int {
	n := 0
	for i := 0; ; i++ {
		_, err := os.Stat(fmt.Sprintf("/proc/self/fd/%d", i))
		if err != nil {
			break
		}
		n++
	}
	return n
}
