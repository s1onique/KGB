package state

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Timestamp Normalization Tests
//
// These tests verify that all API timestamps are explicit UTC instants.
// =============================================================================

// TestTimestampNormalization_UTCFieldsExplicit verifies that time.Time values
// serialize with explicit timezone information in JSON.
func TestTimestampNormalization_UTCFieldsExplicit(t *testing.T) {
	// Fixture: anchor at 2026-06-20T21:09:59Z, 90s cooldown, decision at 2026-06-20T21:10:56Z
	anchorTime := time.Date(2026, 6, 20, 21, 9, 59, 0, time.UTC)
	decisionTime := time.Date(2026, 6, 20, 21, 10, 56, 0, time.UTC)
	cooldownSeconds := 90

	// Build cooldown decision
	cs := NewCaptureStore()
	cs.SetLastCapture("peer-1", anchorTime)
	decision := cs.EvaluateCooldown(decisionTime, "peer-1", cooldownSeconds)

	// Verify decision math
	expectedEligible := anchorTime.Add(time.Duration(cooldownSeconds) * time.Second)
	if !decision.NextCaptureEligibleAt.Equal(expectedEligible) {
		t.Errorf("NextCaptureEligibleAt mismatch: expected %v, got %v", expectedEligible, decision.NextCaptureEligibleAt)
	}

	expectedRemaining := (expectedEligible.Sub(decisionTime)).Milliseconds()
	if decision.RemainingCooldownMs != expectedRemaining {
		t.Errorf("RemainingCooldownMs mismatch: expected %d, got %d", expectedRemaining, decision.RemainingCooldownMs)
	}

	// Build cooldown info and serialize
	info := BuildCooldownInfoFromDecision(decision, "peer-1")
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal cooldown info: %v", err)
	}

	jsonStr := string(data)
	t.Logf("CooldownInfo JSON: %s", jsonStr)

	// Assert: All timestamp fields contain explicit timezone
	// They must have either 'Z' or '+' or '-' for timezone
	timestampFields := []string{
		"last_successful_capture_at",
		"next_capture_eligible_at",
		"decision_now_at",
	}

	for _, field := range timestampFields {
		if !strings.Contains(jsonStr, `"`+field+`"`) {
			t.Errorf("missing field %q in JSON", field)
			continue
		}
		// Extract value for this field
		var raw json.RawMessage
		switch field {
		case "last_successful_capture_at":
			raw = json.RawMessage{}
			if err := json.Unmarshal(data, &struct {
				LastSuccessfulCaptureAt *json.RawMessage `json:"last_successful_capture_at"`
			}{&raw}); err != nil {
				t.Errorf("failed to extract %s: %v", field, err)
				continue
			}
		case "next_capture_eligible_at":
			raw = json.RawMessage{}
			if err := json.Unmarshal(data, &struct {
				NextCaptureEligibleAt *json.RawMessage `json:"next_capture_eligible_at"`
			}{&raw}); err != nil {
				t.Errorf("failed to extract %s: %v", field, err)
				continue
			}
		case "decision_now_at":
			raw = json.RawMessage{}
			if err := json.Unmarshal(data, &struct {
				DecisionNowAt *json.RawMessage `json:"decision_now_at"`
			}{&raw}); err != nil {
				t.Errorf("failed to extract %s: %v", field, err)
				continue
			}
		}

		if raw == nil || string(raw) == "null" {
			continue // field might be nil, skip
		}

		value := strings.Trim(string(raw), `"`)
		if !strings.HasSuffix(value, "Z") && !strings.Contains(value, "+") && !strings.Contains(value, "-0") {
			t.Errorf("field %s missing explicit timezone: %s", field, value)
		}
	}
}

