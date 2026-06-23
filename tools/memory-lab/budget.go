// budget.go — YAML budget loading and decision logic
//
// Loads memory budgets from docs/memory/budgets/<service>-memory-budget.yaml
// and makes pass/fail decisions based on measured values.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// Budget represents a parsed memory budget YAML file.
type Budget struct {
	Version         string                   `yaml:"version"`
	Service         string                   `yaml:"service"`
	Description     string                   `yaml:"description"`
	ArchBudgets     map[string]ArchBudget    `yaml:"arch_budgets"`
	WorkloadBudgets map[string]WorkloadBudget `yaml:"workload_budgets"`
}

// ArchBudget represents budget for a specific architecture.
type ArchBudget struct {
	Idle      *IdleBudget      `yaml:"idle,omitempty"`
	Warm      *WarmBudget      `yaml:"warm,omitempty"`
	HotPaths  map[string]HotPathBudget `yaml:"hot_paths,omitempty"`
	History   *HistoryBudget   `yaml:"history,omitempty"`
	MaxResponse *MaxResponseBudget `yaml:"max_response,omitempty"`
}

// IdleBudget is the idle state budget.
type IdleBudget struct {
	RSSKiB           interface{} `yaml:"rss_kib"` // int or "baseline_required"
	PSSKiB           interface{} `yaml:"pss_kib"`
	HeapAllocBytes   interface{} `yaml:"heap_alloc_bytes"`
	GoroutinesMax    interface{} `yaml:"goroutines_max"`
	Notes            string      `yaml:"notes,omitempty"`
}

// WarmBudget is the warm steady-state budget.
type WarmBudget struct {
	RSSKiB           interface{} `yaml:"rss_kib"`
	PSSKiB           interface{} `yaml:"pss_kib"`
	HeapAllocBytes   interface{} `yaml:"heap_alloc_bytes"`
	GoroutinesMax    interface{} `yaml:"goroutines_max"`
	GrowthAllowedKiB interface{} `yaml:"growth_allowed_kib"`
	Notes            string      `yaml:"notes,omitempty"`
}

// HotPathBudget is per-operation allocation budget.
type HotPathBudget struct {
	AllocBytesMax interface{} `yaml:"alloc_bytes_max"`
	Notes         string      `yaml:"notes,omitempty"`
}

// HistoryBudget is retention limits.
type HistoryBudget struct {
	MaxRetainedRoutes    int `yaml:"max_retained_routes,omitempty"`
	MaxRetainedSessions  int `yaml:"max_retained_sessions,omitempty"`
	MaxRetainedTargets   int `yaml:"max_retained_targets,omitempty"`
	MaxRetainedResults   int `yaml:"max_retained_results,omitempty"`
	MaxDiagBufferKiB     int `yaml:"max_diagnostic_buffer_kib,omitempty"`
	MaxLogBufferMB       int `yaml:"max_log_buffer_mb,omitempty"`
}

// MaxResponseBudget is bounded response sizes.
type MaxResponseBudget struct {
	StatusJSONKiB           int `yaml:"status_json_kib,omitempty"`
	StatusPlaintextKiB      int `yaml:"status_plaintext_kib,omitempty"`
	DiagnosticPacketKiB     int `yaml:"diagnostic_packet_kib,omitempty"`
}

// WorkloadBudget is workload-specific budget.
type WorkloadBudget struct {
	Description        string      `yaml:"description"`
	MaxGrowthKiB       interface{} `yaml:"max_growth_kib"`
	MaxGrowthPercent   float64     `yaml:"max_growth_percent"`
	Notes              string      `yaml:"notes,omitempty"`
}

// LoadBudget loads a budget YAML file from the standard location.
func LoadBudget(service string) (*Budget, error) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return nil, fmt.Errorf("repo root: %w", err)
	}

	budgetPath := filepath.Join(repoRoot, "docs", "memory", "budgets", service+"-memory-budget.yaml")
	data, err := os.ReadFile(budgetPath)
	if err != nil {
		return nil, fmt.Errorf("read budget: %w", err)
	}

	var budget Budget
	if err := yaml.Unmarshal(data, &budget); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	return &budget, nil
}

// findRepoRoot finds the repository root by looking for AGENTS.md.
func findRepoRoot() (string, error) {
	// Start from current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up looking for AGENTS.md
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return cwd, nil
}

// BudgetDecision contains the result of budget checking.
type BudgetDecision struct {
	Pass   bool
	Reason string
}

