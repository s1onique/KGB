package state

import (
	"time"
)

// UTCNormalize ensures a time.Time is normalized to UTC for API serialization.
// This is a belt-and-suspenders helper - Go's json.Marshal already handles time.Time correctly,
// but explicit normalization makes the intent clear and guards against future changes.
//
// Usage: Call utcTime(t) before assigning to any field that will be serialized to JSON API responses.
func UTCNormalize(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC()
}

// FormatUTC formats a time.Time as an explicit RFC3339 string in UTC.
// Returns empty string for zero time.
func FormatUTC(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// MustUTC converts a pointer to time.Time to UTC, handling nil safely.
// Returns nil if the input is nil.
func MustUTC(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}
