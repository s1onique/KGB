// canary.go — Memory canary implementations for lab verification
//
// Provides known-growing and bounded-memory canaries to verify
// the laboratory's measurement integrity.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package dockerlab

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// CanaryMode defines the type of memory canary.
type CanaryMode string

const (
	CanaryGrowing    CanaryMode = "growing"
	CanaryBounded    CanaryMode = "bounded"
	CanaryDescriptor CanaryMode = "descriptor"
)

// CanaryConfig configures the canary container.
type CanaryConfig struct {
	Mode       CanaryMode
	BlockSize  int // Bytes per retained block
	BlockCount int // Number of blocks to retain per request
	Port       int
}

// GrowingMemoryCanary implements a container that grows memory per request.
type GrowingMemoryCanary struct {
	cfg    CanaryConfig
	mu     sync.Mutex
	blocks [][]byte
	server *http.Server
}

// NewGrowingMemoryCanary creates a growing memory canary.
func NewGrowingMemoryCanary(cfg CanaryConfig) *GrowingMemoryCanary {
	return &GrowingMemoryCanary{
		cfg:    cfg,
		blocks: make([][]byte, 0),
	}
}

// Start initializes the canary HTTP server.
func (c *GrowingMemoryCanary) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/alloc", c.handleAlloc)
	mux.HandleFunc("/free", c.handleFree)
	mux.HandleFunc("/stats", c.handleStats)

	c.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", c.cfg.Port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		c.server.Shutdown(context.Background())
	}()

	go c.server.ListenAndServe()
	return nil
}

// handleAlloc allocates memory blocks.
func (c *GrowingMemoryCanary) handleAlloc(w http.ResponseWriter, r *http.Request) {
	count := c.cfg.BlockCount
	if count <= 0 {
		count = 1
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for i := 0; i < count; i++ {
		block := make([]byte, c.cfg.BlockSize)
		// Touch the block to ensure it's resident
		for j := 0; j < len(block); j += 4096 {
			block[j] = 1
		}
		c.blocks = append(c.blocks, block)
	}

	fmt.Fprintf(w, "allocated %d blocks of %d bytes each, total: %d blocks",
		count, c.cfg.BlockSize, len(c.blocks))
}

// handleFree frees all allocated memory.
func (c *GrowingMemoryCanary) handleFree(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.blocks = nil

	fmt.Fprintf(w, "freed all blocks")
}

// handleStats returns current memory usage.
func (c *GrowingMemoryCanary) handleStats(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	fmt.Fprintf(w, "blocks: %d, total_bytes: %d", len(c.blocks), len(c.blocks)*c.cfg.BlockSize)
}

// Stop shuts down the canary.
func (c *GrowingMemoryCanary) Stop() error {
	if c.server != nil {
		return c.server.Shutdown(context.Background())
	}
	return nil
}

// BlockCount returns the number of retained blocks.
func (c *GrowingMemoryCanary) BlockCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.blocks)
}

// BoundedMemoryCanary implements a container that reuses a fixed buffer.
type BoundedMemoryCanary struct {
	cfg      CanaryConfig
	server   *http.Server
	fixedBuf []byte
}

// NewBoundedMemoryCanary creates a bounded memory canary.
func NewBoundedMemoryCanary(cfg CanaryConfig) *BoundedMemoryCanary {
	// Allocate fixed buffer once
	fixedBuf := make([]byte, cfg.BlockSize*cfg.BlockCount)
	// Touch all pages
	for i := 0; i < len(fixedBuf); i += 4096 {
		fixedBuf[i] = 1
	}

	return &BoundedMemoryCanary{
		cfg:      cfg,
		fixedBuf: fixedBuf,
	}
}

// Start initializes the canary HTTP server.
func (c *BoundedMemoryCanary) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/process", c.handleProcess)
	mux.HandleFunc("/stats", c.handleBoundedStats)

	c.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", c.cfg.Port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		c.server.Shutdown(context.Background())
	}()

	go c.server.ListenAndServe()
	return nil
}

func (c *BoundedMemoryCanary) handleProcess(w http.ResponseWriter, r *http.Request) {
	// Process request using fixed buffer
	body, _ := io.ReadAll(io.LimitReader(r.Body, int64(c.cfg.BlockSize)))
	result := c.processWithBuffer(body)
	w.Write(result)
}