// CheckWorkloadBudget checks memory growth against workload budget.
func (b *Budget) CheckWorkloadBudget(workloadType string, growthRSSKiB int64, growthPercent float64) BudgetDecision {
	// Find matching workload budget
	var wb *WorkloadBudget
	switch workloadType {
	case "idle-warmup":
		if v, ok := b.WorkloadBudgets["idle_warmup"]; ok {
			wb = &v
		}
	case "status-json-warmup", "status-warmup":
		if v, ok := b.WorkloadBudgets["status_json_warmup"]; ok {
			wb = &v
		}
	case "status-json-network-diag":
		if v, ok := b.WorkloadBudgets["status_json_network_diag"]; ok {
			wb = &v
		}
	case "status-api-polling":
		if v, ok := b.WorkloadBudgets["status_api_polling"]; ok {
			wb = &v
		}
	case "diagnostic-capture-loop":
		if v, ok := b.WorkloadBudgets["diagnostic_capture_loop"]; ok {
			wb = &v
		}
	}

	if wb == nil {
		// No matching workload budget - measurement only
		return BudgetDecision{
			Pass:   true,
			Reason: fmt.Sprintf("No budget for workload type %q; measurement recorded", workloadType),
		}
	}

	// Check if max_growth_kib is baseline_required
	maxGrowthKiB, isBaseline := parseBudgetValue(wb.MaxGrowthKiB)
	if isBaseline {
		return BudgetDecision{
			Pass:   true,
			Reason: fmt.Sprintf("baseline_required: RSS growth %d KiB (%.2f%%) recorded, budget pending measurement", growthRSSKiB, growthPercent),
		}
	}

	// Check growth against budget
	pass := true
	reason := "within budget"
	if growthRSSKiB > maxGrowthKiB {
		pass = false
		reason = fmt.Sprintf("RSS growth %d KiB exceeds budget %d KiB", growthRSSKiB, maxGrowthKiB)
	}
	if growthPercent > wb.MaxGrowthPercent {
		pass = false
		reason = fmt.Sprintf("RSS growth %.2f%% exceeds budget %.2f%%", growthPercent, wb.MaxGrowthPercent)
	}

	if pass {
		reason = fmt.Sprintf("RSS growth %d KiB (%.2f%%) within budget %d KiB (%.2f%%)",
			growthRSSKiB, growthPercent, maxGrowthKiB, wb.MaxGrowthPercent)
	}

	return BudgetDecision{
		Pass:   pass,
		Reason: reason,
	}
}

// CheckIdleBudget checks idle memory against arch budget.
func (b *Budget) CheckIdleBudget(arch string, rssKiB, pssKiB int64) BudgetDecision {
	archBudget, ok := b.ArchBudgets[arch]
	if !ok {
		return BudgetDecision{
			Pass:   true,
			Reason: fmt.Sprintf("No budget for arch %q; measurement recorded", arch),
		}
	}

	if archBudget.Idle == nil {
		return BudgetDecision{
			Pass:   true,
			Reason: "No idle budget defined; measurement recorded",
		}
	}

	idle := archBudget.Idle
	budgetRSS, isBaseline := parseBudgetValue(idle.RSSKiB)
	if isBaseline {
		return BudgetDecision{
			Pass:   true,
			Reason: fmt.Sprintf("baseline_required: idle RSS %d KiB recorded, budget pending measurement", rssKiB),
		}
	}

	pass := rssKiB <= budgetRSS
	reason := "idle RSS within budget"
	if !pass {
		reason = fmt.Sprintf("idle RSS %d KiB exceeds budget %d KiB", rssKiB, budgetRSS)
	} else {
		reason = fmt.Sprintf("idle RSS %d KiB within budget %d KiB", rssKiB, budgetRSS)
	}

	return BudgetDecision{
		Pass:   pass,
		Reason: reason,
	}
}

// parseBudgetValue parses a budget value that may be int or "baseline_required".
func parseBudgetValue(v interface{}) (int64, bool) {
	if v == nil {
		return 0, true
	}

	switch val := v.(type) {
	case int:
		return int64(val), false
	case int64:
		return val, false
	case float64:
		return int64(val), false
	case string:
		if val == "baseline_required" {
			return 0, true
		}
		return 0, true
	default:
		return 0, true
	}
}

// GetArch returns the current architecture string.
func GetArch() string {
	return "linux/" + runtime.GOARCH
}
