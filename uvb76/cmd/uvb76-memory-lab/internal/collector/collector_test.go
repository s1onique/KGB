package collector

import (
	"testing"
)

func TestArtifactTimeSuffix(t *testing.T) {
	tests := []struct {
		seconds int
		want    string
	}{
		{0, "t000"},
		{30, "t030"},
		{60, "t060"},
		{600, "t600"},
		{61, "t061"},
		{599, "t599"},
	}

	for _, tt := range tests {
		got := ArtifactTimeSuffix(tt.seconds)
		if got != tt.want {
			t.Errorf("ArtifactTimeSuffix(%d) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}
