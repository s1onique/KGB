// Package memlab provides artifact contracts and classification for memory leak labs.
package memlab

// formatDelta formats an int64 delta in bytes for logging.
func formatDelta(label string, delta int64) string {
	if delta >= 0 {
		return label + "_increase_" + formatBytes(delta)
	}
	return label + "_decrease_" + formatBytes(-delta)
}

// formatCountDelta formats an int64 delta as a count (no unit suffix).
// Goroutines are counts, not bytes, so they use this function.
func formatCountDelta(label string, delta int64) string {
	if delta >= 0 {
		return label + "_delta_" + formatInt(delta)
	}
	return label + "_delta_" + formatInt(delta) // negative delta already formatted in formatInt
}

// formatBytes formats bytes for human-readable output.
func formatBytes(n int64) string {
	if n < 1024 {
		return formatInt(n) + "B"
	}
	if n < 1024*1024 {
		return formatFloat(float64(n)/1024, 1) + "KB"
	}
	if n < 1024*1024*1024 {
		return formatFloat(float64(n)/(1024*1024), 1) + "MB"
	}
	return formatFloat(float64(n)/(1024*1024*1024), 2) + "GB"
}

// formatInt formats an int64 without importing strconv.
func formatInt(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// formatFloat formats a float64 without importing strconv.
func formatFloat(f float64, decimals int) string {
	if f < 0 {
		return "-" + formatFloat(-f, decimals)
	}
	// Simple integer part + decimal
	intPart := int64(f)
	fracPart := f - float64(intPart)
	// Build fractional digits
	var fracDigits []byte
	for i := 0; i < decimals; i++ {
		fracPart *= 10
		digit := byte('0' + int(fracPart)%10)
		fracDigits = append(fracDigits, digit)
	}
	return formatInt(intPart) + "." + string(fracDigits)
}