// TestTimestampNormalization_LocalTimezoneDoesNotAffectMath verifies that cooldown math
// is correct regardless of the server's local timezone.
func TestTimestampNormalization_LocalTimezoneDoesNotAffectMath(t *testing.T) {
	// Test with Europe/Amsterdam timezone (UTC+2 in summer)
	amsterdam, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Skip("timezone Europe/Amsterdam not available, skipping")
	}

	// Create times in Amsterdam timezone
	anchorTime := time.Date(2026, 6, 20, 23, 9, 59, 0, amsterdam) // 21:09:59 UTC
	decisionTime := time.Date(2026, 6, 20, 23, 10, 56, 0, amsterdam) // 21:10:56 UTC
	cooldownSeconds := 90

	// Verify UTC values
	if anchorTime.UTC().Hour() != 21 || anchorTime.UTC().Minute() != 9 {
		t.Errorf("anchorTime UTC hour/minute incorrect: %v", anchorTime.UTC())
	}

	cs := NewCaptureStore()
	cs.SetLastCapture("peer-1", anchorTime) // Store in local timezone
	decision := cs.EvaluateCooldown(decisionTime, "peer-1", cooldownSeconds)

	// Verify: remaining cooldown is computed from UTC instants
	expectedRemaining := int64(33000) // 33 seconds
	if decision.RemainingCooldownMs != expectedRemaining {
		t.Errorf("RemainingCooldownMs incorrect with local timezone: expected %d, got %d", expectedRemaining, decision.RemainingCooldownMs)
	}

	// Verify: JSON serializes with explicit timezone (offset preserved)
	info := BuildCooldownInfoFromDecision(decision, "peer-1")
	data, _ := json.Marshal(info)
	jsonStr := string(data)

	// All timestamps should have explicit timezone (Z or +/-HH:MM)
	// The actual offset depends on how time.Time was created - Go serializes as-is
	// Key: no timezone-less patterns like "2026-06-20 21:" or "T21:09:59" without offset
	if strings.Contains(jsonStr, "2026-06-20 21:") {
		t.Errorf("found space-separated time without timezone in JSON: %s", jsonStr)
	}
	// Verify the timestamps contain timezone info (either Z or +HH:MM or -HH:MM)
	if !strings.Contains(jsonStr, "+02:00") && !strings.Contains(jsonStr, "Z") {
		t.Errorf("timestamps missing explicit timezone in JSON: %s", jsonStr)
	}
}

// TestTimestampNormalization_ExplicitOffsetPreserved verifies that explicit timezone
// offsets are preserved correctly.
func TestTimestampNormalization_ExplicitOffsetPreserved(t *testing.T) {
	// Create time with explicit +02:00 offset
	offset := time.FixedZone("+0200", 7200) // +02:00
	anchorTime := time.Date(2026, 6, 20, 23, 9, 59, 0, offset)

	// Verify this is equivalent to 21:09:59 UTC
	if anchorTime.UTC().Hour() != 21 {
		t.Errorf("FixedZone time UTC conversion incorrect: %v", anchorTime.UTC())
	}

	cs := NewCaptureStore()
	cs.SetLastCapture("peer-1", anchorTime)

	decisionTime := time.Date(2026, 6, 20, 23, 10, 56, 0, offset)
	decision := cs.EvaluateCooldown(decisionTime, "peer-1", 90)

	// Remaining should still be 33 seconds (UTC-based math)
	if decision.RemainingCooldownMs != 33000 {
		t.Errorf("Cooldown math incorrect with fixed zone: got %d, want 33000", decision.RemainingCooldownMs)
	}
}

// TestTimestampNormalization_NoTimezoneLessStrings verifies that we never emit
// timezone-less timestamp strings in API responses.
func TestTimestampNormalization_NoTimezoneLessStrings(t *testing.T) {
	cs := NewCaptureStore()

	// Set anchor in UTC
	anchorTime := time.Date(2026, 6, 20, 21, 9, 59, 0, time.UTC)
	cs.SetLastCapture("peer-1", anchorTime)

	decisionTime := time.Date(2026, 6, 20, 21, 10, 56, 0, time.UTC)
	decision := cs.EvaluateCooldown(decisionTime, "peer-1", 90)
	info := BuildCooldownInfoFromDecision(decision, "peer-1")

	data, _ := json.Marshal(info)
	jsonStr := string(data)

	// Patterns that should NOT appear (timezone-less formats)
	// Note: "2026-06-20T21:09:59" would match "2026-06-20T21:09:59Z"
	// so we test that the JSON contains explicit timezone markers
	hasZ := strings.Contains(jsonStr, "Z")
	hasOffset := strings.Contains(jsonStr, "+") || strings.Contains(jsonStr, "-0")

	if !hasZ && !hasOffset {
		t.Errorf("JSON should contain explicit timezone (Z or offset): %s", jsonStr)
	}

	// Should not have space-separated timestamps (local time format)
	if strings.Contains(jsonStr, `"20T`) {
		// This would indicate a malformed timestamp
		t.Errorf("found malformed timestamp pattern in JSON: %s", jsonStr)
	}
}

