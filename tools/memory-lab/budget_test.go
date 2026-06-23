// budget_test.go — Tests for YAML budget loading and decision logic

package main

import (
	"testing"
)

func TestParseBudgetValue(t *testing.T) {
	tests := []struct {
		name        string
		input       interface{}
		expectedVal int64
		isBaseline  bool
	}{
		{"int value", 5000, 5000, false},
		{"int64 value", int64(6000), 6000, false},
		{"float64 value", float64(7000), 7000, false},
		{"baseline_required", "baseline_required", 0, true},
		{"unknown string", "unknown", 0, true},
		{"nil", nil, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, isBaseline := parseBudgetValue(tt.input)
			if val != tt.expectedVal {
				t.Errorf("parseBudgetValue(%v) val = %d, want %d", tt.input, val, tt.expectedVal)
			}
			if isBaseline != tt.isBaseline {
				t.Errorf("parseBudgetValue(%v) isBaseline = %v, want %v", tt.input, isBaseline, tt.isBaseline)
			}
		})
	}
}

func TestBudgetDecisionPass(t *testing.T) {
	budget := &Budget{
		WorkloadBudgets: map[string]WorkloadBudget{
			"status_json_warmup": {
				Description:      "Repeated /status.json calls",
				MaxGrowthKiB:     100,
				MaxGrowthPercent: 5.0,
			},
		},
	}

	decision := budget.CheckWorkloadBudget("status-json-warmup", 50, 2.5)

	if !decision.Pass {
		t.Errorf("Decision.Pass = false, want true for within budget")
	}
	if decision.Reason == "" {
		t.Errorf("Decision.Reason should not be empty")
	}
}

func TestBudgetDecisionFailKiB(t *testing.T) {
	budget := &Budget{
		WorkloadBudgets: map[string]WorkloadBudget{
			"status_json_warmup": {
				Description:      "Repeated /status.json calls",
				MaxGrowthKiB:     100,
				MaxGrowthPercent: 5.0,
			},
		},
	}

	decision := budget.CheckWorkloadBudget("status-json-warmup", 150, 2.0)

	if decision.Pass {
		t.Errorf("Decision.Pass = true, want false for exceeding KiB budget")
	}
}

func TestBudgetDecisionFailPercent(t *testing.T) {
	budget := &Budget{
		WorkloadBudgets: map[string]WorkloadBudget{
			"status_json_warmup": {
				Description:      "Repeated /status.json calls",
				MaxGrowthKiB:     1000,
				MaxGrowthPercent: 5.0,
			},
		},
	}

	decision := budget.CheckWorkloadBudget("status-json-warmup", 100, 10.0)

	if decision.Pass {
		t.Errorf("Decision.Pass = true, want false for exceeding percent budget")
	}
}

func TestBudgetDecisionBaselineRequired(t *testing.T) {
	budget := &Budget{
		WorkloadBudgets: map[string]WorkloadBudget{
			"idle_warmup": {
				Description:      "Idle warmup",
				MaxGrowthKiB:     "baseline_required",
				MaxGrowthPercent: 5.0,
			},
		},
	}

	decision := budget.CheckWorkloadBudget("idle-warmup", 200, 4.0)

	if !decision.Pass {
		t.Errorf("Decision.Pass = false, want true for baseline_required")
	}
	if decision.Reason == "" {
		t.Errorf("Decision.Reason should not be empty for baseline_required")
	}
}

func TestBudgetDecisionNoMatchingWorkload(t *testing.T) {
	budget := &Budget{
		WorkloadBudgets: map[string]WorkloadBudget{},
	}

	decision := budget.CheckWorkloadBudget("unknown-workload", 50, 2.5)

	if !decision.Pass {
		t.Errorf("Decision.Pass = false, want true for no matching workload")
	}
}

func TestCheckIdleBudgetBaselineRequired(t *testing.T) {
	budget := &Budget{
		ArchBudgets: map[string]ArchBudget{
			"linux/arm64": {
				Idle: &IdleBudget{
					RSSKiB: "baseline_required",
					PSSKiB: "baseline_required",
				},
			},
		},
	}

	decision := budget.CheckIdleBudget("linux/arm64", 5120, 4800)

	if !decision.Pass {
		t.Errorf("Decision.Pass = false, want true for baseline_required")
	}
}

func TestCheckIdleBudgetWithinBudget(t *testing.T) {
	budget := &Budget{
		ArchBudgets: map[string]ArchBudget{
			"linux/arm64": {
				Idle: &IdleBudget{
					RSSKiB: 6000,
					PSSKiB: 5500,
				},
			},
		},
	}

	decision := budget.CheckIdleBudget("linux/arm64", 5120, 4800)

	if !decision.Pass {
		t.Errorf("Decision.Pass = false, want true for within budget")
	}
}

func TestCheckIdleBudgetExceedsBudget(t *testing.T) {
	budget := &Budget{
		ArchBudgets: map[string]ArchBudget{
			"linux/arm64": {
				Idle: &IdleBudget{
					RSSKiB: 4000,
					PSSKiB: 3500,
				},
			},
		},
	}

	decision := budget.CheckIdleBudget("linux/arm64", 5120, 4800)

	if decision.Pass {
		t.Errorf("Decision.Pass = true, want false for exceeding budget")
	}
}

func TestCheckIdleBudgetNoArch(t *testing.T) {
	budget := &Budget{
		ArchBudgets: map[string]ArchBudget{},
	}

	decision := budget.CheckIdleBudget("linux/arm64", 5120, 4800)

	if !decision.Pass {
		t.Errorf("Decision.Pass = false, want true for no matching arch")
	}
}

func TestGetArch(t *testing.T) {
	arch := GetArch()
	if arch == "" {
		t.Errorf("GetArch() returned empty string")
	}
	// Should contain "linux/" prefix
	if arch[:6] != "linux/" {
		t.Errorf("GetArch() = %q, want linux/ prefix", arch)
	}
}