func (c *BoundedMemoryCanary) handleBoundedStats(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "bounded_buffer_size: %d", len(c.fixedBuf))
}

// processWithBuffer processes request using fixed buffer.
func (c *BoundedMemoryCanary) processWithBuffer(input []byte) []byte {
	n := len(input)
	if n > len(c.fixedBuf) {
		n = len(c.fixedBuf)
	}
	copy(c.fixedBuf, input)
	return c.fixedBuf[:n]
}

// Stop shuts down the canary.
func (c *BoundedMemoryCanary) Stop() error {
	if c.server != nil {
		return c.server.Shutdown(context.Background())
	}
	return nil
}

// DescriptorCanary leaks one descriptor per operation.
type DescriptorCanary struct {
	cfg         CanaryConfig
	descriptors []io.Closer
	mu          sync.Mutex
	server      *http.Server
}

// NewDescriptorCanary creates a descriptor-leaking canary.
func NewDescriptorCanary(cfg CanaryConfig) *DescriptorCanary {
	return &DescriptorCanary{
		cfg:         cfg,
		descriptors: make([]io.Closer, 0),
	}
}

// Start initializes the canary HTTP server.
func (c *DescriptorCanary) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/leak", c.handleLeak)
	mux.HandleFunc("/stats", c.handleDescriptorStats)

	c.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", c.cfg.Port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		c.server.Shutdown(context.Background())
	}()

	go c.server.ListenAndServe()
	return nil
}

// handleLeak creates new pipe descriptors.
func (c *DescriptorCanary) handleLeak(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := 0; i < c.cfg.BlockCount; i++ {
		reader, writer := io.Pipe()
		c.descriptors = append(c.descriptors, reader)
		c.descriptors = append(c.descriptors, writer)
	}

	fmt.Fprintf(w, "leaked %d descriptors, total: %d", c.cfg.BlockCount*2, len(c.descriptors))
}

// handleDescriptorStats returns descriptor count.
func (c *DescriptorCanary) handleDescriptorStats(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	fmt.Fprintf(w, "descriptor_count: %d", len(c.descriptors))
}

// Stop shuts down and leaks descriptors.
func (c *DescriptorCanary) Stop() error {
	if c.server != nil {
		return c.server.Shutdown(context.Background())
	}
	return nil
}

// DescriptorCount returns the number of leaked descriptors.
func (c *DescriptorCanary) DescriptorCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.descriptors)
}

// CanaryVerifier verifies canary behavior.
type CanaryVerifier struct{}

// NewCanaryVerifier creates a new verifier.
func NewCanaryVerifier() *CanaryVerifier {
	return &CanaryVerifier{}
}

// VerifyGrowing verifies that a growing canary is detected as growing.
func (v *CanaryVerifier) VerifyGrowing(initialRSS, finalRSS int64) bool {
	// Should see significant growth
	growth := finalRSS - initialRSS
	return growth > 1024 // More than 1MB growth
}

// VerifyBounded verifies that a bounded canary is not falsely classified.
func (v *CanaryVerifier) VerifyBounded(initialRSS, finalRSS int64) bool {
	// Should see minimal growth (within allocator variation)
	growth := finalRSS - initialRSS
	return growth < 10240 // Less than 10MB growth
}

// VerifyDescriptors verifies descriptor growth is detected.
func (v *CanaryVerifier) VerifyDescriptors(initialFDs, finalFDs int) bool {
	return finalFDs > initialFDs+10 // Should have significantly more FDs
}

// CanaryClient is a client for hitting canary endpoints.
type CanaryClient struct {
	URL    string
	Client *http.Client
}

// NewCanaryClient creates a canary client.
func NewCanaryClient(url string) *CanaryClient {
	return &CanaryClient{
		URL: url,
		Client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Allocate triggers allocation in a growing canary.
func (c *CanaryClient) Allocate() error {
	resp, err := c.Client.Get(c.URL + "/alloc")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

// GetStats gets canary statistics.
func (c *CanaryClient) GetStats() (string, error) {
	resp, err := c.Client.Get(c.URL + "/stats")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