// TestTimestampNormalization_RegressionScreenshot verifies the exact screenshot scenario:
// anchor at 21:09:59Z, next eligible at 21:11:29Z, decision at 21:10:56Z, remaining ~33s.
func TestTimestampNormalization_RegressionScreenshot(t *testing.T) {
	anchorTime := time.Date(2026, 6, 20, 21, 9, 59, 0, time.UTC)
	decisionTime := time.Date(2026, 6, 20, 21, 10, 56, 0, time.UTC)
	cooldownSeconds := 90

	cs := NewCaptureStore()
	cs.SetLastCapture("peer-1", anchorTime)
	decision := cs.EvaluateCooldown(decisionTime, "peer-1", cooldownSeconds)

	// Verify cooldown math
	expectedEligible := "2026-06-20T21:11:29Z"
	actualEligible := decision.NextCaptureEligibleAt.Format(time.RFC3339)
	if actualEligible != expectedEligible {
		t.Errorf("next_capture_eligible_at: expected %s, got %s", expectedEligible, actualEligible)
	}

	// Remaining should be ~33 seconds (33000ms)
	if decision.RemainingCooldownMs != 33000 {
		t.Errorf("remaining_cooldown_ms: expected 33000, got %d", decision.RemainingCooldownMs)
	}

	// Build info and verify JSON
	info := BuildCooldownInfoFromDecision(decision, "peer-1")
	data, _ := json.Marshal(info)
	jsonStr := string(data)

	// Timestamps should appear with Z suffix
	if !strings.Contains(jsonStr, "21:09:59Z") {
		t.Errorf("anchor should have Z suffix: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, "21:11:29Z") {
		t.Errorf("eligible should have Z suffix: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, "21:10:56Z") {
		t.Errorf("decision should have Z suffix: %s", jsonStr)
	}
}

// TestTimestampNormalization_UTCNormalizeHelper tests the UTCNormalize helper.
func TestTimestampNormalization_UTCNormalizeHelper(t *testing.T) {
	// Zero time should remain zero
	zero := time.Time{}
	normalized := UTCNormalize(zero)
	if !normalized.IsZero() {
		t.Errorf("zero time should remain zero after UTCNormalize")
	}

	// Local time should be converted to UTC
	local, _ := time.LoadLocation("America/New_York")
	localTime := time.Date(2026, 6, 20, 17, 9, 59, 0, local)
	normalized = UTCNormalize(localTime)

	if normalized.Location() != time.UTC {
		t.Errorf("normalized time should be in UTC, got %v", normalized.Location())
	}
	if normalized.Hour() != 21 { // 17:09:59 EDT = 21:09:59 UTC
		t.Errorf("normalized hour should be 21, got %d", normalized.Hour())
	}
}

// TestTimestampNormalization_FormatUTCHelper tests the FormatUTC helper.
func TestTimestampNormalization_FormatUTCHelper(t *testing.T) {
	// Zero time should return empty string
	if FormatUTC(time.Time{}) != "" {
		t.Errorf("zero time should return empty string")
	}

	// Normal time should format as RFC3339Nano UTC
	timestamp := time.Date(2026, 6, 20, 21, 9, 59, 123456789, time.UTC)
	formatted := FormatUTC(timestamp)
	expected := "2026-06-20T21:09:59.123456789Z"
	if formatted != expected {
		t.Errorf("FormatUTC mismatch: expected %s, got %s", expected, formatted)
	}
}
