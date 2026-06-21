// Package polling provides typed polling logic for UVB-76 capture netns lab.
package polling

// --- Test Helpers ---

func latencySeriesJSON(retained, returned, sample int) []byte {
	return []byte(`{"retained_sample_count":` + itoa(retained) +
		`,"returned_point_count":` + itoa(returned) +
		`,"sample_count":` + itoa(sample) + `}`)
}

func spikesResponseJSON(count int, spikes string) []byte {
	return []byte(`{"count":` + itoa(count) + `,"spikes":` + spikes + `}`)
}

func spikeJSON(eventID, sampleTS string, reasons []string, captures string) string {
	reasonStr := `[]`
	if len(reasons) > 0 {
		// Build: ["reason1","reason2",...]
		quoted := make([]string, len(reasons))
		for i, r := range reasons {
			quoted[i] = `"` + r + `"`
		}
		reasonStr = `[` + joinStrings(quoted, `,`) + `]`
	}
	return `{"event_id":"` + eventID + `","sample_ts":"` + sampleTS + `","reasons":` + reasonStr + `,"captures":` + captures + `}`
}

func captureJSON(status, captureStatus string) string {
	return `[{"status":"` + status + `","capture_status":"` + captureStatus + `","capture_started_at":"2024-01-01T00:01:00Z"}]`
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

func joinStrings(s []string, sep string) string {
	if len(s) == 0 {
		return ""
	}
	result := s[0]
	for i := 1; i < len(s); i++ {
		result += sep + s[i]
	}
	return result
}
