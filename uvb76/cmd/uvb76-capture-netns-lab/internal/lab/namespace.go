package lab

import (
	"context"
	"fmt"
	"time"
)

// Namespace represents a Linux network namespace.
type Namespace struct {
	Name   string
	IPCIDR string
}

// NetnsLabConfig holds the netns lab topology configuration.
type NetnsLabConfig struct {
	UVB76NS       Namespace
	TovarischNS   Namespace
	TovarischPort int
	UVB76Port     int
}

// DefaultNetnsLabConfig returns the default configuration.
func DefaultNetnsLabConfig() NetnsLabConfig {
	return NetnsLabConfig{
		UVB76NS: Namespace{
			Name:   "kgb-lab-uvb76",
			IPCIDR: "10.88.76.1/24",
		},
		TovarischNS: Namespace{
			Name:   "kgb-lab-tovarisch",
			IPCIDR: "10.88.76.2/24",
		},
		TovarischPort: 8317,
		UVB76Port:     8316,
	}
}

// NetnsHelper provides namespace operation helpers.
type NetnsHelper struct {
	Runner CommandRunner
	Config NetnsLabConfig
}

// NewNetnsHelper creates a new namespace helper.
func NewNetnsHelper(runner CommandRunner, config NetnsLabConfig) *NetnsHelper {
	return &NetnsHelper{
		Runner: runner,
		Config: config,
	}
}

// CreateNamespaces creates the lab namespaces and veth pairs.
func (h *NetnsHelper) CreateNamespaces(ctx context.Context) error {
	// Create namespaces
	if res := h.Runner.Run(ctx, "ip", "netns", "add", h.Config.UVB76NS.Name); !res.OK() {
		// Ignore "already exists" errors
		if res.ExitCode != 2 && !contains(res.Stderr, "File exists") {
			return fmt.Errorf("create uvb76 namespace: %w", res.Err)
		}
	}
	if res := h.Runner.Run(ctx, "ip", "netns", "add", h.Config.TovarischNS.Name); !res.OK() {
		if res.ExitCode != 2 && !contains(res.Stderr, "File exists") {
			return fmt.Errorf("create tovarisch namespace: %w", res.Err)
		}
	}

	// Create veth pair
	vethUVB76 := "uvb76-veth"
	vethTovarisch := "tovarisch-veth"
	if res := h.Runner.Run(ctx, "ip", "link", "add", vethUVB76, "type", "veth", "peer", "name", vethTovarisch); !res.OK() {
		return fmt.Errorf("create veth pair: %w", res.Err)
	}

	// Move ends to namespaces
	if res := h.Runner.Run(ctx, "ip", "link", "set", vethUVB76, "netns", h.Config.UVB76NS.Name); !res.OK() {
		return fmt.Errorf("move uvb76 veth: %w", res.Err)
	}
	if res := h.Runner.Run(ctx, "ip", "link", "set", vethTovarisch, "netns", h.Config.TovarischNS.Name); !res.OK() {
		return fmt.Errorf("move tovarisch veth: %w", res.Err)
	}

	return nil
}

// ConfigureInterfaces configures IP addresses on namespace interfaces.
func (h *NetnsHelper) ConfigureInterfaces(ctx context.Context) error {
	vethUVB76 := "uvb76-veth"
	vethTovarisch := "tovarisch-veth"

	// Configure uvb76 namespace
	if err := h.configureNS(h.Config.UVB76NS.Name, vethUVB76, h.Config.UVB76NS.IPCIDR); err != nil {
		return err
	}

	// Configure tovarisch namespace
	if err := h.configureNS(h.Config.TovarischNS.Name, vethTovarisch, h.Config.TovarischNS.IPCIDR); err != nil {
		return err
	}

	return nil
}

func (h *NetnsHelper) configureNS(ns, iface, ipCIDR string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	configs := [][]string{
		{"ip", "netns", "exec", ns, "ip", "addr", "add", ipCIDR, "dev", iface},
		{"ip", "netns", "exec", ns, "ip", "link", "set", iface, "up"},
		{"ip", "netns", "exec", ns, "ip", "link", "set", "lo", "up"},
	}

	for _, args := range configs {
		if res := h.Runner.Run(ctx, args[0], args[1:]...); !res.OK() {
			return fmt.Errorf("configure %s/%s: %w", ns, iface, res.Err)
		}
	}
	return nil
}

// NetnsExec runs a command inside a namespace.
func (h *NetnsHelper) NetnsExec(ctx context.Context, ns string, args ...string) CommandResult {
	fullArgs := append([]string{"netns", "exec", ns}, args...)
	return h.Runner.Run(ctx, "ip", fullArgs...)
}

// TC runs a tc command in a namespace.
func (h *NetnsHelper) TC(ctx context.Context, ns string, args ...string) CommandResult {
	fullArgs := append([]string{"netns", "exec", ns, "tc"}, args...)
	return h.Runner.Run(ctx, "ip", fullArgs...)
}

// Curl performs an HTTP request from within a namespace.
func (h *NetnsHelper) Curl(ctx context.Context, ns string, url string) CommandResult {
	return h.Runner.Run(ctx, "ip", "netns", "exec", ns, "curl", "-s", "-w", "%{http_code}", "-o", "/dev/null", url)
}

// Ping tests connectivity from one namespace to an IP.
func (h *NetnsHelper) Ping(ctx context.Context, ns string, targetIP string, count int) CommandResult {
	return h.Runner.Run(ctx, "ip", "netns", "exec", ns, "ping", "-c", fmt.Sprintf("%d", count), "-W", "2", targetIP)
}

// DeleteNamespaces removes the lab namespaces.
func (h *NetnsHelper) DeleteNamespaces(ctx context.Context) []error {
	var errors []error

	// Order matters: delete tovarisch first (connected to uvb76)
	for _, ns := range []string{h.Config.TovarischNS.Name, h.Config.UVB76NS.Name} {
		if res := h.Runner.Run(ctx, "ip", "netns", "del", ns); !res.OK() {
			errors = append(errors, fmt.Errorf("delete namespace %s: %w", ns, res.Err))
		}
	}

	return errors
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

