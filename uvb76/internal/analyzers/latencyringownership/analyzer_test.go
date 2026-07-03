package latencyringownership

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	// Test the analyzer with the testdata fixtures
	analysistest.Run(t, analysistest.TestData(), Analyzer,
		"github.com/s1onique/KGB/uvb76/state",
		"github.com/s1onique/KGB/uvb76/server",
		"github.com/s1onique/KGB/uvb76/diagnostics",
		"github.com/s1onique/KGB/uvb76/generated",
	)
}
